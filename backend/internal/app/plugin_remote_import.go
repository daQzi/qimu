package app

import (
	"context"
	"encoding/json"
	"errors"
	"image"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"
)

func (s *Service) importPluginArtifact(ctx context.Context, task model.Task, rawURL, kind, identity string) (*model.Resource, error) {
	if existing, err := s.resourceForUploadKey(task.UserID, normalizedResourceUploadKey([]string{identity})); err != nil {
		return nil, err
	} else if existing != nil && existing.Status == model.ResourceStatusReady {
		return existing, nil
	}
	if _, err := ValidateCustomRelayURL(rawURL); err != nil {
		return nil, errors.New("结果下载地址被出站策略拒绝")
	}
	policy, err := s.RuntimePolicy()
	if err != nil {
		return nil, err
	}
	limit := min(int64(64<<20), megabytes(policy.Resource.ResourceUploadMB))
	if limit <= 0 {
		return nil, errors.New("资源上传额度无效")
	}
	client := CustomRelayHTTPClient(45 * time.Second)
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 {
			return errors.New("结果下载重定向过多")
		}
		_, err := ValidateCustomRelayURL(req.URL.String())
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, errors.New("结果下载地址无效")
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, errors.New("结果下载失败，可查询原任务刷新结果地址")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, errors.New("结果不可下载或已过期，可查询原任务刷新地址")
	}
	file, err := os.CreateTemp("", "qimu-plugin-result-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	size, err := io.Copy(file, io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, errors.New("结果下载中断")
	}
	if size <= 0 || size > limit {
		return nil, errors.New("结果为空或超过资源大小限制")
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	width, height, duration, name, err := probePluginArtifact(ctx, file, kind)
	if err != nil {
		return nil, err
	}
	if _, err = file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	if err = s.repo.WithPluginTask(task, func(_ *repository.Repository, _ *model.Task, remote *model.PluginRemoteExecution, _ *model.PluginRun) error {
		if remote.CancelRequested {
			return repository.ErrTaskStateConflict
		}
		return nil
	}); err != nil {
		return nil, err
	}
	// Resource upload identity makes an imported immutable artifact reusable after
	// lease takeover. Run/Task/ledger publication still requires the live fence.
	return s.UploadResourceFile(task.UserID, name, size, kind, width, height, duration, file, identity)
}

func probePluginArtifact(ctx context.Context, file *os.File, kind string) (int, int, int64, string, error) {
	if kind == "image" {
		config, format, err := image.DecodeConfig(file)
		if err != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 100_000_000 {
			return 0, 0, 0, "", errors.New("结果不是有效的受支持图片")
		}
		return config.Width, config.Height, 0, "plugin-result." + format, nil
	}
	probe, err := exec.LookPath("ffprobe")
	if err != nil {
		return 0, 0, 0, "", errors.New("验证音视频结果需要 ffprobe，安装后可仅重试导入")
	}
	probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(probeCtx, probe, "-v", "error", "-protocol_whitelist", "file,pipe", "-format_whitelist", "mov,matroska,mp3,wav,aac,flac,ogg", "-show_entries", "stream=codec_type,width,height:format=duration,format_name", "-of", "json", file.Name()).Output()
	if err != nil || len(raw) > 64<<10 {
		return 0, 0, 0, "", errors.New("音视频结果探测失败")
	}
	var metadata struct {
		Streams []struct {
			Type   string `json:"codec_type"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
			Name     string `json:"format_name"`
		} `json:"format"`
	}
	if json.Unmarshal(raw, &metadata) != nil {
		return 0, 0, 0, "", errors.New("音视频元数据无效")
	}
	seconds, err := strconv.ParseFloat(metadata.Format.Duration, 64)
	if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 || seconds > 86400 {
		return 0, 0, 0, "", errors.New("音视频时长无效")
	}
	ext := "mp4"
	for _, candidate := range []string{"webm", "mp3", "wav", "aac", "flac", "ogg"} {
		if strings.Contains(metadata.Format.Name, candidate) {
			ext = candidate
			break
		}
	}
	for _, stream := range metadata.Streams {
		if stream.Type == kind {
			return stream.Width, stream.Height, int64(seconds * 1000), "plugin-result." + ext, nil
		}
	}
	return 0, 0, 0, "", errors.New("音视频内容与插件声明的类型不符")
}
