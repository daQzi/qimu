package app

import (
	"context"
	"encoding/json"
	"infinite-canvas/backend/internal/mediaanalysis"
	"infinite-canvas/backend/internal/model"
)

// Runs under the normal Task lease, before billing enters running and before
// any provider request. Resource I/O and subprocesses never hold a catalog lock.
func (s *Service) preparePluginModelProbe(ctx context.Context, task *model.Task) error {
	var input map[string]json.RawMessage
	if err := json.Unmarshal([]byte(task.InputJSON), &input); err != nil {
		return err
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal(input["metadata"], &metadata); err != nil {
		return err
	}
	var required bool
	_ = json.Unmarshal(metadata["pluginProbeRequired"], &required)
	if !required {
		return nil
	}
	if len(metadata["pluginMediaProbe"]) > 0 {
		return nil
	}
	var versions map[string]mediaanalysis.Source
	if err := json.Unmarshal(metadata["pluginModelSources"], &versions); err != nil {
		return err
	}
	source, ok := versions["resourceId"]
	if !ok {
		return BadAuthRequest("视频分析缺少来源")
	}
	probe, err := s.ProbeResource(ctx, task.UserID, source.ResourceID)
	if err != nil {
		return err
	}
	source.Digest, source.DurationMs = probe.Digest, probe.DurationMs
	metadata["pluginResourceVersions"] = metadata["pluginModelSources"]
	sources := map[string]mediaanalysis.Source{"resourceId": source}
	metadata["pluginModelSources"], _ = json.Marshal(sources)
	metadata["pluginMediaProbe"], _ = json.Marshal(probe)
	audio := false
	_ = json.Unmarshal(metadata["pluginAudioAllowed"], &audio)
	hasAudio := false
	hasVideo := false
	for _, stream := range probe.Streams {
		if stream.Kind == "video" {
			hasVideo = true
		}
		if stream.Kind == "audio" {
			hasAudio = true
		}
	}
	if !hasVideo {
		return BadAuthRequest("原片没有可分析的视频流")
	}
	if !hasAudio {
		audio = false
		metadata["pluginAudioAllowed"], _ = json.Marshal(false)
	}
	evidence, _ := json.Marshal(map[string]any{"sources": sources, "probe": probe, "audioAllowed": audio})
	task.Prompt += "\n执行前本地探测（以下 source 覆盖报价时资源版本指纹，digest 是实际文件 SHA-256；原片时间从播放起点算起）：" + string(evidence)
	if !audio {
		task.Prompt += "\n此次音轨不可分析，audioAnalyzed=false，只允许字幕证据并说明限制。"
	}
	input["prompt"], _ = json.Marshal(task.Prompt)
	input["metadata"], _ = json.Marshal(metadata)
	encoded, err := json.Marshal(input)
	if err != nil {
		return err
	}
	task.InputJSON = string(encoded)
	if err = validatePluginModelMedia(s.repo, task); err != nil {
		return err
	}
	if err = s.validatePluginModelDispatch(*task); err != nil {
		return err
	}
	return s.repo.SavePluginModelProbe(task)
}
