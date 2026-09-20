package app

import (
	"encoding/json"
	"infinite-canvas/backend/internal/mediaanalysis"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/repository"
)

func validatePluginMediaInput(repo *repository.Repository, user, runID, profile string, invocation contracts.Invocation, raw json.RawMessage) error {
	var input struct {
		Report json.RawMessage    `json:"report"`
		Plan   mediaanalysis.Plan `json:"plan"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		return BadAuthRequest("人工修正数据格式无效")
	}
	var id string
	if json.Unmarshal(invocation.Input["resourceId"], &id) != nil || id == "" {
		return BadAuthRequest("流程缺少原视频资源")
	}
	resource, err := repo.LockResourceForUser(user, id)
	if err != nil {
		return err
	}
	if resource.Status != model.ResourceStatusReady || resource.Kind != "video" {
		return BadAuthRequest("原视频尚未就绪")
	}
	report, err := mediaanalysis.Decode(input.Report, pluginVideoSource(resource))
	if err != nil {
		return BadAuthRequest(err.Error())
	}
	if report.AudioAnalyzed {
		return BadAuthRequest("当前流程尚未核验音轨理解能力，不能标记为已分析音轨")
	}
	switch profile {
	case "video-report/v1":
		return repo.AddPluginRunResources(user, runID, "input", []string{id})
	case "video-plan/v1":
		if err = mediaanalysis.ValidatePlan(input.Plan, report); err != nil {
			return BadAuthRequest(err.Error())
		}
		ids := []string{id}
		for _, item := range input.Plan.Mappings {
			if item.MaterialID == "" {
				continue
			}
			material, err := repo.LockResourceForUser(user, item.MaterialID)
			if err != nil {
				return err
			}
			if material.Status != model.ResourceStatusReady {
				return BadAuthRequest("替换素材尚未就绪")
			}
			if item.Kind == "voice" && material.Kind != "audio" {
				return BadAuthRequest("语音替换须引用音频素材")
			}
			if item.Kind != "voice" && material.Kind != "image" && material.Kind != "video" {
				return BadAuthRequest("视觉替换须引用图片或视频素材")
			}
			ids = append(ids, item.MaterialID)
		}
		return repo.AddPluginRunResources(user, runID, "input", ids)
	default:
		return BadAuthRequest("宿主未提供该输入校验器")
	}
}
