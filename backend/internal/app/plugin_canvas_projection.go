package app

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"

	"gorm.io/gorm"
	"infinite-canvas/backend/internal/kernel"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/platform"
	"infinite-canvas/backend/internal/plugins"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/repository"
)

type pluginProjectionInput struct {
	RunID          string  `json:"runId"`
	ResultDigest   string  `json:"resultDigest"`
	InputRequestID string  `json:"inputRequestId,omitempty"`
	BlueprintID    string  `json:"blueprintId"`
	SnapshotHash   string  `json:"snapshotHash"`
	InstanceKey    string  `json:"instanceKey"`
	X              float64 `json:"x"`
	Y              float64 `json:"y"`
}

func pluginProjectionConflict() error {
	return &kernel.AppError{Status: 409, Code: 409, Reason: "projection_conflict", Message: "画布或绑定节点已变化，结果仍保留。请取消旧审批，读取最新画布后重试；已修改或删除的节点请使用新的 instanceKey 另建节点。"}
}

func preparePluginCanvasProjection(repo *repository.Repository, userID string, input map[string]json.RawMessage, ctx plugins.HostOperationContext) (plugins.PreparedOperation, error) {
	var a pluginProjectionInput
	raw, err := json.Marshal(input)
	if err != nil {
		return plugins.PreparedOperation{}, err
	}
	if err = decodeCloudAgentJSONObject(string(raw), &a); err != nil {
		return plugins.PreparedOperation{}, err
	}
	if ctx.CanvasID == "" || a.RunID == "" || (a.InputRequestID == "" && len(a.ResultDigest) != 64) || (a.InputRequestID != "" && a.ResultDigest != "") || len(a.SnapshotHash) != 64 || len(a.InstanceKey) < 1 || len(a.InstanceKey) > 80 || strings.TrimSpace(a.InstanceKey) != a.InstanceKey || math.Abs(a.X) > 100000 || math.Abs(a.Y) > 100000 {
		return plugins.PreparedOperation{}, BadAuthRequest("需要真实运行输入或结果、画布快照和有效绑定标识")
	}
	source, err := repo.PluginRunForUser(userID, a.RunID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return plugins.PreparedOperation{}, Forbidden("结果不存在或不属于当前用户")
	}
	if err != nil {
		return plugins.PreparedOperation{}, err
	}
	if source.ReleaseID != ctx.ReleaseID {
		return plugins.PreparedOperation{}, BadAuthRequest("来源与当前操作必须属于同一发布")
	}
	var inputRequest *model.PluginInputRequest
	if a.InputRequestID != "" {
		inputRequest, err = repo.PluginInput(source.ID, a.InputRequestID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return plugins.PreparedOperation{}, BadAuthRequest("输入请求不存在或不属于该运行")
		}
		if err != nil {
			return plugins.PreparedOperation{}, err
		}
		if inputRequest.Status != "pending" || (source.Status != "waiting_input" && source.Status != "running" && source.Status != "waiting_approval") {
			return plugins.PreparedOperation{}, BadAuthRequest("输入已完成或运行已停止，请读取最新运行")
		}
		a.ResultDigest = creationHashRaw(source.ID + ":" + inputRequest.ID + ":" + inputRequest.SchemaJSON)
	} else if source.Status != "succeeded" || creationHashRaw(source.ResultJSON) != a.ResultDigest {
		return plugins.PreparedOperation{}, BadAuthRequest("必须引用真实成功结果及摘要")
	}
	var manifest contracts.Manifest
	if err = json.Unmarshal(ctx.Files["manifest.json"], &manifest); err != nil {
		return plugins.PreparedOperation{}, err
	}
	var blueprint contracts.CanvasBlueprint
	for _, ref := range manifest.Contributes.CanvasBlueprints {
		if ref.ID == a.BlueprintID {
			if err = json.Unmarshal(ctx.Files[ref.Ref], &blueprint); err != nil {
				return plugins.PreparedOperation{}, err
			}
		}
	}
	if len(blueprint.Nodes) == 0 {
		return plugins.PreparedOperation{}, BadAuthRequest("发布中不存在该画布蓝图")
	}
	if err := contracts.ValidateBlueprint(blueprint, ctx.Files); err != nil {
		return plugins.PreparedOperation{}, err
	}
	if inputRequest != nil {
		pipeline, err := contracts.LoadPipeline(ctx.Files, source.PipelineID)
		if err != nil {
			return plugins.PreparedOperation{}, err
		}
		matched := false
		for _, step := range pipeline.Steps {
			if step.Key == inputRequest.StepKey {
				matched, err = contracts.BlueprintMatchesInput(ctx.Files, blueprint, step)
				if err != nil {
					return plugins.PreparedOperation{}, err
				}
			}
		}
		if !matched {
			return plugins.PreparedOperation{}, BadAuthRequest("输入视图与流程步骤不匹配")
		}
	}
	views := map[string]contracts.ResultView{}
	for _, ref := range manifest.Contributes.Views {
		var view contracts.ResultView
		if err = json.Unmarshal(ctx.Files[ref.Ref], &view); err != nil {
			return plugins.PreparedOperation{}, err
		}
		views[ref.ID] = view
	}
	canvas, err := repo.CanvasProjectForUser(userID, ctx.CanvasID)
	if err != nil {
		return plugins.PreparedOperation{}, err
	}
	doc, err := creationDocument(canvas.PayloadJSON)
	if err != nil {
		return plugins.PreparedOperation{}, err
	}
	identityParts := []string{userID, ctx.CanvasID, source.ID, ctx.ReleaseID, a.BlueprintID, a.InstanceKey}
	if inputRequest != nil {
		identityParts = append(identityParts, inputRequest.ID)
	}
	identity, _ := json.Marshal(identityParts)
	projectionID := creationHashRaw(string(identity))
	existing, err := repo.PluginCanvasProjection(userID, projectionID)
	if err != nil {
		return plugins.PreparedOperation{}, err
	}
	nodes := creationMaps(doc["nodes"])
	if existing != nil {
		var originals []map[string]any
		if err = json.Unmarshal([]byte(existing.NodesJSON), &originals); err != nil {
			return plugins.PreparedOperation{}, err
		}
		for _, original := range originals {
			index := cloudAgentNodeIndex(nodes, stringValue(original["id"]))
			if index < 0 || pluginProjectionNodeHash(nodes[index]) != pluginProjectionNodeHash(original) {
				return plugins.PreparedOperation{}, pluginProjectionConflict()
			}
		}
		if !pluginProjectionEdgesIntact(doc, blueprint, existing.BindingsJSON, projectionID) {
			return plugins.PreparedOperation{}, pluginProjectionConflict()
		}
		return plugins.PreparedOperation{Result: json.RawMessage(existing.ResultJSON), SourceDigest: a.ResultDigest, ProjectsToCanvas: true}, nil
	}
	if cloudAgentCanvasHash(doc) != a.SnapshotHash {
		return plugins.PreparedOperation{}, pluginProjectionConflict()
	}
	if _, supplied := input["x"]; !supplied {
		a.X = 80
		for _, node := range nodes {
			position, _ := node["position"].(map[string]any)
			x, _ := position["x"].(float64)
			width, _ := node["width"].(float64)
			a.X = math.Max(a.X, x+width+80)
		}
		if a.X > 100000 {
			return plugins.PreparedOperation{}, BadAuthRequest("当前画布自动布局超出范围，请指定保存位置")
		}
	}
	if _, supplied := input["y"]; !supplied {
		a.Y = 80
	}
	bindings := map[string]string{}
	added := []map[string]any{}
	for _, item := range blueprint.Nodes {
		view, exists := views[item.View]
		if !exists || (item.Binding == "input") != (inputRequest != nil) {
			return plugins.PreparedOperation{}, BadAuthRequest("蓝图与当前输入/结果绑定不匹配")
		}
		if inputRequest == nil && contracts.ValidateData(ctx.Files, view.SchemaRef, []byte(source.ResultJSON)) != nil {
			return plugins.PreparedOperation{}, BadAuthRequest("结果不符合蓝图视图 Schema")
		}
		id := "plugin-" + creationHashRaw(projectionID + ":" + item.Key)[:32]
		if cloudAgentNodeIndex(nodes, id) >= 0 {
			return plugins.PreparedOperation{}, pluginProjectionConflict()
		}
		x, y := a.X+item.Position.X, a.Y+item.Position.Y
		if math.Abs(x) > 100000 || math.Abs(y) > 100000 {
			return plugins.PreparedOperation{}, BadAuthRequest("蓝图相对布局超出画布范围")
		}
		binding := map[string]any{"runId": source.ID, "digest": a.ResultDigest, "releaseId": source.ReleaseID, "viewId": item.View, "projectionId": projectionID, "bindingKey": item.Key, "canvasId": canvas.ID, "actions": item.Actions}
		bindingField, status := "pluginResult", "success"
		if inputRequest != nil {
			bindingField, status = "pluginInput", "idle"
			binding["inputRequestId"] = inputRequest.ID
			binding["stepKey"] = inputRequest.StepKey
		}
		node := creationAddedNode(CreationCanvasOp{Type: "add_node", ID: id, NodeType: item.NodeType, Title: item.Title, X: &x, Y: &y, Metadata: map[string]any{"status": status, bindingField: binding}})
		bindings[item.Key] = id
		added = append(added, node)
		nodes = append(nodes, node)
	}
	doc["nodes"] = nodes
	edges := creationMaps(doc["connections"])
	for _, edge := range blueprint.Connections {
		addedEdge := pluginProjectionEdge(projectionID, edge.From, edge.To, bindings)
		for _, existingEdge := range edges {
			if existingEdge["id"] == addedEdge["id"] {
				return plugins.PreparedOperation{}, pluginProjectionConflict()
			}
		}
		edges = append(edges, addedEdge)
	}
	doc["connections"] = edges
	result, err := json.Marshal(map[string]any{"canvasId": canvas.ID, "sourceRunId": source.ID, "blueprintId": a.BlueprintID, "projectionId": projectionID, "bindings": bindings})
	if err != nil {
		return plugins.PreparedOperation{}, err
	}
	bindingJSON, _ := json.Marshal(bindings)
	nodesJSON, _ := json.Marshal(added)
	return plugins.PreparedOperation{Result: result, SourceDigest: a.ResultDigest, ProjectsToCanvas: true, Commit: func(tx *repository.Repository) error {
		policy, err := platform.New(tx, nil, nil).RuntimePolicy()
		if err != nil {
			return err
		}
		if err = saveCloudAgentDocument(tx, canvas, doc, policy); err != nil {
			if errors.Is(err, repository.ErrCreationConflict) || errors.Is(err, repository.ErrCanvasRevisionConflict) {
				return pluginProjectionConflict()
			}
			return err
		}
		return tx.CreatePluginCanvasProjection(&model.PluginCanvasProjection{ID: projectionID, UserID: userID, CanvasID: canvas.ID, SourceRunID: source.ID, ReleaseID: ctx.ReleaseID, BlueprintID: a.BlueprintID, BindingsJSON: string(bindingJSON), NodesJSON: string(nodesJSON), ResultJSON: string(result)})
	}}, nil
}

// Client hydration may add timestamps and local presentation metadata. Compare
// editable layout/title and the immutable binding, never overwrite other fields.
func pluginProjectionNodeHash(node map[string]any) string {
	value := map[string]any{}
	for _, key := range []string{"id", "type", "title", "position", "width", "height"} {
		value[key] = node[key]
	}
	meta, _ := node["metadata"].(map[string]any)
	value["pluginResult"] = meta["pluginResult"]
	if input, ok := meta["pluginInput"]; ok {
		value["pluginInput"] = input
	}
	return creationHash(value)
}

func pluginProjectionEdge(projectionID, from, to string, bindings map[string]string) map[string]any {
	return map[string]any{"id": "plugin-edge-" + creationHashRaw(projectionID + ":" + from + ":" + to)[:32], "fromNodeId": bindings[from], "toNodeId": bindings[to], "relation": "plugin-flow"}
}
func pluginProjectionEdgesIntact(doc map[string]any, bp contracts.CanvasBlueprint, rawBindings, projectionID string) bool {
	bindings := map[string]string{}
	if json.Unmarshal([]byte(rawBindings), &bindings) != nil {
		return false
	}
	for _, expected := range bp.Connections {
		want := pluginProjectionEdge(projectionID, expected.From, expected.To, bindings)
		found := false
		for _, edge := range creationMaps(doc["connections"]) {
			if edge["id"] == want["id"] && edge["fromNodeId"] == want["fromNodeId"] && edge["toNodeId"] == want["toNodeId"] && edge["relation"] == want["relation"] {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func creationHashRaw(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func (s *Service) PluginCanvasSnapshot(userID, canvasID string) (map[string]any, error) {
	if err := s.pluginOperationAccess(userID); err != nil {
		return nil, err
	}
	canvas, err := s.repo.CanvasProjectForUser(userID, canvasID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, kernel.NotFound("画布不存在")
	}
	if err != nil {
		return nil, err
	}
	doc, err := creationDocument(canvas.PayloadJSON)
	if err != nil {
		return nil, err
	}
	return map[string]any{"canvasId": canvas.ID, "snapshotHash": cloudAgentCanvasHash(doc)}, nil
}
