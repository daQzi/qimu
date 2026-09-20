package app

import (
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/repository"
)

func p06Start(t *testing.T, s *Service, release string, items string) plugins.RunView {
	t.Helper()
	out, err := s.InvokePluginOperation("user", newID(), contracts.Invocation{Operation: "batch-helper.process", ReleaseID: release, Input: p03Input(map[string]any{"resourceId": "video-one"})})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.applicationPlugins().AdvancePipeline("user", out.RunID); err != nil {
		t.Fatal(err)
	}
	view, err := s.PluginRun("user", out.RunID)
	if err != nil || len(view.Pipeline.Inputs) != 1 {
		t.Fatalf("input: %+v %v", view, err)
	}
	input := view.Pipeline.Inputs[0]
	if _, err = s.UpdatePluginInput("user", out.RunID, input.ID, newID(), PluginInputUpdate{Revision: input.Revision, Mode: "submit", Value: json.RawMessage(`{"items":` + items + `}`)}); err != nil {
		t.Fatal(err)
	}
	if err = s.applicationPlugins().AdvancePipeline("user", out.RunID); err != nil {
		t.Fatal(err)
	}
	view, err = s.PluginRun("user", out.RunID)
	if err != nil {
		t.Fatal(err)
	}
	return view
}
func p06Approve(t *testing.T, s *Service, id string) plugins.BatchApproveRequest {
	t.Helper()
	q, err := s.PluginBatchQuote("user", id)
	if err != nil {
		t.Fatal(err)
	}
	req := plugins.BatchApproveRequest{Digest: q.Digest, Count: len(q.Items), AmountMicrocredits: q.AmountMicrocredits, ExpiresAt: q.ExpiresAt, AcceptExternalBilling: true}
	if _, err = s.ApprovePluginBatch("user", id, req); err != nil {
		t.Fatal(err)
	}
	return req
}
func p06Finish(t *testing.T, s *Service, db *gorm.DB, id string) plugins.RunView {
	t.Helper()
	for i := 0; i < 20; i++ {
		p04Tick(t, s, db)
		if err := s.applicationPlugins().AdvancePipeline("user", id); err != nil {
			t.Fatal(err)
		}
		view, err := s.PluginRun("user", id)
		if err != nil {
			t.Fatal(err)
		}
		if view.Status == "succeeded" {
			return view
		}
		if view.Status == "waiting_approval" {
			p06Approve(t, s, id)
		}
	}
	t.Fatal("batch did not complete")
	return plugins.RunView{}
}

func TestP06BatchLifecycle(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s, db, release, calls := p05Fixture(t, driver, func(w http.ResponseWriter, r *http.Request) {
				var req map[string]string
				json.NewDecoder(r.Body).Decode(&req)
				json.NewEncoder(w).Encode(map[string]string{"message": req["prompt"]})
			}, "batch")
			view := p06Start(t, s, release, `[{"id":"b","prompt":"second","enabled":true},{"id":"a","prompt":"first","enabled":true},{"id":"skip","prompt":"none","enabled":false}]`)
			if view.Status != "waiting_approval" || calls.Load() != 0 {
				t.Fatalf("premature work: %+v", view)
			}
			q, err := s.PluginBatchQuote("user", view.ID)
			if err != nil || len(q.Items) != 2 || q.VendorCostKnown {
				t.Fatalf("quote=%+v %v", q, err)
			}
			bad := plugins.BatchApproveRequest{Digest: q.Digest, Count: 2, ExpiresAt: q.ExpiresAt}
			if _, err = s.ApprovePluginBatch("user", view.ID, bad); err == nil {
				t.Fatal("unknown vendor fee accepted")
			}
			bad.AcceptExternalBilling = true
			bad.ExpiresAt = time.Now().Add(-time.Minute)
			if _, err = s.ApprovePluginBatch("user", view.ID, bad); err == nil {
				t.Fatal("expired approval accepted")
			}
			if _, err = s.PluginBatchQuote("other", view.ID); err == nil {
				t.Fatal("foreign quote")
			}
			receipt := p06Approve(t, s, view.ID)
			if _, err = s.ApprovePluginBatch("user", view.ID, receipt); err != nil {
				t.Fatal("approval replay", err)
			}
			restarted := New(repository.New(db), s.dataDir)
			defer restarted.Close()
			view = p06Finish(t, restarted, db, view.ID)
			if string(view.Result) != `{"items":[{"message":"second"},{"message":"first"},null]}` || calls.Load() != 2 {
				t.Fatalf("order/result=%s calls=%d", view.Result, calls.Load())
			}
			originalRevision := view.Revision
			derived, err := s.DerivePluginRun("user", view.ID, "derive-one-change", PluginDeriveRequest{ReuseCompleted: true, Inputs: map[string]json.RawMessage{"choose": json.RawMessage(`{"items":[{"id":"a","prompt":"first changed","enabled":true},{"id":"b","prompt":"second","enabled":true}]}`)}})
			if err != nil {
				t.Fatal(err)
			}
			if derived.DerivedFromRunID != view.ID || derived.Attempt != 2 {
				t.Fatalf("lineage: %+v", derived.PluginRun)
			}
			derived = p06Finish(t, s, db, derived.ID)
			if calls.Load() != 3 || string(derived.Result) != `{"items":[{"message":"first changed"},{"message":"second"}]}` {
				t.Fatalf("reuse: %d %s", calls.Load(), derived.Result)
			}
			old, _ := s.PluginRun("user", view.ID)
			if old.Revision != originalRevision {
				t.Fatal("historical success overwritten")
			}
			random, err := s.DerivePluginRun("user", derived.ID, "derive-random-redo", PluginDeriveRequest{})
			if err != nil {
				t.Fatal(err)
			}
			p06Finish(t, s, db, random.ID)
			if calls.Load() != 5 {
				t.Fatal("implicit random cache", calls.Load())
			}
			var tasks int64
			db.Model(&model.Task{}).Where("type=?", model.TaskTypePluginOperation).Count(&tasks)
			if tasks != 5 {
				t.Fatal("duplicate task", tasks)
			}
		})
	}
}

func TestP06DuplicateKeyAndCancellation(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s, db, release, calls := p05Fixture(t, driver, nil, "batch")
			bad := p06Start(t, s, release, `[{"id":"same","prompt":"a","enabled":true},{"id":"same","prompt":"b","enabled":true}]`)
			if err := s.applicationPlugins().AdvancePipeline("user", bad.ID); err != nil {
				t.Fatal(err)
			}
			bad, _ = s.PluginRun("user", bad.ID)
			if bad.Status != "failed" || len(bad.Pipeline.StepRuns) != 0 {
				t.Fatalf("duplicate admitted %+v", bad)
			}
			view := p06Start(t, s, release, `[{"id":"a","prompt":"a","enabled":true},{"id":"b","prompt":"b","enabled":true},{"id":"c","prompt":"c","enabled":true}]`)
			p06Approve(t, s, view.ID)
			view, _ = s.PluginRun("user", view.ID)
			if _, err := s.CancelPluginRun("user", view.ID, view.Revision); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 3; i++ {
				if err := s.applicationPlugins().AdvancePipeline("user", view.ID); err != nil {
					t.Fatal(err)
				}
			}
			view, _ = s.PluginRun("user", view.ID)
			if view.Status != "cancelled" || calls.Load() != 0 {
				t.Fatalf("cancel %+v", view)
			}
			var count int64
			db.Model(&model.Task{}).Where("status=?", model.TaskStatusCancelled).Count(&count)
			if count != 2 {
				t.Fatal("inflight not cancelled", count)
			}
			unapproved := p06Start(t, s, release, `[{"id":"late","prompt":"late","enabled":true}]`)
			child := unapproved.Pipeline.StepRuns[0]
			if _, err := s.CancelPluginRun("user", unapproved.ID, unapproved.Revision); err != nil {
				t.Fatal(err)
			}
			if _, err := s.DecidePluginRun("user", child.ID, child.ApprovalID, "approve", child.Revision); err == nil {
				t.Fatal("late child approval bypassed parent cancellation")
			}
		})
	}
}

func TestP06ConcurrentSchedulerAndBudgetRollback(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s, db, release, _ := p05Fixture(t, driver, nil, "batch")
			view := p06Start(t, s, release, `[{"id":"a","prompt":"a","enabled":true},{"id":"b","prompt":"b","enabled":true}]`)
			var wg sync.WaitGroup
			errs := make(chan error, 2)
			for i := 0; i < 2; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); errs <- s.applicationPlugins().AdvancePipeline("user", view.ID) }()
			}
			wg.Wait()
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			children, err := s.repo.PluginPipelineChildren("user", view.ID)
			if err != nil || len(children) != 2 {
				t.Fatal("duplicate children", len(children), err)
			}
			// Change a prepared quote in the second child: the first enqueue must roll
			// back too, including Task and receipt, when Decide rechecks current pricing.
			if err = db.Model(&model.PluginRun{}).Where("id=?", children[1].ID).Update("source_digest", "changed").Error; err != nil {
				t.Fatal(err)
			}
			q, err := s.PluginBatchQuote("user", view.ID)
			if err != nil {
				t.Fatal(err)
			}
			_, err = s.ApprovePluginBatch("user", view.ID, PluginBatchApproveRequest{Digest: q.Digest, Count: 2, AmountMicrocredits: q.AmountMicrocredits, ExpiresAt: q.ExpiresAt, AcceptExternalBilling: true})
			if err == nil {
				t.Fatal("changed quote accepted")
			}
			var count int64
			db.Model(&model.Task{}).Where("type=?", model.TaskTypePluginOperation).Count(&count)
			if count != 0 {
				t.Fatal("partial task commit", count)
			}
			rows, err := s.repo.PluginBatchApprovals("user", view.ID)
			if err != nil || len(rows) != 0 {
				t.Fatal("partial receipt", err)
			}
		})
	}
}

func TestP06AtomicCreditReservation(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s, db, release, _ := p05Fixture(t, driver, nil, "batch")
			if err := db.Create(&model.CreditAccount{UserID: "user", AvailableMicrocredits: 150}).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := s.SavePluginOperationPrice(&model.User{ID: "admin", Role: model.UserRoleAdmin}, PluginOperationPriceInput{ReleaseID: release, OperationID: "echo", FeeMicrocredits: 100}); err != nil {
				t.Fatal(err)
			}
			view := p06Start(t, s, release, `[{"id":"a","prompt":"a","enabled":true},{"id":"b","prompt":"b","enabled":true}]`)
			q, err := s.PluginBatchQuote("user", view.ID)
			if err != nil {
				t.Fatal(err)
			}
			req := PluginBatchApproveRequest{Digest: q.Digest, Count: 2, AmountMicrocredits: 200, ExpiresAt: q.ExpiresAt, AcceptExternalBilling: true}
			if _, err = s.ApprovePluginBatch("user", view.ID, req); err == nil {
				t.Fatal("over budget accepted")
			}
			diagnostics, err := s.PluginBatchDiagnostics("user")
			if err != nil || diagnostics.BudgetConflicts != 1 {
				t.Fatal("budget diagnostics", diagnostics, err)
			}
			var account model.CreditAccount
			db.First(&account, "user_id=?", "user")
			if account.AvailableMicrocredits != 150 {
				t.Fatal("partial debit", account)
			}
			var count int64
			db.Model(&model.BillingOrder{}).Where("user_id=?", "user").Count(&count)
			if count != 0 {
				t.Fatal("partial billing", count)
			}
			if err = db.Model(&model.CreditAccount{}).Where("user_id=?", "user").Update("available_microcredits", 200).Error; err != nil {
				t.Fatal(err)
			}
			if _, err = s.ApprovePluginBatch("user", view.ID, req); err != nil {
				t.Fatal(err)
			}
			if _, err = s.ApprovePluginBatch("user", view.ID, req); err != nil {
				t.Fatal(err)
			}
			p06Finish(t, s, db, view.ID)
			db.Model(&model.BillingOrder{}).Where("user_id=? AND status=?", "user", model.BillingStatusSettled).Count(&count)
			if count != 2 {
				t.Fatal("settlement count", count)
			}
			db.First(&account, "user_id=?", "user")
			if account.AvailableMicrocredits != 0 {
				t.Fatal("duplicate debit", account)
			}
		})
	}
}

func TestP06ScopedSlotsAndFencing(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s, db, _, _ := p05Fixture(t, driver, nil, "batch")
			expires := time.Now().Add(time.Minute)
			one := model.Task{ID: "slot-one", UserID: "user", Type: model.TaskTypePluginOperation, Status: model.TaskStatusRunning, LeaseOwner: "worker-one", LeaseExpiresAt: &expires}
			two := one
			two.ID = "slot-two"
			two.LeaseOwner = "worker-two"
			for _, task := range []*model.Task{&one, &two} {
				if err := db.Create(task).Error; err != nil {
					t.Fatal(err)
				}
			}
			slot := model.PluginExecutionSlot{UserID: "user", ConnectionID: "connection", PluginID: "plugin", ModelKey: "model"}
			for _, limits := range []repository.PluginConcurrencyLimits{{Host: 1, User: 8, Connection: 8, Model: 8, Plugin: 8}, {Host: 8, User: 1, Connection: 8, Model: 8, Plugin: 8}, {Host: 8, User: 8, Connection: 1, Model: 8, Plugin: 8}, {Host: 8, User: 8, Connection: 8, Model: 1, Plugin: 8}, {Host: 8, User: 8, Connection: 8, Model: 8, Plugin: 1}} {
				ok, err := s.repo.AcquirePluginExecutionSlot(one, slot, limits)
				if err != nil || !ok {
					t.Fatal("first slot", ok, err)
				}
				ok, err = s.repo.AcquirePluginExecutionSlot(two, slot, limits)
				if err != nil || ok {
					t.Fatal("scope cap", ok, err)
				}
				if err = s.repo.ReleasePluginExecutionSlot(one.ID, one.LeaseOwner); err != nil {
					t.Fatal(err)
				}
			}
			limits := repository.PluginConcurrencyLimits{Host: 1, User: 1, Connection: 1, Model: 1, Plugin: 1}
			var wg sync.WaitGroup
			results := make(chan bool, 2)
			errs := make(chan error, 2)
			for _, task := range []model.Task{one, two} {
				wg.Add(1)
				go func(task model.Task) {
					defer wg.Done()
					ok, err := s.repo.AcquirePluginExecutionSlot(task, slot, limits)
					results <- ok
					errs <- err
				}(task)
			}
			wg.Wait()
			close(results)
			close(errs)
			count := 0
			for ok := range results {
				if ok {
					count++
				}
			}
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			if count != 1 {
				t.Fatal("dual worker limit", count)
			}
			if err := db.Model(&model.Task{}).Where("id IN ?", []string{one.ID, two.ID}).Update("lease_expires_at", time.Now().Add(-time.Minute)).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&model.Task{}).Where("id=?", one.ID).Updates(map[string]any{"lease_owner": "new-owner", "lease_expires_at": expires}).Error; err != nil {
				t.Fatal(err)
			}
			if ok, err := s.repo.AcquirePluginExecutionSlot(one, slot, limits); err == nil || ok {
				t.Fatal("stale acquired")
			}
			current := one
			current.LeaseOwner = "new-owner"
			if ok, err := s.repo.AcquirePluginExecutionSlot(current, slot, limits); err != nil || !ok {
				t.Fatal("expired slots not recovered", err)
			}
			if err := s.repo.ReleasePluginExecutionSlot(one.ID, one.LeaseOwner); err != nil {
				t.Fatal(err)
			}
			d, err := s.PluginBatchDiagnostics("user")
			if err != nil || d.ActiveSlots != 1 {
				t.Fatal("stale release removed current slot", d, err)
			}
			if err = db.Model(&model.Task{}).Where("id=?", one.ID).Updates(map[string]any{"lease_owner": "", "lease_expires_at": nil}).Error; err != nil {
				t.Fatal(err)
			}
			d, err = s.PluginBatchDiagnostics("user")
			if err != nil || d.ActiveSlots != 0 {
				t.Fatal("polling retains execution slot", d, err)
			}
		})
	}
}

func TestP06UnknownAndFailedSibling(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s, db, release, calls := p05Fixture(t, driver, func(w http.ResponseWriter, r *http.Request) { panic(http.ErrAbortHandler) }, "batch")
			view := p06Start(t, s, release, `[{"id":"a","prompt":"a","enabled":true}]`)
			p06Approve(t, s, view.ID)
			p04Tick(t, s, db)
			for i := 0; i < 3; i++ {
				if err := s.applicationPlugins().AdvancePipeline("user", view.ID); err != nil {
					t.Fatal(err)
				}
				p04Tick(t, s, db)
			}
			if calls.Load() != 1 {
				t.Fatal("unknown resubmitted", calls.Load())
			}
			d, err := s.PluginBatchDiagnostics("user")
			if err != nil || d.Unknown != 1 {
				t.Fatal("unknown metric", d, err)
			}
			if _, err = s.DerivePluginRun("user", view.ID, "unknown-derived", PluginDeriveRequest{}); err == nil {
				t.Fatal("unknown bypassed by derive")
			}
			view, _ = s.PluginRun("user", view.ID)
			if _, err = s.CancelPluginRun("user", view.ID, view.Revision); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 4; i++ {
				if err = s.applicationPlugins().AdvancePipeline("user", view.ID); err != nil {
					t.Fatal(err)
				}
				p04Tick(t, s, db)
			}
			if _, err = s.DerivePluginRun("user", view.ID, "unknown-cancel-derived", PluginDeriveRequest{}); err == nil {
				t.Fatal("unconfirmed external cancel bypassed")
			}
		})
	}
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run("failure-"+driver, func(t *testing.T) {
			s, db, release, calls := p05Fixture(t, driver, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(400) }, "batch")
			view := p06Start(t, s, release, `[{"id":"a","prompt":"a","enabled":true},{"id":"b","prompt":"b","enabled":true},{"id":"c","prompt":"c","enabled":true}]`)
			p06Approve(t, s, view.ID)
			p04Tick(t, s, db)
			for i := 0; i < 3; i++ {
				if err := s.applicationPlugins().AdvancePipeline("user", view.ID); err != nil {
					t.Fatal(err)
				}
			}
			view, _ = s.PluginRun("user", view.ID)
			if view.Status != "failed" || calls.Load() != 1 || len(view.Pipeline.StepRuns) != 2 {
				t.Fatalf("sibling failure %+v calls=%d", view.PluginRun, calls.Load())
			}
		})
	}
}
func TestP06RevocationStopsNewWave(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s, db, release, calls := p05Fixture(t, driver, nil, "batch")
			view := p06Start(t, s, release, `[{"id":"a","prompt":"a","enabled":true},{"id":"b","prompt":"b","enabled":true},{"id":"c","prompt":"c","enabled":true}]`)
			p06Approve(t, s, view.ID)
			install, err := s.repo.UserPluginState("user", "batch-helper")
			if err != nil {
				t.Fatal(err)
			}
			if err = s.ActivateApplicationPlugin(&model.User{ID: "user", Role: model.UserRoleUser}, "batch-helper", ApplicationPluginActivation{ReleaseID: release, Revision: install.Revision, Enabled: false}); err != nil {
				t.Fatal(err)
			}
			p04Tick(t, s, db)
			p04Tick(t, s, db)
			if err = s.applicationPlugins().AdvancePipeline("user", view.ID); err != nil {
				t.Fatal(err)
			}
			view, _ = s.PluginRun("user", view.ID)
			if view.Status != "paused" || len(view.Pipeline.StepRuns) != 2 || calls.Load() != 2 {
				t.Fatalf("revoked admitted next wave %+v calls=%d", view.PluginRun, calls.Load())
			}
		})
	}
}
