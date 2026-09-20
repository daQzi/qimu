package authoring

import (
	"encoding/json"
	"infinite-canvas/backend/internal/plugins/contracts"
)

// Inspection describes the validated package, not the current installation or
// an authorization decision. No package instructions, keys or model calls run.
type Inspection struct {
	ID              string                 `json:"id"`
	Version         string                 `json:"version"`
	HostAPI         string                 `json:"hostApi"`
	PackageDigest   string                 `json:"packageDigest"`
	RuntimeVerified bool                   `json:"runtimeVerified"`
	Permissions     []string               `json:"permissions"`
	Dependencies    []contracts.Dependency `json:"dependencies"`
	Operations      []OperationInspection  `json:"operations"`
	Contributions   map[string]int         `json:"contributions"`
	Checks          []string               `json:"requiredRuntimeChecks"`
}
type OperationInspection struct {
	Address         string              `json:"address"`
	Execution       contracts.Execution `json:"execution"`
	InputSchema     string              `json:"inputSchema"`
	OutputSchema    string              `json:"outputSchema"`
	Permissions     []string            `json:"permissions"`
	Effects         []string            `json:"effects"`
	RequiresCanvas  bool                `json:"requiresCanvas"`
	RequiresProject bool                `json:"requiresProject"`
}

func Inspect(files contracts.PackageFiles, policy contracts.Policy) (Inspection, error) {
	if err := contracts.ValidatePackage(files, policy); err != nil {
		return Inspection{}, err
	}
	var m contracts.Manifest
	if err := json.Unmarshal(files["manifest.json"], &m); err != nil {
		return Inspection{}, err
	}
	result := Inspection{ID: m.ID, Version: m.Version, HostAPI: m.Requires.HostAPI, PackageDigest: contracts.PackageDigest(files), Permissions: m.Permissions, Dependencies: m.Dependencies, Operations: []OperationInspection{}, Contributions: map[string]int{"skills": len(m.Contributes.Skills), "operations": len(m.Contributes.Operations), "views": len(m.Contributes.Views), "pipelines": len(m.Contributes.Pipelines), "canvasBlueprints": len(m.Contributes.CanvasBlueprints), "workbenches": len(m.Contributes.Workbenches), "recipes": len(m.Contributes.Recipes), "connectors": len(m.Contributes.Connectors)}, Checks: []string{"管理员受控安装与发布可用性", "当前账号启用、权限授权与依赖版本核验"}}
	seen := map[string]bool{}
	add := func(check string) {
		if !seen[check] {
			seen[check] = true
			result.Checks = append(result.Checks, check)
		}
	}
	for _, ref := range m.Contributes.Operations {
		var op contracts.Operation
		if err := json.Unmarshal(files[ref.Ref], &op); err != nil {
			return Inspection{}, err
		}
		execution := op.Execution
		execution.Instruction = "" // Text is untrusted task material, not diagnostic data.
		result.Operations = append(result.Operations, OperationInspection{Address: m.ID + "." + op.ID, Execution: execution, InputSchema: op.InputSchemaRef, OutputSchema: op.OutputSchemaRef, Permissions: op.RequiredPermissions, Effects: op.Effects, RequiresCanvas: op.Context.RequiresCanvas, RequiresProject: op.Context.RequiresProject})
		switch op.Execution.Kind {
		case "model":
			add("系统模型目录、能力/媒体限制、有效价格、余额及逐次审批")
			if op.Execution.OutputProfile == "video-report/v1" {
				add("ffprobe/ffmpeg、原片归属与音轨能力声明")
			}
		case "http":
			add("账号连接配置、密钥与域名白名单、服务价格、远端提交/查询/取消和结果导入")
		case "pipeline":
			add("Worker/调度、人工输入、重启恢复、取消与派生复用")
		}
		if op.Context.RequiresCanvas || op.Execution.Adapter == "canvas.blueprint.instantiate" {
			add("本人画布、最新 snapshotHash 与写入确认")
		}
	}
	if len(m.Contributes.Workbenches) > 0 {
		add("工作台输入、配方合并及 Agent 启动入口")
	}
	if len(result.Operations) == 0 {
		add("技能可被 Agent 读取；仅有技能文本不代表新增执行能力")
	}
	return result, nil
}

// This lists implemented authoring profiles. It deliberately does not claim
// arbitrary package code, arbitrary UI, machine OAuth or marketplace support.
func Capabilities() any {
	return map[string]any{
		"apiVersion": "yingce.plugin/v3", "hostApi": "3.3.0", "scope": "offline-authoring", "runtimeVerified": false,
		"templates":         []string{"skill", "resource", "model-review"},
		"executionProfiles": []string{"host/inline", "http/task", "pipeline/task", "model/task"},
		"hostAdapters":      []string{"resource.inspect", "resource.snapshot", "canvas.blueprint.instantiate", "object.read", "object.search", "object.save"},
		"contributions":     []string{"skills", "operations", "views", "canvasBlueprints", "connectors", "pipelines", "workbenches", "recipes"},
		"notProvided":       []string{"包内任意脚本执行", "包内任意前端组件加载", "公网插件市场", "外部系统机器身份/OAuth 开放 API"},
		"notice":            "编译版本的能力说明，不证明部署已启用、账号已授权或外部服务可用。使用 -inspect 查看具体包的运行前置条件。",
	}
}
