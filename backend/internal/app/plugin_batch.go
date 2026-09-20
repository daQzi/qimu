package app

import (
	"errors"
	"fmt"
	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins"
	"infinite-canvas/backend/internal/repository"
	"log"
	"os"
	"strconv"
	"strings"
)

type PluginBatchApproveRequest = plugins.BatchApproveRequest
type PluginDeriveRequest = plugins.DeriveRequest

func (s *Service) PluginBatchQuote(user, id string) (plugins.BatchQuote, error) {
	if err := s.pluginOperationAccess(user); err != nil {
		return plugins.BatchQuote{}, err
	}
	return s.applicationPlugins().BatchQuote(user, id)
}
func (s *Service) ApprovePluginBatch(user, id string, req PluginBatchApproveRequest) (plugins.RunView, error) {
	if err := s.pluginOperationAccess(user); err != nil {
		return plugins.RunView{}, err
	}
	result, err := s.applicationPlugins().ApproveBatch(user, id, req)
	if err != nil {
		var appErr *kernel.AppError
		if errors.As(err, &appErr) && (appErr.Reason == "plugin_budget_exceeded" || appErr.Reason == "quote_changed") {
			if metricErr := s.repo.RecordPluginBatchConflict(user, appErr.Reason == "plugin_budget_exceeded"); metricErr != nil {
				log.Printf("plugin approval metric failed: %v", metricErr)
			}
		}
	}
	return result, err
}
func (s *Service) DerivePluginRun(user, id, key string, req PluginDeriveRequest) (plugins.RunView, error) {
	if err := s.pluginOperationAccess(user); err != nil {
		return plugins.RunView{}, err
	}
	return s.applicationPlugins().Derive(user, id, key, req)
}
func (s *Service) PluginBatchDiagnostics(user string) (repository.PluginBatchDiagnostics, error) {
	return s.repo.PluginBatchDiagnostics(user)
}

func (s *Service) acquirePluginSlot(task *model.Task, runtime pluginRemoteRuntime) (bool, error) {
	policy, err := s.runtimeConcurrencySetting()
	if err != nil {
		return false, err
	}
	values := []int{min(8, policy.WorkerConcurrency), 4, min(2, policy.ChannelConcurrency), 2, 4}
	for i, name := range []string{"HOST", "USER", "CONNECTION", "MODEL", "PLUGIN"} {
		if raw := os.Getenv("CANVAS_PLUGIN_" + name + "_CONCURRENCY"); raw != "" {
			v, e := strconv.Atoi(raw)
			if e != nil || v < 1 || v > 8 {
				return false, fmt.Errorf("CANVAS_PLUGIN_%s_CONCURRENCY must be 1..8", name)
			}
			values[i] = min(values[i], v)
		}
	}
	// Generic HTTP has no verified supplier model registry. Pool the connection
	// conservatively so an arbitrary input.model cannot bypass the model ceiling.
	modelKey := hashStrings("unclassified-model", runtime.Connection.ConnectionID)
	return s.repo.AcquirePluginExecutionSlot(*task, model.PluginExecutionSlot{UserID: task.UserID, ConnectionID: runtime.Connection.ConnectionID, PluginID: strings.Split(runtime.Run.Operation, ".")[0], ModelKey: modelKey}, repository.PluginConcurrencyLimits{Host: values[0], User: values[1], Connection: values[2], Model: values[3], Plugin: values[4]})
}
