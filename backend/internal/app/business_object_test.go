package app

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/objects"
	"infinite-canvas/backend/internal/plugins/contracts"
)

func brandWrite(key string) objects.WriteRequest {
	return objects.WriteRequest{ClientKey: key, Brand: objects.Brand{Name: "品牌原文", Audience: "创作者", Positioning: "可信内容", Claims: []string{"可核实的卖点"}, Restrictions: []string{"不编造价格"}}}
}

func TestBusinessObjectLifecycle(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s, db, _ := p03Fixture(t, driver)
			req := brandWrite("create-brand-one")
			first, err := s.SaveBusinessObject("user", req)
			if err != nil {
				t.Fatal(err)
			}
			replay, err := s.SaveBusinessObject("user", req)
			if err != nil || replay.Reference != first.Reference {
				t.Fatal("create replay", err)
			}
			req.Brand.Name = "篡改"
			if _, err = s.SaveBusinessObject("user", req); err == nil {
				t.Fatal("same key changed body")
			}
			for _, ref := range []objects.Reference{first.Reference, {ObjectID: first.Reference.ObjectID, Version: 1, Type: "product", SchemaVersion: 1}, {ObjectID: first.Reference.ObjectID, Version: 1, Type: "brand", SchemaVersion: 2}} {
				user := "user"
				if ref == first.Reference {
					user = "other"
				}
				if _, err = objects.Read(s.repo, user, ref, true); err == nil {
					t.Fatal("foreign or incompatible reference accepted", ref)
				}
			}
			rows, err := s.ListBusinessObjects("other", "", false, 0)
			if err != nil || len(rows) != 0 {
				t.Fatal("list leaked", err)
			}
			update := brandWrite("update-brand-one")
			update.ObjectID = first.Reference.ObjectID
			update.ExpectedVersion = 1
			update.Brand.Name = "品牌新版"
			var wg sync.WaitGroup
			results := make(chan error, 2)
			for i := 0; i < 2; i++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					r := update
					r.ClientKey += string(rune('a' + i))
					_, e := s.SaveBusinessObject("user", r)
					results <- e
				}(i)
			}
			wg.Wait()
			close(results)
			success := 0
			for e := range results {
				if e == nil {
					success++
				}
			}
			if success != 1 {
				t.Fatalf("CAS admitted %d writers", success)
			}
			old, err := s.ReadBusinessObject("user", first.Reference.ObjectID, 1)
			if err != nil || old.Brand.Name != "品牌原文" || old.CurrentVersion != 2 {
				t.Fatal("history mutated", old, err)
			}
			if err = s.ArchiveBusinessObject("other", first.Reference.ObjectID, 2, true); err == nil {
				t.Fatal("foreign archive")
			}
			if err = s.ArchiveBusinessObject("user", first.Reference.ObjectID, 1, true); err == nil {
				t.Fatal("stale archive")
			}
			if err = s.ArchiveBusinessObject("user", first.Reference.ObjectID, 2, true); err != nil {
				t.Fatal(err)
			}
			if _, err = objects.Read(s.repo, "user", first.Reference, false); err == nil {
				t.Fatal("fresh use of archive")
			}
			if _, err = s.ReadBusinessObject("user", first.Reference.ObjectID, 1); err != nil {
				t.Fatal("archive erased history", err)
			}
			derive := brandWrite("derive-brand-one")
			derive.Source = &first.Reference
			derived, err := s.SaveBusinessObject("user", derive)
			if err != nil || derived.Reference.ObjectID == first.Reference.ObjectID || derived.Source == nil || *derived.Source != first.Reference {
				t.Fatal("derivation", derived, err)
			}
			derive.ClientKey = "derive-foreign"
			if _, err = s.SaveBusinessObject("other", derive); err == nil {
				t.Fatal("foreign derivation")
			}
			if err = s.ArchiveBusinessObject("user", first.Reference.ObjectID, 2, false); err != nil {
				t.Fatal(err)
			}
			if _, err = objects.Read(s.repo, "user", first.Reference, false); err != nil {
				t.Fatal(err)
			}
			var count int64
			db.Model(&model.BusinessObjectVersion{}).Where("object_id = ?", first.Reference.ObjectID).Count(&count)
			if count != 2 {
				t.Fatal("unexpected retained versions", count)
			}
			// Quota failures must also reject the approval preview, without an extra write.
			db.Model(&model.BusinessObject{}).Where("id = ?", first.Reference.ObjectID).Update("version", 100)
			update.ExpectedVersion = 100
			update.ClientKey = "quota-version"
			if _, err = objects.Preview(s.repo, "user", update); err == nil {
				t.Fatal("preview bypassed version quota")
			}
			if _, err = s.SaveBusinessObject("user", update); err == nil {
				t.Fatal("save bypassed version quota")
			}
		})
	}
}

func TestBusinessObjectPluginsReuseAndApproval(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			s, db, _ := p03Fixture(t, driver)
			first, err := s.SaveBusinessObject("user", brandWrite("cross-plugin-brand"))
			if err != nil {
				t.Fatal(err)
			}
			ctx := contracts.InvocationContext{HostSurface: "agent-home"}
			releases := map[string]string{}
			for _, name := range []string{"brand-points", "brand-script"} {
				releases[name] = installWorkbenchSample(t, s, name)
				preview, e := s.ComposePluginWorkbench("user", PluginWorkbenchComposeRequest{ID: name + ".compose", ReleaseID: releases[name], RecipeIDs: []string{}, Input: map[string]any{"reference": first.Reference}, Context: ctx})
				if e != nil || !preview.Valid || preview.Objects["reference"].Reference != first.Reference || preview.Objects["reference"].Brand.Name != first.Brand.Name {
					t.Fatal("shared version", preview, e)
				}
				wire, _ := json.Marshal(preview.Selection)
				var selection contracts.WorkbenchSelection
				json.Unmarshal(wire, &selection)
				if _, _, e = s.applicationPlugins().VerifyWorkbench("user", selection, ctx); e != nil {
					t.Fatal("reference digest changed across HTTP", e)
				}
				output, e := s.InvokePluginOperation("user", "", contracts.Invocation{Operation: name + ".read", ReleaseID: releases[name], Input: p03Input(preview.Input), Context: &ctx})
				if e != nil || output.Kind != "inline" {
					t.Fatal("read", output, e)
				}
			}
			write := brandWrite("approved-brand-update")
			write.ObjectID = first.Reference.ObjectID
			write.ExpectedVersion = 1
			write.Brand.Name = "已审批品牌"
			raw, _ := json.Marshal(write)
			var input map[string]json.RawMessage
			json.Unmarshal(raw, &input)
			invocation := contracts.Invocation{Operation: "brand-points.save", ReleaseID: releases["brand-points"], Input: input, Context: &ctx}
			pending, err := s.InvokePluginOperation("user", "save-brand-run", invocation)
			if err != nil || pending.Status != "waiting_approval" {
				t.Fatal("approval bypass", pending, err)
			}
			old, _ := s.ReadBusinessObject("user", first.Reference.ObjectID, 1)
			if old.CurrentVersion != 1 {
				t.Fatal("wrote before approval")
			}
			run := p03Approve(t, s, pending)
			if run.Status != "succeeded" {
				t.Fatal("write not completed", run.Status)
			}
			replay, err := s.InvokePluginOperation("user", "save-brand-run", invocation)
			if err != nil || replay.RunID != pending.RunID {
				t.Fatal("duplicate run", err)
			}
			old, err = s.ReadBusinessObject("user", first.Reference.ObjectID, 1)
			if err != nil || old.CurrentVersion != 2 || old.Brand.Name != first.Brand.Name {
				t.Fatal("version overwritten", err)
			}
			if err = s.ArchiveBusinessObject("user", first.Reference.ObjectID, 2, true); err != nil {
				t.Fatal(err)
			}
			req := PluginWorkbenchComposeRequest{ID: "brand-script.compose", ReleaseID: releases["brand-script"], RecipeIDs: []string{}, Input: map[string]any{"reference": first.Reference}, Context: ctx}
			if _, err = s.ComposePluginWorkbench("user", req); err == nil {
				t.Fatal("archived new launch")
			}
			var saved model.PluginRun
			if err = db.First(&saved, "id = ?", run.ID).Error; err != nil || !strings.Contains(saved.RequestJSON, first.Reference.ObjectID) {
				t.Fatal("run provenance lost", err)
			}
			s.ArchiveBusinessObject("user", first.Reference.ObjectID, 2, false)
			state, _ := s.repo.UserPluginState("user", "brand-script")
			if err = s.ActivateApplicationPlugin(&model.User{ID: "user", Role: model.UserRoleUser}, "brand-script", ApplicationPluginActivation{ReleaseID: releases["brand-script"], Enabled: true, Revision: state.Revision, GrantedPermissions: []string{}}); err != nil {
				t.Fatal(err)
			}
			if _, err = s.ComposePluginWorkbench("user", req); err == nil {
				t.Fatal("revoked read grant accepted")
			}
		})
	}
}

func TestBusinessObjectAgentFreezesFacts(t *testing.T) {
	s, db, _, _ := creationTestService(t)
	if err := db.Create(&model.User{ID: "user", Username: "user", Role: model.UserRoleUser, Status: model.UserStatusActive}).Error; err != nil {
		t.Fatal(err)
	}
	release := installWorkbenchSample(t, s, "brand-script")
	first, err := s.SaveBusinessObject("user", brandWrite("agent-brand"))
	if err != nil {
		t.Fatal(err)
	}
	preview, err := s.ComposePluginWorkbench("user", PluginWorkbenchComposeRequest{ID: "brand-script.compose", ReleaseID: release, RecipeIDs: []string{}, Input: map[string]any{"reference": first.Reference}, Context: contracts.InvocationContext{HostSurface: "agent-home"}})
	if err != nil || !preview.Valid {
		t.Fatal(err)
	}
	thread, err := s.CreateAgentThread("user", AgentThreadCreate{ClientKey: "brand-thread"})
	if err != nil {
		t.Fatal(err)
	}
	req := AgentThreadMessage{Revision: thread.Revision, Request: threadTestRequest()}
	req.Request.Workbench = &preview.Selection
	admitted, err := s.AppendAgentThreadMessage("user", thread.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	before, _, err := s.cloudAgentTask("user", admitted.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(before.InputJSON)
	if !strings.Contains(string(raw), "品牌原文") || !strings.Contains(string(raw), first.Reference.ObjectID) {
		t.Fatal("Agent lacks actual immutable facts")
	}
	update := brandWrite("agent-brand-update")
	update.ObjectID = first.Reference.ObjectID
	update.ExpectedVersion = 1
	update.Brand.Name = "不可泄入旧任务"
	if _, err = s.SaveBusinessObject("user", update); err != nil {
		t.Fatal(err)
	}
	after, _, err := s.cloudAgentTask("user", admitted.Run.ID)
	if err != nil {
		t.Fatal(err)
	}
	frozen := []byte(after.InputJSON)
	if string(frozen) != string(raw) {
		t.Fatal("admitted facts changed")
	}
}
