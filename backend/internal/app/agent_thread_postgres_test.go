package app

import (
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"
	"sync"
	"testing"
)

func TestAgentThreadPostgresAdmission(t *testing.T) {
	s, db, _ := p03Fixture(t, "postgres")
	capability := mustEncodeModelCapabilityConfig(t, DefaultModelCapabilityConfigForModel(string(model.ChannelInterfaceChatCompletion), "text-test"))
	for _, row := range []any{&model.ModelChannel{ID: "channel", Scope: model.ChannelScopeSystem, Enabled: true, Name: "Thread test"}, &model.ChannelModel{ID: "cm", ChannelID: "channel", ModelKey: "text-test", Capability: "text", Protocol: model.ChannelInterfaceChatCompletion, CapabilityConfigJSON: capability, BillingMode: "fixed_request", UnitPriceMicrocredits: 100, PriceConfigured: true, Enabled: true}, &model.ChannelModelPriceTier{ID: "tier", ChannelModelID: "cm", SelectorKey: "{}", SelectorJSON: "{}", BillingMode: "fixed_request", UnitPriceMicrocredits: 100, PriceConfigured: true, Enabled: true}, &model.CreditAccount{UserID: "user", AvailableMicrocredits: 10000}} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	thread, err := s.CreateAgentThread("user", AgentThreadCreate{ClientKey: "pg-thread-test"})
	if err != nil {
		t.Fatal(err)
	}
	// Separate service instances exercise the database lock, not the in-process mutex.
	services := []*Service{s, {repo: repository.New(db), dataDir: s.dataDir}}
	results := make(chan *AgentThreadMessageResult, 2)
	failures := make(chan error, 2)
	var wg sync.WaitGroup
	req := AgentThreadMessage{Revision: 1, Request: threadTestRequest()}
	for _, svc := range services {
		wg.Add(1)
		go func(svc *Service) {
			defer wg.Done()
			result, err := svc.AppendAgentThreadMessage("user", thread.ID, req)
			results <- result
			failures <- err
		}(svc)
	}
	wg.Wait()
	close(results)
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	id := ""
	for result := range results {
		if id != "" && id != result.Run.ID {
			t.Fatal("two runs for one key")
		}
		id = result.Run.ID
	}
	var count int64
	db.Model(&model.Task{}).Where("operation = ?", cloudAgentOperation).Count(&count)
	if count != 1 {
		t.Fatal("duplicate task")
	}
	var account model.CreditAccount
	db.First(&account, "user_id = ?", "user")
	if account.AvailableMicrocredits != 9900 {
		t.Fatalf("fee charged more than once: %d", account.AvailableMicrocredits)
	}
	fresh, err := s.GetAgentThread("user", thread.ID, 0)
	if err != nil || fresh.Thread.LastSequence != 1 {
		t.Fatalf("thread read: %v", err)
	}
}
