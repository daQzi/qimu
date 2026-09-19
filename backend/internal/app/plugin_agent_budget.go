package app

import (
	"infinite-canvas/backend/internal/repository"
	"math"
)

func pluginAgentRemaining(repo *repository.Repository, userID, agentID string, state cloudAgentRuntime) (int64, error) {
	remaining := int64(math.Floor(state.Request.Budget.MaxCredits * float64(CreditScale)))
	orders, err := repo.BillingOrdersByTaskIDs(userID, state.TaskIDs)
	if err != nil {
		return 0, err
	}
	for _, order := range orders {
		remaining -= order.AmountMicrocredits
	}
	if state.Request.PluginToolsVersion == 1 {
		charges, err := repo.PluginAgentCharges(userID, agentID)
		if err != nil {
			return 0, err
		}
		remaining -= charges
	}
	return remaining, nil
}
