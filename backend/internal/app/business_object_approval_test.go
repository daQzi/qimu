package app

import (
	"encoding/json"
	"infinite-canvas/backend/internal/plugins/contracts"
	"testing"
)

func TestBusinessObjectApprovalRechecksState(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s, _, _ := p03Fixture(t, driver)
			release := installWorkbenchSample(t, s, "brand-points")
			first, err := s.SaveBusinessObject("user", brandWrite("approval-first"))
			if err != nil {
				t.Fatal(err)
			}
			write := brandWrite("approval-update")
			write.ObjectID = first.Reference.ObjectID
			write.ExpectedVersion = 1
			invoke := func(key string) PluginInvocationOutput {
				t.Helper()
				raw, _ := json.Marshal(write)
				var input map[string]json.RawMessage
				json.Unmarshal(raw, &input)
				pending, e := s.InvokePluginOperation("user", key, contracts.Invocation{Operation: "brand-points.save", ReleaseID: release, Input: input})
				if e != nil {
					t.Fatal(e)
				}
				return pending
			}
			rejected := invoke("reject-brand-update")
			if _, err = s.DecidePluginRun("user", rejected.RunID, rejected.ApprovalID, "reject", rejected.Revision); err != nil {
				t.Fatal(err)
			}
			old, _ := s.ReadBusinessObject("user", first.Reference.ObjectID, 1)
			if old.CurrentVersion != 1 {
				t.Fatal("rejected write committed")
			}
			pending := invoke("stale-brand-update")
			changed := write
			changed.ClientKey = "direct-concurrent"
			changed.Brand.Name = "另一个明确保存"
			if _, err = s.SaveBusinessObject("user", changed); err != nil {
				t.Fatal(err)
			}
			if _, err = s.DecidePluginRun("user", pending.RunID, pending.ApprovalID, "approve", pending.Revision); err == nil {
				t.Fatal("stale approval committed")
			}
			old, _ = s.ReadBusinessObject("user", first.Reference.ObjectID, 1)
			if old.CurrentVersion != 2 {
				t.Fatal("stale approval changed versions")
			}
			write.ExpectedVersion = 2
			write.ClientKey = "archive-waiting"
			pending = invoke("archive-waiting-run")
			if err = s.ArchiveBusinessObject("user", first.Reference.ObjectID, 2, true); err != nil {
				t.Fatal(err)
			}
			if _, err = s.DecidePluginRun("user", pending.RunID, pending.ApprovalID, "approve", pending.Revision); err == nil {
				t.Fatal("archived approval committed")
			}
		})
	}
}
