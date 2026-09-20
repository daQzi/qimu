package mediaanalysis

import "fmt"

type Replacement struct {
	EntityID   string `json:"entityId"`
	Kind       string `json:"kind"`
	Source     string `json:"source"`
	Target     string `json:"target"`
	Prompt     string `json:"prompt"`
	MaterialID string `json:"materialId"`
	Executable bool   `json:"executable"`
	Reason     string `json:"reason"`
}
type Plan struct {
	Mappings    []Replacement `json:"mappings"`
	Limitations []string      `json:"limitations"`
}

// P11 freezes proposals only. No generation/editing provider is authorized by
// submitting this plan, even if a model claims that a replacement is executable.
func ValidatePlan(plan Plan, report Report) error {
	if plan.Mappings == nil || len(plan.Mappings) > 200 || !notes(plan.Limitations) || len(plan.Limitations) == 0 {
		return fmt.Errorf("替换方案须包含映射数组和能力限制说明")
	}
	entities := map[string]string{}
	for _, e := range report.Entities {
		entities[e.ID] = e.Kind
	}
	for _, d := range report.Dialogue {
		entities[d.ID] = "subtitle"
	}
	seen := map[string]bool{}
	for _, m := range plan.Mappings {
		kind, ok := entities[m.EntityID]
		if !ok || (m.Kind != kind && !(m.Kind == "voice" && kind == "character")) {
			return fmt.Errorf("替换项引用不存在的实体或类型不符：%s", m.EntityID)
		}
		key := m.Kind + ":" + m.EntityID
		if seen[key] {
			return fmt.Errorf("替换项重复：%s", key)
		}
		seen[key] = true
		if !text(m.Source, 500) || !text(m.Target, 2000) || !text(m.Prompt, 4000) || !text(m.Reason, 500) || len(m.MaterialID) > 80 {
			return fmt.Errorf("替换项须补全原内容、目标、提示词和不可执行原因")
		}
		if m.Executable {
			return fmt.Errorf("P11 只确认方案，不批准视频替换或生成")
		}
	}
	return nil
}
