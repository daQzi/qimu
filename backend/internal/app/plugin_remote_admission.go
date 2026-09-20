package app

import (
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/repository"
	"net/url"
	"sort"
	"strings"
	"time"
)

type pluginRemoteHost struct{ svc *Service }
type pluginRemotePrepared struct {
	RunID            string            `json:"runId"`
	ReleaseID        string            `json:"releaseId"`
	Operation        string            `json:"operation"`
	ConnectorID      string            `json:"connectorId"`
	ActionID         string            `json:"actionId"`
	ResourceIDs      map[string]string `json:"resourceIds"`
	ResourceVersions map[string]string `json:"resourceVersions"`
}

func (h pluginRemoteHost) Prepare(repo *repository.Repository, userID string, request contracts.Invocation, ctx plugins.HostOperationContext, policy plugins.InvocationPolicy) (plugins.PreparedRemoteOperation, error) {
	parts := strings.Split(request.Operation, ".")
	if len(parts) != 2 {
		return plugins.PreparedRemoteOperation{}, BadAuthRequest("远程操作名称无效")
	}
	var manifest contracts.Manifest
	if err := json.Unmarshal(ctx.Files["manifest.json"], &manifest); err != nil {
		return plugins.PreparedRemoteOperation{}, err
	}
	var op contracts.Operation
	for _, ref := range manifest.Contributes.Operations {
		if ref.ID == parts[1] {
			if err := json.Unmarshal(ctx.Files[ref.Ref], &op); err != nil {
				return plugins.PreparedRemoteOperation{}, err
			}
		}
	}
	connector, err := contracts.ReadHTTPConnector(ctx.Files, manifest, op.Execution.Connector)
	if err != nil {
		return plugins.PreparedRemoteOperation{}, err
	}
	action, ok := connector.Actions[op.Execution.Action]
	if !ok {
		return plugins.PreparedRemoteOperation{}, BadAuthRequest("远程动作不存在")
	}
	connection, err := repo.PluginConnection(userID, parts[0], connector.ID)
	if err != nil {
		return plugins.PreparedRemoteOperation{}, err
	}
	if connection == nil || !connection.Enabled {
		return plugins.PreparedRemoteOperation{}, &kernel.AppError{Status: 409, Code: 409, Reason: "connection_unconfigured", Message: "请先配置并启用插件连接"}
	}
	version, err := repo.PluginConnectionVersion(userID, connection.VersionID)
	if err != nil {
		return plugins.PreparedRemoteOperation{}, err
	}
	resources := map[string]string{}
	resourceVersions := map[string]string{}
	sourceParts := []string{request.Operation, version.ID}
	fields := make([]string, 0, len(action.Resources))
	for field := range action.Resources {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	for _, field := range fields {
		kind := action.Resources[field]
		var id string
		if raw, exists := request.Input[field]; !exists || json.Unmarshal(raw, &id) != nil || id == "" {
			return plugins.PreparedRemoteOperation{}, BadAuthRequest("远程操作缺少资源：" + field)
		}
		resource, err := repo.LockResourceForUser(userID, id)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return plugins.PreparedRemoteOperation{}, Forbidden("资源不存在或不属于当前用户")
		}
		if err != nil {
			return plugins.PreparedRemoteOperation{}, err
		}
		if resource.Status != model.ResourceStatusReady || (kind != "" && resource.Kind != kind) {
			return plugins.PreparedRemoteOperation{}, BadAuthRequest("资源尚未就绪或类型不匹配：" + field)
		}
		resources[field] = id
		resourceVersions[field] = resource.UpdatedAt.UTC().Format(time.RFC3339Nano)
		sourceParts = append(sourceParts, field, id, resource.UpdatedAt.UTC().Format(time.RFC3339Nano))
	}
	price, err := repo.PluginOperationPrice(ctx.ReleaseID, parts[1])
	if err != nil {
		return plugins.PreparedRemoteOperation{}, err
	}
	preview, _ := json.Marshal(map[string]any{"operation": request.Operation, "connection": connection.Name, "targetHost": connectionHost(version.BaseURL), "platformFeeMicrocredits": price.FeeMicrocredits, "vendorBilling": "external_byok", "resourceCount": len(resources), "resources": resources, "priceRevision": price.Revision, "idempotency": action.Idempotency.Mode, "cancellation": action.Cancellation.Mode, "lookupSupported": action.Lookup != nil, "asynchronous": action.Poll != nil})
	digest := hashStrings(append(sourceParts, string(preview))...)
	prepared := pluginRemotePrepared{ReleaseID: ctx.ReleaseID, Operation: request.Operation, ConnectorID: connector.ID, ActionID: op.Execution.Action, ResourceIDs: resources, ResourceVersions: resourceVersions}
	return plugins.PreparedRemoteOperation{Preview: preview, SourceDigest: digest, ConnectionVersionID: version.ID, ResourceIDs: mapStringValues(resources), Enqueue: func(tx *repository.Repository, run *model.PluginRun) error {
		prepared.RunID = run.ID
		return h.enqueue(tx, userID, run, request, prepared, price.FeeMicrocredits, policy)
	}}, nil
}
func connectionHost(raw string) string { u, _ := url.Parse(raw); return u.Hostname() }
func mapStringValues(values map[string]string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}
func hashStrings(values ...string) string {
	raw, _ := json.Marshal(values)
	return creationHashRaw(string(raw))
}

func (h pluginRemoteHost) enqueue(repo *repository.Repository, userID string, run *model.PluginRun, request contracts.Invocation, prepared pluginRemotePrepared, fee int64, policy plugins.InvocationPolicy) error {
	if run.TaskID != nil {
		return nil
	}
	if run.AgentRunID != "" && fee > 0 {
		agent, err := repo.LockPluginAgentBudget(userID, run.AgentRunID)
		if err != nil {
			return err
		}
		state, err := cloudAgentDecode(agent)
		if err != nil {
			return err
		}
		remaining, err := pluginAgentRemaining(repo, userID, run.AgentRunID, state)
		if err != nil {
			return err
		}
		if fee > remaining {
			return &kernel.AppError{Status: 400, Code: 400, Reason: "plugin_budget_exceeded", Message: "Agent 剩余预算不足以批准此插件任务"}
		}
	}
	payload, _ := json.Marshal(map[string]any{"runId": run.ID})
	task := &model.Task{ID: newID(), UserID: userID, ProjectID: valueOrEmpty(request.Context, func(c *contracts.InvocationContext) string { return c.CanvasID }), Type: model.TaskTypePluginOperation, Status: model.TaskStatusQueued, Stage: "等待插件任务执行", Prompt: "插件远程操作", Operation: "plugin_operation", Provider: "application-plugin", Model: prepared.Operation, InputJSON: string(payload), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	var order *model.BillingOrder
	task.PluginRunID = &run.ID
	task.AgentRunID = run.AgentRunID
	task.ApprovalID = run.ApprovalID
	task.AuthorizedChargeMicrocredits = fee
	if fee > 0 {
		order = &model.BillingOrder{ID: newID(), UserID: userID, IdempotencyKey: "plugin:" + run.ID, TaskID: task.ID, ChannelID: "application-plugin", Model: prepared.Operation, Capability: "plugin_operation", Scene: "plugin_operation", BillingMode: "fixed_request", UnitPriceMicrocredits: fee, Quantity: 1, AmountMicrocredits: fee, ReservedAmountMicrocredits: fee, Status: model.BillingStatusReserved, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		task.BillingOrderID = order.ID
		order.ChargeLimitMicrocredits = fee
		order.ChargeLimitSet = true
	}
	runtime, err := (&Service{repo: repo}).RuntimePolicy()
	if err != nil {
		return err
	}
	if err = createTaskWithStorageQuotaRepository(repo, task, order, runtime); err != nil {
		if errors.Is(err, repository.ErrInsufficientCredits) {
			return &kernel.AppError{Status: 400, Code: 400, Reason: "plugin_budget_exceeded", Message: "积分不足，请先使用兑换码充值"}
		}
		return err
	}
	preparedRaw, _ := json.Marshal(prepared)
	cipher, err := h.svc.encryptSettingSecret(string(preparedRaw))
	if err != nil {
		return err
	}
	execution := &model.PluginRemoteExecution{TaskID: task.ID, RunID: run.ID, UserID: userID, ConnectionVersionID: run.ConnectionVersionID, SubmissionKey: "plugin-" + run.ID, Attempt: max(1, run.Attempt), State: "prepared", PreparedCipher: cipher, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err = repo.CreatePluginRemote(execution); err != nil {
		return err
	}
	run.TaskID = &task.ID
	return nil
}
func valueOrEmpty[T any](value *T, get func(*T) string) string {
	if value == nil {
		return ""
	}
	return get(value)
}

func (h pluginRemoteHost) Cancel(repo *repository.Repository, run *model.PluginRun) error {
	remote, err := repo.PluginRemoteForRun(run.UserID, run.ID)
	if err != nil {
		return err
	}
	task, err := repo.LockPluginTask(run.UserID, remote.TaskID)
	if err != nil {
		return err
	}
	if task.Status == model.TaskStatusQueued && remote.State == "prepared" {
		now := time.Now()
		task.Status = model.TaskStatusCancelled
		task.Stage = "插件任务已取消"
		task.CompletedAt = &now
		if err = repo.SavePluginTask(task); err != nil {
			return err
		}
		if task.BillingOrderID != "" {
			if err = repo.RefundBillingOrder(task.BillingOrderID, "插件任务在提交前取消"); err != nil {
				return err
			}
		}
		run.Status = "cancelled"
		return repo.ReleasePluginInputResources(run.UserID, run.ID)
	}
	remote.CancelRequested = true
	remote.UpdatedAt = time.Now()
	if task.Status == model.TaskStatusPaused {
		task.Status = model.TaskStatusQueued
		task.NextPollAt = nil
		if err = repo.SavePluginTask(task); err != nil {
			return err
		}
	}
	if err = repo.SavePluginRemote(remote); err != nil {
		return err
	}
	run.Status = "cancelling"
	return nil
}
func (h pluginRemoteHost) Resume(repo *repository.Repository, run *model.PluginRun, request plugins.RemoteResumeRequest) error {
	action := request.Action
	jobID := strings.TrimSpace(request.ProviderJobID)
	remote, err := repo.PluginRemoteForRun(run.UserID, run.ID)
	if err != nil {
		return err
	}
	task, err := repo.LockPluginTask(run.UserID, remote.TaskID)
	if err != nil {
		return err
	}
	if run.Status != "paused" || task.Status != model.TaskStatusPaused {
		return creationConflict("运行当前不可恢复")
	}
	runtime, err := h.svc.loadPluginRemoteRuntime(run.UserID, task.ID)
	if err != nil {
		return err
	}
	switch action {
	case "attach_job":
		if remote.State != "unknown" || !validPluginJobID(jobID) || runtime.Action.Poll == nil {
			return BadAuthRequest("需要为提交不明的运行填写真实上游任务 ID")
		}
		remote.ProviderJobID = jobID
		remote.State = "submitted"
		task.ProviderRequestID = jobID
	case "retry_safe":
		if remote.State != "unknown" || (!pluginReplayAllowed(runtime) && runtime.Action.Lookup == nil) {
			return BadAuthRequest("该连接不能安全自动重试提交")
		}
		remote.State = "submitting"
	case "retry_poll":
		if remote.ProviderJobID == "" {
			return BadAuthRequest("没有可查询的上游任务")
		}
		remote.State = "submitted"
	case "retry_import":
		if remote.State != "import_pending" {
			return BadAuthRequest("当前没有待重试的导入")
		}
	default:
		return BadAuthRequest("未知恢复动作")
	}
	remote.PollFailures = 0
	remote.ImportAttempts = 0
	now := time.Now()
	remote.FailureReason = ""
	remote.FailureMessage = ""
	remote.UpdatedAt = now
	task.Status = model.TaskStatusQueued
	task.Stage = "插件任务已恢复"
	task.Error = ""
	task.CompletedAt = nil
	task.NextPollAt = nil
	task.LeaseOwner = ""
	task.LeaseExpiresAt = nil
	run.Status = "running"
	run.FailureMessage = ""
	if err = repo.SavePluginRemote(remote); err != nil {
		return err
	}
	if err = repo.SavePluginTask(task); err != nil {
		return err
	}
	if err = repo.ResumePluginBilling(task.BillingOrderID); err != nil {
		return err
	}
	return nil
}
