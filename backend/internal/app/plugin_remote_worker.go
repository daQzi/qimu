package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/repository"
)

const (
	pluginRemoteResponseLimit = 256 << 10
	pluginRemoteRequestLimit  = 256 << 10
	pluginRemotePollDelay     = 5 * time.Second
)

type pluginRemoteRuntime struct {
	Run        model.PluginRun
	Remote     model.PluginRemoteExecution
	Prepared   pluginRemotePrepared
	Connection model.PluginConnectionVersion
	Connector  contracts.HTTPConnector
	Action     contracts.HTTPAction
	Operation  contracts.Operation
	Input      map[string]json.RawMessage
	Files      contracts.PackageFiles
}
type pluginHTTPError struct {
	status int
}

func (e *pluginHTTPError) Error() string {
	return fmt.Sprintf("远程服务返回 HTTP %d", e.status)
}

type pluginAmbiguousError struct{ cause error }

func (e *pluginAmbiguousError) Error() string {
	return "远程提交结果不明，请查询原请求或人工核实"
}
func (e *pluginAmbiguousError) Unwrap() error { return e.cause }

func (w *taskWorkerCoordinator) processPluginOperation(task *model.Task, ctx context.Context) error {
	runtime, err := w.service.loadPluginRemoteRuntime(task.UserID, task.ID)
	if err != nil {
		return w.pausePluginState(task, "configuration_unavailable", "固定版本配置暂不可用，请修复配置后恢复原任务")
	}
	acquired, err := w.service.acquirePluginSlot(task, runtime)
	if err != nil {
		return err
	}
	if !acquired {
		return w.deferPluginTask(task, "等待插件并发槽", runtime.Remote.State, time.Second, nil)
	}
	defer func() {
		if err := w.service.repo.ReleasePluginExecutionSlot(task.ID, task.LeaseOwner); err != nil {
			log.Printf("plugin slot release failed: task=%s error=%v", task.ID, err)
		}
	}()
	if runtime.Remote.CancelRequested {
		return w.cancelPluginRemote(task, ctx, runtime)
	}
	switch runtime.Remote.State {
	case "prepared":
		body, buildErr := w.service.buildPluginRequestBody(task.UserID, &runtime)
		if buildErr != nil {
			return w.failPluginTask(task, "远程请求准备失败", errors.New("无法准备批准的资源或参数，请检查资源与公网访问配置"))
		}
		cipher, markErr := w.service.markPluginSubmitting(*task, body)
		if markErr != nil {
			return markErr
		}
		runtime.Remote.RequestCipher = cipher
		now := time.Now()
		runtime.Remote.FirstSubmittedAt = &now
		runtime.Remote.State = "submitting"
		return w.submitPluginRemote(task, ctx, runtime, false)
	case "submitting":
		if runtime.Action.Lookup != nil {
			return w.submitPluginRemote(task, ctx, runtime, true)
		}
		if pluginReplayAllowed(runtime) {
			return w.submitPluginRemote(task, ctx, runtime, false)
		}
		return w.pausePluginUnknown(task, errors.New("Worker 恢复时发现提交可能已发送，且上游不支持幂等或按提交键查询"))
	case "submitted", "polling":
		return w.pollPluginRemote(task, ctx, runtime)
	case "upstream_completed", "import_pending":
		return w.importPluginRemote(task, ctx, runtime)
	case "cancel_requested":
		return w.cancelPluginRemote(task, ctx, runtime)
	default:
		return w.failPluginTask(task, "插件任务状态无效", fmt.Errorf("unsupported plugin remote state %q", runtime.Remote.State))
	}
}

func (s *Service) loadPluginRemoteRuntime(userID, taskID string) (pluginRemoteRuntime, error) {
	remote, err := s.repo.PluginRemoteTask(taskID)
	if err != nil {
		return pluginRemoteRuntime{}, err
	}
	run, err := s.repo.PluginRunForUser(userID, remote.RunID)
	if err != nil {
		return pluginRemoteRuntime{}, err
	}
	version, err := s.repo.PluginConnectionVersion(userID, remote.ConnectionVersionID)
	if err != nil {
		return pluginRemoteRuntime{}, err
	}
	release, err := s.repo.PluginRelease(run.ReleaseID)
	if err != nil {
		return pluginRemoteRuntime{}, err
	}
	files, err := s.applicationPlugins().LoadReleasePackage(*release)
	if err != nil {
		return pluginRemoteRuntime{}, err
	}
	var manifest contracts.Manifest
	if err = json.Unmarshal(files["manifest.json"], &manifest); err != nil {
		return pluginRemoteRuntime{}, err
	}
	var operation contracts.Operation
	parts := strings.Split(run.Operation, ".")
	if len(parts) != 2 {
		return pluginRemoteRuntime{}, errors.New("invalid plugin operation address")
	}
	for _, ref := range manifest.Contributes.Operations {
		if ref.ID == parts[1] {
			if err = json.Unmarshal(files[ref.Ref], &operation); err != nil {
				return pluginRemoteRuntime{}, err
			}
		}
	}
	connector, err := contracts.ReadHTTPConnector(files, manifest, operation.Execution.Connector)
	if err != nil {
		return pluginRemoteRuntime{}, err
	}
	action, ok := connector.Actions[operation.Execution.Action]
	if !ok {
		return pluginRemoteRuntime{}, errors.New("connector action missing")
	}
	preparedRaw, err := s.decryptSettingSecret(remote.PreparedCipher)
	if err != nil {
		return pluginRemoteRuntime{}, err
	}
	var prepared pluginRemotePrepared
	if err = json.Unmarshal([]byte(preparedRaw), &prepared); err != nil {
		return pluginRemoteRuntime{}, err
	}
	var invocation contracts.Invocation
	if err = json.Unmarshal([]byte(run.RequestJSON), &invocation); err != nil {
		return pluginRemoteRuntime{}, err
	}
	return pluginRemoteRuntime{Run: *run, Remote: *remote, Prepared: prepared, Connection: *version, Connector: connector, Action: action, Operation: operation, Input: invocation.Input, Files: files}, nil
}

func pluginReplayAllowed(runtime pluginRemoteRuntime) bool {
	return runtime.Action.Idempotency.Mode == "header" && runtime.Remote.FirstSubmittedAt != nil && time.Since(*runtime.Remote.FirstSubmittedAt) < time.Duration(runtime.Action.Idempotency.RetentionSeconds)*time.Second
}
func (s *Service) buildPluginRequestBody(userID string, runtime *pluginRemoteRuntime) ([]byte, error) {
	payload := map[string]any{}
	for key, binding := range runtime.Action.Submit.Body {
		value, err := s.pluginHTTPBinding(userID, runtime, binding)
		if err != nil {
			return nil, err
		}
		payload[key] = value
	}
	raw, err := json.Marshal(payload)
	if len(raw) > pluginRemoteRequestLimit {
		return nil, errors.New("请求体超限")
	}
	return raw, err
}
func (s *Service) markPluginSubmitting(task model.Task, body []byte) (string, error) {
	cipher, err := s.encryptSettingSecret(string(body))
	if err != nil {
		return "", err
	}
	err = s.repo.WithPluginTask(task, func(repo *repository.Repository, current *model.Task, remote *model.PluginRemoteExecution, _ *model.PluginRun) error {
		if remote.State != "prepared" || remote.CancelRequested {
			return repository.ErrTaskStateConflict
		}
		now := time.Now()
		remote.State = "submitting"
		remote.FirstSubmittedAt = &now
		remote.RequestCipher = cipher
		remote.UpdatedAt = now
		current.Stage = "提交远程请求"
		current.Progress = 0
		if current.BillingOrderID != "" {
			if err := repo.MarkBillingRunning(current.BillingOrderID); err != nil {
				return err
			}
		}
		if err := repo.SavePluginRemote(remote); err != nil {
			return err
		}
		return repo.SavePluginTask(current)
	})
	return cipher, err
}
func (w *taskWorkerCoordinator) submitPluginRemote(task *model.Task, ctx context.Context, runtime pluginRemoteRuntime, lookup bool) error {
	request := runtime.Action.Submit
	if lookup {
		request = *runtime.Action.Lookup
	}
	response, err := w.service.pluginHTTPRequest(ctx, task, &runtime, request, map[string]string{"submissionKey": runtime.Remote.SubmissionKey}, !lookup)
	if err != nil {
		var httpErr *pluginHTTPError
		if !lookup && errors.As(err, &httpErr) && (httpErr.status == 400 || httpErr.status == 401 || httpErr.status == 403 || httpErr.status == 404 || httpErr.status == 422) {
			return w.failPluginTask(task, "远程提交被拒绝", err)
		}
		if lookup && errors.As(err, &httpErr) && httpErr.status == 404 {
			return w.pausePluginUnknown(task, errors.New("按提交键查询未找到任务，不能据此推断从未提交"))
		}
		if lookup || pluginReplayAllowed(runtime) {
			return w.deferPluginTask(task, "等待安全重试", runtime.Remote.State, pluginRemotePollDelay, err)
		}
		return w.pausePluginUnknown(task, err)
	}
	if runtime.Action.JobIDPath == "" {
		if err = w.service.storePluginResponse(*task, response, "upstream_completed", ""); err != nil {
			return err
		}
		return w.importPluginRemote(task, ctx, runtime)
	}
	jobIDValue, ok := pluginJSONPointer(response, runtime.Action.JobIDPath)
	jobID, _ := jobIDValue.(string)
	if !ok || !validPluginJobID(jobID) {
		return w.pausePluginUnknown(task, errors.New("提交响应缺少有效任务 ID"))
	}
	return w.deferPluginTaskWithResponse(task, "等待远程任务", response, "submitted", jobID, pluginRemotePollDelay)
}
func (w *taskWorkerCoordinator) pollPluginRemote(task *model.Task, ctx context.Context, runtime pluginRemoteRuntime) error {
	if runtime.Action.Poll == nil || runtime.Remote.ProviderJobID == "" {
		return w.failPluginTask(task, "远程轮询配置无效", errors.New("missing poll request or provider job id"))
	}
	response, err := w.service.pluginHTTPRequest(ctx, task, &runtime, *runtime.Action.Poll, map[string]string{"jobId": runtime.Remote.ProviderJobID}, false)
	if err != nil {
		return w.deferPluginTask(task, "远程状态查询失败，稍后重试", "polling", pluginRemotePollDelay, err)
	}
	statusValue, ok := pluginJSONPointer(response, runtime.Action.StatusPath)
	if !ok {
		return w.pausePluginState(task, "upstream_output_invalid", "轮询响应缺少状态，保留原任务等待核实")
	}
	status, known := runtime.Action.StatusMap[fmt.Sprint(statusValue)]
	if !known {
		return w.pausePluginState(task, "upstream_output_invalid", "上游返回未知状态，保留原任务等待核实")
	}
	switch status {
	case "pending":
		return w.deferPluginTaskWithResponse(task, "远程任务执行中", response, "polling", runtime.Remote.ProviderJobID, pluginRemotePollDelay)
	case "succeeded":
		if runtime.Remote.CancelRequested {
			return w.finishPluginCancellation(task, "completed_after_cancel")
		}
		if err = w.service.storePluginResponse(*task, response, "upstream_completed", runtime.Remote.ProviderJobID); err != nil {
			return err
		}
		return w.importPluginRemote(task, ctx, runtime)
	case "failed":
		return w.failPluginTask(task, "远程任务失败", errors.New("远程服务报告任务失败"))
	case "cancelled":
		return w.finishPluginCancellation(task, "confirmed")
	default:
		return w.failPluginTask(task, "远程响应无效", errors.New("状态映射无效"))
	}
}

func (s *Service) pluginHTTPRequest(ctx context.Context, task *model.Task, runtime *pluginRemoteRuntime, spec contracts.HTTPRequest, vars map[string]string, submit bool) (json.RawMessage, error) {
	if err := s.waitPluginRequestSlot(ctx, *task, runtime.Connection.ConnectionID); err != nil {
		return nil, err
	}
	path := spec.Path
	for key, value := range vars {
		path = strings.ReplaceAll(path, "{"+key+"}", url.PathEscape(value))
	}
	target := strings.TrimSuffix(runtime.Connection.BaseURL, "/") + path
	if _, err := ValidateCustomRelayURL(target); err != nil {
		return nil, err
	}
	var body io.Reader
	if submit {
		if runtime.Remote.RequestCipher == "" {
			return nil, errors.New("缺少持久化提交正文")
		}
		raw, err := s.decryptSettingSecret(runtime.Remote.RequestCipher)
		if err != nil {
			return nil, errors.New("无法解密提交正文")
		}
		body = strings.NewReader(raw)
	}
	if submit {
		if err := s.repo.WithPluginTask(*task, func(_ *repository.Repository, _ *model.Task, remote *model.PluginRemoteExecution, _ *model.PluginRun) error {
			if remote.CancelRequested {
				return repository.ErrTaskStateConflict
			}
			return nil
		}); err != nil {
			return nil, err
		}
	}

	req, err := http.NewRequestWithContext(ctx, spec.Method, target, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	ApplyDefaultOutboundHeaders(req)
	secret, err := s.decryptSettingSecret(runtime.Connection.SecretCipher)
	if err != nil {
		return nil, err
	}
	switch runtime.Connector.Auth.Type {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+secret)
	case "header":
		req.Header.Set(runtime.Connector.Auth.Header, secret)
	}
	if submit && runtime.Action.Idempotency.Mode == "header" {
		req.Header.Set(runtime.Action.Idempotency.Header, runtime.Remote.SubmissionKey)
	}
	response, err := CustomRelayHTTPClient(30 * time.Second).Do(req)
	if err != nil {
		if submit {
			return nil, &pluginAmbiguousError{cause: err}
		}
		return nil, errors.New("远程查询暂不可用")
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, pluginRemoteResponseLimit+1))
	if err != nil {
		if submit {
			return nil, &pluginAmbiguousError{cause: err}
		}
		return nil, err
	}
	if len(raw) > pluginRemoteResponseLimit {
		return nil, errors.New("远程响应超过 256KB")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &pluginHTTPError{status: response.StatusCode}
	}
	value, err := contracts.Decode(raw)
	if err != nil {
		return nil, errors.New("远程服务未返回有效 JSON")
	}
	normalized, err := json.Marshal(value)
	return normalized, err
}
func (s *Service) pluginHTTPBinding(userID string, runtime *pluginRemoteRuntime, binding contracts.Binding) (any, error) {
	if len(binding.Literal) > 0 {
		var value any
		if err := json.Unmarshal(binding.Literal, &value); err != nil {
			return nil, err
		}
		return value, nil
	}
	if strings.HasPrefix(binding.From, "input.") {
		raw, ok := runtime.Input[strings.TrimPrefix(binding.From, "input.")]
		if !ok {
			return nil, BadAuthRequest("远程映射缺少输入")
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		return value, nil
	}
	if strings.HasPrefix(binding.From, "resource.") {
		field := strings.TrimPrefix(binding.From, "resource.")
		id := runtime.Prepared.ResourceIDs[field]
		if id == "" {
			return nil, BadAuthRequest("远程映射缺少资源")
		}
		resource, err := s.repo.ResourceForUser(userID, id)
		if err != nil {
			return nil, err
		}
		if resource.UpdatedAt.UTC().Format(time.RFC3339Nano) != runtime.Prepared.ResourceVersions[field] {
			return nil, errors.New("批准的资源已变化")
		}
		return s.DirectResourceURL(userID, id)
	}
	return nil, BadAuthRequest("远程请求映射无效")
}
func (s *Service) waitPluginRequestSlot(ctx context.Context, task model.Task, connectionID string) error {
	for {
		var allowed bool
		var next time.Time
		err := s.repo.WithPluginTask(task, func(repo *repository.Repository, _ *model.Task, _ *model.PluginRemoteExecution, _ *model.PluginRun) error {
			var err error
			allowed, next, err = repo.AllowPluginRequest(connectionID)
			return err
		})
		if err != nil {
			return err
		}
		if allowed {
			return nil
		}
		timer := time.NewTimer(time.Until(next))
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (s *Service) storePluginResponse(task model.Task, response json.RawMessage, state, jobID string) error {
	cipher, err := s.encryptSettingSecret(string(response))
	if err != nil {
		return err
	}
	sum := sha256.Sum256(response)
	return s.repo.WithPluginTask(task, func(repo *repository.Repository, current *model.Task, remote *model.PluginRemoteExecution, _ *model.PluginRun) error {
		remote.ResponseCipher = cipher
		remote.ResponseDigest = hex.EncodeToString(sum[:])
		remote.State = state
		if jobID != "" {
			remote.ProviderJobID = jobID
			current.ProviderRequestID = jobID
		}
		remote.UpdatedAt = time.Now()
		current.Stage = "远程结果待导入"
		current.Progress = 0
		if err = repo.SavePluginRemote(remote); err != nil {
			return err
		}
		return repo.SavePluginTask(current)
	})
}
func (w *taskWorkerCoordinator) deferPluginTask(task *model.Task, stage, state string, delay time.Duration, cause error) error {
	return w.deferPluginTaskWithResponse(task, stage, nil, state, "", delay, cause)
}
func (w *taskWorkerCoordinator) deferPluginTaskWithResponse(task *model.Task, stage string, response json.RawMessage, state, jobID string, delay time.Duration, causes ...error) error {
	s := w.service
	var cipher, digest string
	if len(response) > 0 {
		var err error
		cipher, err = s.encryptSettingSecret(string(response))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(response)
		digest = hex.EncodeToString(sum[:])
	}
	return s.repo.WithPluginTask(*task, func(repo *repository.Repository, current *model.Task, remote *model.PluginRemoteExecution, run *model.PluginRun) error {
		remote.State = state
		if jobID != "" {
			remote.ProviderJobID = jobID
			current.ProviderRequestID = jobID
		}
		if cipher != "" {
			remote.ResponseCipher = cipher
			remote.ResponseDigest = digest
		}
		if len(causes) > 0 && causes[0] != nil {
			remote.FailureMessage = "远程请求失败，保留原提交身份"
			remote.PollFailures++
		}
		remote.UpdatedAt = time.Now()
		current.Stage = stage
		current.Progress = 0
		current.NextPollAt = ptrPluginTime(time.Now().Add(delay))
		if remote.PollFailures >= 3 {
			current.Status = model.TaskStatusPaused
			current.NextPollAt = nil
			run.Status = "paused"
			run.FailureMessage = "连续查询失败，保留原任务，可恢复查询"
			if remote.State == "submitting" {
				remote.State = "unknown"
				remote.FailureReason = "upstream_submission_unknown"
			}
			if err := plugins.SaveRunTransition(repo, run, "run.paused"); err != nil {
				return err
			}
		}
		current.LeaseOwner = ""
		current.LeaseExpiresAt = nil
		if err := repo.SavePluginRemote(remote); err != nil {
			return err
		}
		return repo.SavePluginTask(current)
	})
}
func ptrPluginTime(value time.Time) *time.Time { return &value }
func (w *taskWorkerCoordinator) pausePluginUnknown(task *model.Task, cause error) error {
	s := w.service
	return s.repo.WithPluginTask(*task, func(repo *repository.Repository, current *model.Task, remote *model.PluginRemoteExecution, run *model.PluginRun) error {
		remote.State = "unknown"
		remote.FailureReason = "upstream_submission_unknown"
		remote.FailureMessage = "远程调用未完成，需要核实或恢复"
		remote.UpdatedAt = time.Now()
		current.Status = model.TaskStatusPaused
		current.Stage = "远程提交待核实"
		current.Error = remote.FailureMessage
		current.LeaseOwner = ""
		current.LeaseExpiresAt = nil
		current.NextPollAt = nil
		run.Status = "paused"
		run.FailureMessage = "远程提交结果不明，系统不会自动重复提交"
		if err := repo.SavePluginRemote(remote); err != nil {
			return err
		}
		if err := repo.SavePluginTask(current); err != nil {
			return err
		}
		if current.BillingOrderID != "" {
			if err := repo.MarkBillingUncertain(current.BillingOrderID, run.FailureMessage); err != nil {
				return err
			}
		}
		return plugins.SaveRunTransition(repo, run, "run.paused")
	})
}

func (w *taskWorkerCoordinator) importPluginRemote(task *model.Task, ctx context.Context, runtime pluginRemoteRuntime) error {
	s := w.service
	fresh, err := s.loadPluginRemoteRuntime(task.UserID, task.ID)
	if err != nil {
		return err
	}
	runtime = fresh
	if runtime.Remote.ImportAttempts > 0 && runtime.Remote.ProviderJobID != "" && runtime.Action.Poll != nil {
		response, err := s.pluginHTTPRequest(ctx, task, &runtime, *runtime.Action.Poll, map[string]string{"jobId": runtime.Remote.ProviderJobID}, false)
		if err != nil {
			return w.deferPluginImport(task, err)
		}
		status, ok := pluginJSONPointer(response, runtime.Action.StatusPath)
		if !ok || runtime.Action.StatusMap[fmt.Sprint(status)] != "succeeded" {
			return w.pausePluginState(task, "upstream_output_invalid", "上游结果状态无法确认，已保留旧响应")
		}
		if err = s.storePluginResponse(*task, response, "upstream_completed", runtime.Remote.ProviderJobID); err != nil {
			return err
		}
		runtime, err = s.loadPluginRemoteRuntime(task.UserID, task.ID)
		if err != nil {
			return err
		}
	}
	raw, err := s.decryptSettingSecret(runtime.Remote.ResponseCipher)
	if err != nil {
		return w.failPluginTask(task, "远程结果读取失败", err)
	}
	if creationHashRaw(raw) != runtime.Remote.ResponseDigest {
		return w.pausePluginState(task, "upstream_output_invalid", "已保存响应摘要不匹配")
	}
	var response any
	if err = json.Unmarshal([]byte(raw), &response); err != nil {
		return w.failPluginTask(task, "远程结果读取失败", err)
	}
	result := map[string]any{}
	for field, pointer := range runtime.Action.Outputs {
		value, ok := pluginJSONPointer(response, pointer)
		if !ok {
			return w.failPluginTask(task, "远程输出无效", fmt.Errorf("结果缺少字段 %s", field))
		}
		result[field] = value
	}
	resourceIDs := []string{}
	if artifact := runtime.Action.Artifact; artifact != nil {
		value, ok := pluginJSONPointer(response, artifact.URLPath)
		if !ok {
			return w.failPluginTask(task, "远程输出无效", errors.New("结果缺少产物地址"))
		}
		rawURL, ok := value.(string)
		if !ok || rawURL == "" {
			return w.failPluginTask(task, "远程输出无效", errors.New("产物地址无效"))
		}
		if _, err = ValidateCustomRelayURL(rawURL); err != nil {
			return w.failPluginTask(task, "远程产物地址被拒绝", err)
		}
		resource, importErr := s.importPluginArtifact(ctx, *task, rawURL, artifact.Kind, "plugin:"+runtime.Run.ID+":"+artifact.Field)
		if importErr != nil {
			return w.deferPluginImport(task, importErr)
		}
		result[artifact.Field] = map[string]any{"resourceId": resource.ID, "kind": resource.Kind, "mimeType": resource.MimeType, "bytes": resource.Size}
		resourceIDs = append(resourceIDs, resource.ID)
	}
	secret, _ := s.decryptSettingSecret(runtime.Connection.SecretCipher)
	if !safePluginResult(result, secret) {
		return w.failPluginTask(task, "远程输出含敏感信息或未导入地址", errors.New("unsafe output"))
	}
	resultRaw, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if len(resultRaw) > 64<<10 {
		return w.failPluginTask(task, "远程输出无效", errors.New("结果超过 64KB"))
	}
	if err = contracts.ValidateData(runtime.Files, runtime.Operation.OutputSchemaRef, resultRaw); err != nil {
		return w.failPluginTask(task, "远程输出无效", errors.New("结果不符合插件输出 Schema"))
	}
	return s.repo.WithPluginTask(*task, func(repo *repository.Repository, current *model.Task, remote *model.PluginRemoteExecution, run *model.PluginRun) error {
		if remote.CancelRequested {
			return finishPluginCancellationTx(repo, current, remote, run, "completed_after_cancel")
		}
		now := time.Now()
		current.Status = model.TaskStatusSucceeded
		current.Stage = "插件任务完成"
		current.Progress = 100
		current.ResultJSON = string(resultRaw)
		current.CompletedAt = &now
		current.LeaseOwner = ""
		current.LeaseExpiresAt = nil
		remote.State = "imported"
		remote.FailureReason = ""
		remote.FailureMessage = ""
		remote.ImportAttempts++
		remote.UpdatedAt = now
		run.Status = "succeeded"
		run.ResultJSON = string(resultRaw)
		run.FailureMessage = ""
		if err := repo.AddPluginRunResources(run.UserID, run.ID, "output", resourceIDs); err != nil {
			return err
		}
		if err := repo.SavePluginRemote(remote); err != nil {
			return err
		}
		if err := repo.SavePluginTask(current); err != nil {
			return err
		}
		if current.BillingOrderID != "" {
			if err := repo.SettleBillingOrder(current.BillingOrderID, remote.ProviderJobID); err != nil {
				return err
			}
		}
		return plugins.SaveRunTransition(repo, run, "result.ready")
	})
}
func (w *taskWorkerCoordinator) deferPluginImport(task *model.Task, cause error) error {
	return w.service.repo.WithPluginTask(*task, func(repo *repository.Repository, current *model.Task, remote *model.PluginRemoteExecution, run *model.PluginRun) error {
		remote.State = "import_pending"
		remote.ImportAttempts++
		remote.FailureReason = "import_pending"
		remote.FailureMessage = truncateRunes(cause.Error(), 1000)
		remote.UpdatedAt = time.Now()
		current.Stage = "远程结果导入待重试"
		if remote.ImportAttempts >= 3 {
			current.Status = model.TaskStatusPaused
			run.Status = "paused"
			run.FailureMessage = "导入连续失败，已保留上游结果；恢复只重试导入"
			if err := plugins.SaveRunTransition(repo, run, "run.paused"); err != nil {
				return err
			}
		}
		current.NextPollAt = ptrPluginTime(time.Now().Add(pluginRemotePollDelay))
		current.LeaseOwner = ""
		current.LeaseExpiresAt = nil
		if err := repo.SavePluginRemote(remote); err != nil {
			return err
		}
		return repo.SavePluginTask(current)
	})
}
func (w *taskWorkerCoordinator) failPluginTask(task *model.Task, stage string, cause error) error {
	s := w.service
	return s.repo.WithPluginTask(*task, func(repo *repository.Repository, current *model.Task, remote *model.PluginRemoteExecution, run *model.PluginRun) error {
		now := time.Now()
		current.Status = model.TaskStatusFailed
		current.Stage = stage
		current.Error = stage
		current.CompletedAt = &now
		current.LeaseOwner = ""
		current.LeaseExpiresAt = nil
		remote.State = "failed"
		remote.FailureReason = "upstream_output_invalid"
		remote.FailureMessage = current.Error
		remote.UpdatedAt = now
		run.Status = "failed"
		run.FailureMessage = current.Error
		if err := repo.ReleasePluginInputResources(run.UserID, run.ID); err != nil {
			return err
		}
		if err := repo.SavePluginRemote(remote); err != nil {
			return err
		}
		if err := repo.SavePluginTask(current); err != nil {
			return err
		}
		if current.BillingOrderID != "" {
			if err := repo.RefundBillingOrder(current.BillingOrderID, current.Error); err != nil {
				return err
			}
		}
		return plugins.SaveRunTransition(repo, run, "run.failed")
	})
}
func (w *taskWorkerCoordinator) cancelPluginRemote(task *model.Task, ctx context.Context, runtime pluginRemoteRuntime) error {
	if runtime.Remote.State == "prepared" {
		return w.finishPluginCancellation(task, "not_submitted")
	}
	if runtime.Remote.State == "upstream_completed" || runtime.Remote.State == "import_pending" {
		return w.finishPluginCancellation(task, "completed_after_cancel")
	}
	if runtime.Remote.ProviderJobID == "" {
		return w.finishPluginCancellation(task, "unsupported_external_may_continue")
	}
	if runtime.Action.Cancellation.Mode == "unsupported" || runtime.Action.Cancellation.Request == nil {
		return w.finishPluginCancellation(task, "unsupported_external_may_continue")
	}
	if runtime.Remote.CancelSent {
		return w.pollPluginRemote(task, ctx, runtime)
	}
	err := w.service.repo.WithPluginTask(*task, func(repo *repository.Repository, _ *model.Task, remote *model.PluginRemoteExecution, _ *model.PluginRun) error {
		remote.CancelSent = true
		remote.CancelStatus = "requested"
		return repo.SavePluginRemote(remote)
	})
	if err != nil {
		return err
	}
	response, err := w.service.pluginHTTPRequest(ctx, task, &runtime, *runtime.Action.Cancellation.Request, map[string]string{"jobId": runtime.Remote.ProviderJobID}, false)
	if err == nil {
		status, ok := pluginJSONPointer(response, runtime.Action.StatusPath)
		if ok && runtime.Action.StatusMap[fmt.Sprint(status)] == "cancelled" {
			return w.finishPluginCancellation(task, "confirmed")
		}
	}
	return w.deferPluginTask(task, "取消请求待确认，继续查询原任务", "submitted", pluginRemotePollDelay, err)
}
func (w *taskWorkerCoordinator) finishPluginCancellation(task *model.Task, status string) error {
	return w.service.repo.WithPluginTask(*task, func(repo *repository.Repository, current *model.Task, remote *model.PluginRemoteExecution, run *model.PluginRun) error {
		return finishPluginCancellationTx(repo, current, remote, run, status)
	})
}
func finishPluginCancellationTx(repo *repository.Repository, current *model.Task, remote *model.PluginRemoteExecution, run *model.PluginRun, status string) error {
	now := time.Now()
	current.Status = model.TaskStatusCancelled
	current.Stage = "插件任务已停止"
	current.CompletedAt = &now
	current.LeaseOwner = ""
	current.LeaseExpiresAt = nil
	current.NextPollAt = nil
	remote.State = "cancelled"
	remote.CancelStatus = status
	remote.UpdatedAt = now
	run.Status = "cancelled"
	run.FailureMessage = map[string]string{"confirmed": "远程服务已确认取消；供应商费用以供应商账单为准", "not_submitted": "任务尚未提交，已取消", "completed_after_cancel": "外部任务已完成，本地已停止后续处理；供应商可能收取费用", "unsupported_external_may_continue": "上游不支持取消，本地已停止，外部任务可能继续运行并收费"}[status]
	if status != "unsupported_external_may_continue" {
		if err := repo.ReleasePluginInputResources(run.UserID, run.ID); err != nil {
			return err
		}
	}
	if err := repo.SavePluginRemote(remote); err != nil {
		return err
	}
	if err := repo.SavePluginTask(current); err != nil {
		return err
	}
	if current.BillingOrderID != "" {
		if err := repo.RefundBillingOrder(current.BillingOrderID, "本地服务费退回；供应商费用需独立核对"); err != nil {
			return err
		}
	}
	return plugins.SaveRunTransition(repo, run, "run.cancelled")
}
func (w *taskWorkerCoordinator) pausePluginState(task *model.Task, reason, message string) error {
	return w.service.repo.WithPluginTask(*task, func(repo *repository.Repository, current *model.Task, remote *model.PluginRemoteExecution, run *model.PluginRun) error {
		remote.FailureReason = reason
		remote.FailureMessage = message
		current.Status = model.TaskStatusPaused
		current.Stage = "插件任务待恢复"
		current.LeaseOwner = ""
		current.LeaseExpiresAt = nil
		current.NextPollAt = nil
		run.Status = "paused"
		run.FailureMessage = message
		if err := repo.SavePluginRemote(remote); err != nil {
			return err
		}
		if err := repo.SavePluginTask(current); err != nil {
			return err
		}
		return plugins.SaveRunTransition(repo, run, "run.paused")
	})
}

var pluginJobPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,159}$`)

func validPluginJobID(id string) bool {
	return pluginJobPattern.MatchString(id) && !strings.Contains(id, "..")
}
func safePluginResult(value any, secret string) bool {
	switch v := value.(type) {
	case string:
		return (secret == "" || !strings.Contains(v, secret)) && !strings.Contains(strings.ToLower(v), "http://") && !strings.Contains(strings.ToLower(v), "https://")
	case map[string]any:
		for k, item := range v {
			if !safePluginResult(k, secret) || !safePluginResult(item, secret) {
				return false
			}
		}
	case []any:
		for _, item := range v {
			if !safePluginResult(item, secret) {
				return false
			}
		}
	}
	return true
}
func pluginJSONPointer(value any, pointer string) (any, bool) {
	if raw, ok := value.(json.RawMessage); ok {
		decoded, err := contracts.Decode(raw)
		if err != nil {
			return nil, false
		}
		value = decoded
	}
	if pointer == "" {
		return value, true
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, false
	}
	current := value
	for _, part := range strings.Split(strings.TrimPrefix(pointer, "/"), "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch typed := current.(type) {
		case map[string]any:
			var ok bool
			current, ok = typed[part]
			if !ok {
				return nil, false
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || strconv.Itoa(index) != part || index < 0 || index >= len(typed) {
				return nil, false
			}
			current = typed[index]
		default:
			return nil, false
		}
	}
	return current, true
}
