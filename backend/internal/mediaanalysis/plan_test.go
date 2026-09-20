package mediaanalysis

import "testing"

func TestPlanReferencesAndExecutionBoundary(t *testing.T) {
	report := Report{Entities: []Entity{{ID: "p1", Kind: "character"}}, Dialogue: []Utterance{{ID: "d1"}}}
	base := Replacement{EntityID: "p1", Kind: "character", Source: "人物", Target: "本地化人物", Prompt: "保留剧情", Reason: "未接入编辑服务"}
	plan := Plan{Mappings: []Replacement{base}, Limitations: []string{"只保存方案"}}
	if err := ValidatePlan(plan, report); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Replacement){func(v *Replacement) { v.Executable = true }, func(v *Replacement) { v.EntityID = "missing" }, func(v *Replacement) { v.Kind = "scene" }, func(v *Replacement) { v.Prompt = "" }} {
		item := base
		mutate(&item)
		plan.Mappings = []Replacement{item}
		if ValidatePlan(plan, report) == nil {
			t.Fatal("invalid mapping accepted", item)
		}
	}
	plan.Mappings = []Replacement{base, base}
	if ValidatePlan(plan, report) == nil {
		t.Fatal("duplicate mapping accepted")
	}
	voice := base
	voice.Kind = "voice"
	plan.Mappings = []Replacement{base, voice}
	if err := ValidatePlan(plan, report); err != nil {
		t.Fatal("separate visual and voice proposals rejected", err)
	}
}
