package plugins

import (
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"sort"
	"time"

	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"
)

type BatchQuoteItem struct {
	RunID           string `json:"runId"`
	Operation       string `json:"operation"`
	Revision        int64  `json:"revision"`
	ApprovalID      string `json:"approvalId"`
	ScopeDigest     string `json:"scopeDigest"`
	FeeMicrocredits int64  `json:"feeMicrocredits"`
}
type BatchQuote struct {
	Digest             string           `json:"digest"`
	Items              []BatchQuoteItem `json:"items"`
	AmountMicrocredits int64            `json:"amountMicrocredits"`
	VendorCostKnown    bool             `json:"vendorCostKnown"`
	ExpiresAt          time.Time        `json:"expiresAt"`
}
type BatchApproveRequest struct {
	Digest                string    `json:"digest"`
	Count                 int       `json:"count"`
	AmountMicrocredits    int64     `json:"amountMicrocredits"`
	ExpiresAt             time.Time `json:"expiresAt"`
	AcceptExternalBilling bool      `json:"acceptExternalBilling"`
}

func (s *Service) BatchQuote(user, id string) (BatchQuote, error) {
	q := BatchQuote{Items: []BatchQuoteItem{}, ExpiresAt: time.Now().UTC().Add(5 * time.Minute), VendorCostKnown: true}
	run, err := s.repo.PluginRunForUser(user, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return q, kernel.NotFound("插件运行不存在")
	}
	if err != nil {
		return q, err
	}
	if run.PipelineID == "" || !contains([]string{"running", "waiting_approval", "waiting_input"}, run.Status) {
		return q, issue(409, "run_revision_conflict", "当前流程不能批量批准")
	}
	rows, err := s.repo.PluginPipelineChildren(user, id)
	if err != nil {
		return q, err
	}
	for _, row := range rows {
		if row.Status != "waiting_approval" {
			continue
		}
		// Draft/canvas mutations retain their individual approval boundary.
		if row.ConnectionVersionID == "" {
			continue
		}
		var preview struct {
			Fee           *int64 `json:"platformFeeMicrocredits"`
			VendorBilling string `json:"vendorBilling"`
		}
		if json.Unmarshal([]byte(row.PlanJSON), &preview) != nil || preview.Fee == nil || *preview.Fee < 0 || *preview.Fee > 9007199254740991-q.AmountMicrocredits {
			return q, issue(409, "quote_changed", "服务费未知或超出安全范围，不能批量批准")
		}
		q.AmountMicrocredits += *preview.Fee
		// No supplier price source exists in the current BYOK connector contract.
		// An absent label is still unknown, never evidence of a free supplier.
		q.VendorCostKnown = false
		q.Items = append(q.Items, BatchQuoteItem{RunID: row.ID, Operation: row.Operation, Revision: row.Revision, ApprovalID: row.ApprovalID, ScopeDigest: hashBytes([]byte(encode([]string{row.RequestDigest, row.ContractHash, row.SourceDigest, row.ConnectionVersionID}))), FeeMicrocredits: *preview.Fee})
	}
	sort.Slice(q.Items, func(i, j int) bool { return q.Items[i].RunID < q.Items[j].RunID })
	q.Digest = hashBytes([]byte(encode(q.Items)))
	return q, nil
}

// Every Task reservation is made by the original Decide/Enqueue path in one
// transaction. A failure rolls back the entire wave, including its receipt.
func (s *Service) ApproveBatch(user, id string, req BatchApproveRequest) (RunView, error) {
	err := s.repo.WithPluginCatalog(func(repo *repository.Repository) error {
		if _, err := repo.PluginRunForUser(user, id); err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return kernel.NotFound("插件运行不存在")
			}
			return err
		}
		receipts, err := repo.PluginBatchApprovals(user, id)
		if err != nil {
			return err
		}
		for _, row := range receipts {
			if row.Digest == req.Digest {
				if row.Count != req.Count || row.AmountMicrocredits != req.AmountMicrocredits || !row.ExpiresAt.Equal(req.ExpiresAt) {
					return issue(409, "run_revision_conflict", "批次授权参数不一致")
				}
				return nil
			}
		}
		if req.ExpiresAt.Before(time.Now()) || req.ExpiresAt.After(time.Now().Add(5*time.Minute)) {
			return issue(409, "quote_changed", "批次授权已过期，请刷新报价")
		}
		local := *s
		local.repo = repo
		q, err := local.BatchQuote(user, id)
		if err != nil {
			return err
		}
		if len(q.Items) == 0 || len(q.Items) > 8 || req.Count != len(q.Items) || req.Digest != q.Digest || req.AmountMicrocredits != q.AmountMicrocredits {
			return issue(409, "quote_changed", "批次内容或费用已变化，请刷新报价")
		}
		if !q.VendorCostKnown && !req.AcceptExternalBilling {
			return issue(400, "operation_input_invalid", "供应商费用未知，需明确接受供应商独立计费；此额度只约束平台服务费")
		}
		if s.runAccess != nil {
			if err = s.runAccess(repo, user); err != nil {
				return err
			}
		}
		for _, item := range q.Items {
			if _, err = local.Decide(user, item.RunID, item.ApprovalID, "approve", item.Revision); err != nil {
				return err
			}
		}
		receipt := model.PluginBatchApproval{ID: kernel.NewID(), RunID: id, UserID: user, Digest: q.Digest, ScopeJSON: encode(q), AmountMicrocredits: q.AmountMicrocredits, Count: len(q.Items), ExpiresAt: req.ExpiresAt}
		if err = repo.CreatePluginBatchApproval(&receipt); err != nil {
			return err
		}
		run, err := repo.PluginRunForUser(user, id)
		if err != nil {
			return err
		}
		return SaveRunTransition(repo, run, "batch.approved")
	})
	if err != nil {
		return RunView{}, err
	}
	return s.GetRun(user, id)
}
