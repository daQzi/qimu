package mediaanalysis

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"
)

const MaxMediaBytes int64 = 512 << 20

type Stream struct {
	Index      int    `json:"index"`
	Codec      string `json:"codec_name"`
	Kind       string `json:"codec_type"`
	TimeBase   string `json:"time_base"`
	StartTime  string `json:"start_time"`
	Duration   string `json:"duration"`
	FrameRate  string `json:"avg_frame_rate"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	SampleRate string `json:"sample_rate"`
	Channels   int    `json:"channels"`
}
type Probe struct {
	Digest     string   `json:"digest"`
	DurationMs int64    `json:"durationMs"`
	Streams    []Stream `json:"streams"`
	StartTime  string   `json:"startTime"`
}

// All input is a host-owned local copy. Neither playlist network references nor
// plugin supplied command arguments can reach ffmpeg/ffprobe.
func localMedia(ctx context.Context, reader io.Reader, fn func(string, string) error) error {
	dir, err := os.MkdirTemp("", "qimu-media-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "source")
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(file, hash), io.LimitReader(reader, MaxMediaBytes+1))
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if n == 0 || n > MaxMediaBytes {
		return fmt.Errorf("媒体为空或超过 512 MiB 探测上限")
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return fn(path, hex.EncodeToString(hash.Sum(nil)))
}

type boundedBuffer struct {
	bytes.Buffer
	max int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.max-b.Len() {
		return 0, fmt.Errorf("媒体工具输出超出上限")
	}
	return b.Buffer.Write(p)
}
func command(ctx context.Context, tool string, args ...string) ([]byte, error) {
	bin, err := exec.LookPath(tool)
	if err != nil {
		return nil, fmt.Errorf("媒体分析需要安装 %s", tool)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	output := &boundedBuffer{max: 4 << 20}
	cmd.Stdout = output
	stderr := &boundedBuffer{max: 16 << 10}
	cmd.Stderr = stderr
	if err = cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%s 无法处理该媒体（文件损坏、格式不支持或资源限制）", tool)
	}
	return output.Bytes(), nil
}
func ProbeReader(ctx context.Context, reader io.Reader) (Probe, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	var result Probe
	err := localMedia(ctx, reader, func(path, digest string) error {
		data, err := command(ctx, "ffprobe", "-v", "error", "-protocol_whitelist", "file", "-format_whitelist", "mov,matroska,webm,avi,flv,mpeg,mpegts,ogg,wav,mp3,flac,aac", "-show_streams", "-show_format", "-of", "json", path)
		if err != nil {
			return err
		}
		var raw struct {
			Streams []Stream `json:"streams"`
			Format  struct {
				Duration  string `json:"duration"`
				StartTime string `json:"start_time"`
			} `json:"format"`
		}
		if err = json.Unmarshal(data, &raw); err != nil {
			return err
		}
		seconds, err := strconv.ParseFloat(raw.Format.Duration, 64)
		if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 || seconds > 6*3600 {
			return fmt.Errorf("无法确认时长或媒体超过六小时")
		}
		if len(raw.Streams) == 0 || len(raw.Streams) > 32 {
			return fmt.Errorf("媒体流数量无效")
		}
		result = Probe{Digest: digest, DurationMs: int64(math.Round(seconds * 1000)), Streams: raw.Streams, StartTime: raw.Format.StartTime}
		return nil
	})
	return result, err
}

// Frame is a preview at the requested presentation timestamp. The timestamp is
// not advertised as an exact decoded-frame PTS on variable frame rate sources.
func FrameReader(ctx context.Context, reader io.Reader, atMs int64) ([]byte, error) {
	if atMs < 0 || atMs > 6*3600*1000 {
		return nil, fmt.Errorf("抽帧时间无效")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	var output []byte
	err := localMedia(ctx, reader, func(path, _ string) error {
		var err error
		output, err = command(ctx, "ffmpeg", "-nostdin", "-v", "error", "-threads", "1", "-protocol_whitelist", "file", "-format_whitelist", "mov,matroska,webm,avi,flv,mpeg,mpegts,ogg", "-ss", strconv.FormatFloat(float64(atMs)/1000, 'f', 3, 64), "-i", path, "-map", "0:v:0", "-frames:v", "1", "-vf", "scale=w=640:h=640:force_original_aspect_ratio=decrease", "-f", "image2pipe", "-c:v", "mjpeg", "pipe:1")
		if err == nil && len(output) == 0 {
			return fmt.Errorf("该时间没有可用画面")
		}
		return err
	})
	return output, err
}
