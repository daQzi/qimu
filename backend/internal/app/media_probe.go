package app

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"infinite-canvas/backend/internal/mediaanalysis"
	"infinite-canvas/backend/internal/model"
)

// Bounded shared media primitive: ownership is checked on each call, including
// requests from plugin views. It never accepts a URL or filesystem path.
var mediaProbeSlots = make(chan struct{}, 2)

func (s *Service) ProbeResource(ctx context.Context, user, id string) (mediaanalysis.Probe, error) {
	select {
	case mediaProbeSlots <- struct{}{}:
		defer func() { <-mediaProbeSlots }()
	default:
		return mediaanalysis.Probe{}, BadAuthRequest("媒体探测繁忙，请稍后重试")
	}
	resource, reader, err := s.OpenResource(user, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return mediaanalysis.Probe{}, NotFound("媒体不存在或不属于当前账号")
	}
	if err != nil {
		return mediaanalysis.Probe{}, err
	}
	defer reader.Close()
	if resource.Status != model.ResourceStatusReady || (resource.Kind != "video" && resource.Kind != "audio") || resource.Size > mediaanalysis.MaxMediaBytes {
		return mediaanalysis.Probe{}, BadAuthRequest("仅支持已就绪且不超过 512 MiB 的音视频")
	}
	result, err := mediaanalysis.ProbeReader(ctx, reader)
	if err != nil {
		return result, BadAuthRequest(err.Error())
	}
	return result, nil
}
func (s *Service) ResourceFrame(ctx context.Context, user, id string, atMs int64) ([]byte, error) {
	select {
	case mediaProbeSlots <- struct{}{}:
		defer func() { <-mediaProbeSlots }()
	default:
		return nil, BadAuthRequest("媒体抽帧繁忙，请稍后重试")
	}
	resource, reader, err := s.OpenResource(user, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, NotFound("媒体不存在或不属于当前账号")
	}
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	if resource.Status != model.ResourceStatusReady || resource.Kind != "video" || resource.Size > mediaanalysis.MaxMediaBytes || atMs < 0 || (resource.DurationMs > 0 && atMs >= resource.DurationMs) {
		return nil, BadAuthRequest("视频状态、大小或抽帧时间无效")
	}
	result, err := mediaanalysis.FrameReader(ctx, reader, atMs)
	if err != nil {
		return nil, BadAuthRequest(err.Error())
	}
	return result, nil
}
