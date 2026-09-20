package mediaanalysis

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestProbeAndFrame(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	path := os.Getenv("CANVAS_TEST_LOCAL_VIDEO")
	if path == "" {
		path = filepath.Join(t.TempDir(), "sample.mp4")
		if out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=c=blue:s=128x72:r=25:d=2", "-f", "lavfi", "-i", "sine=frequency=500:duration=2", "-c:v", "mpeg4", "-c:a", "aac", "-shortest", path).CombinedOutput(); err != nil {
			t.Fatalf("generate fixture: %v %s", err, out)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	probe, err := ProbeReader(context.Background(), bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if probe.Digest != fmt.Sprintf("%x", sha256.Sum256(data)) || probe.DurationMs <= 0 || len(probe.Streams) < 2 {
		t.Fatalf("bad probe: %+v", probe)
	}
	for _, stream := range probe.Streams {
		if stream.TimeBase == "" {
			t.Fatal("time base missing")
		}
	}
	for _, at := range []int64{0, probe.DurationMs / 2, probe.DurationMs - 100} {
		frame, err := FrameReader(context.Background(), bytes.NewReader(data), at)
		if err != nil || !bytes.HasPrefix(frame, []byte{0xff, 0xd8}) {
			t.Fatalf("frame %d: %v", at, err)
		}
	}
	t.Logf("duration=%dms digest=%s streams=%d", probe.DurationMs, probe.Digest, len(probe.Streams))
	if _, err = ProbeReader(context.Background(), bytes.NewReader([]byte("not a video"))); err == nil {
		t.Fatal("invalid video accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = ProbeReader(ctx, bytes.NewReader(data)); err == nil {
		t.Fatal("cancel ignored")
	}
}
