package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"

	"infinite-canvas/backend/internal/mediaanalysis"
	"infinite-canvas/backend/internal/model"
)

func TestP11VideoWorkerProbeAndAudio(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
			if driver == "postgres" {
				t.Setenv("REDIS_URL", os.Getenv("CANVAS_TEST_REDIS_URL"))
			}
			s, db, release, report := p11Fixture(t, driver)
			dir := filepath.Join(s.dataDir, "resources")
			if err := os.MkdirAll(dir, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(dir, "probe.mp4")
			if out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=c=blue:s=128x72:r=25:d=10", "-f", "lavfi", "-i", "sine=frequency=500:duration=10", "-c:v", "mpeg4", "-c:a", "aac", "-shortest", path).CombinedOutput(); err != nil {
				t.Fatalf("fixture %v %s", err, out)
			}
			stat, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if err = db.Model(&model.Resource{}).Where("id=?", "video-one").Updates(map[string]any{"provider": "local", "object_key": "probe.mp4", "size": stat.Size()}).Error; err != nil {
				t.Fatal(err)
			}
			profile := DefaultModelCapabilityConfigForModel(string(model.ChannelInterfaceChatCompletion), "text-test")
			profile.Text.References.PromptMaxChars = 200000
			profile.Text.References.MaxVideos = 1
			profile.Text.References.MaxVideoBytes = 100 << 20
			profile.Text.References.VideoAudio = true
			if err = db.Model(&model.ChannelModel{}).Where("id=?", "cm").Update("capability_config_json", mustEncodeModelCapabilityConfig(t, profile)).Error; err != nil {
				t.Fatal(err)
			}
			probe, err := s.ProbeResource(context.Background(), "user", "video-one")
			if err != nil {
				t.Fatal(err)
			}
			report.Source = mediaanalysis.Source{ResourceID: "video-one", Digest: probe.Digest, DurationMs: probe.DurationMs}
			report.AudioAnalyzed = true
			report.Dialogue = []mediaanalysis.Utterance{{ID: "line1", Span: mediaanalysis.Span{StartMs: 0, EndMs: 1000}, Text: "模拟音轨识别", SpeakerID: "person1", EvidenceKind: "audio", Uncertainties: []string{"合成测试，不是真实识别"}}}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				payload, _ := json.Marshal(report)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": string(payload)}}}})
			}))
			defer server.Close()
			if err = db.Model(&model.ModelChannel{}).Where("id=?", "channel").Updates(map[string]any{"base_url": server.URL + "/v1", "api_key": "isolated"}).Error; err != nil {
				t.Fatal(err)
			}
			out, err := s.InvokePluginOperation("user", "probe-worker", p11Request(release, "analyze", report))
			if err != nil {
				t.Fatal(err)
			}
			view, err := s.PluginRun("user", out.RunID)
			if err != nil {
				t.Fatal(err)
			}
			view, err = s.DecidePluginRun("user", view.ID, view.ApprovalID, "approve", view.Revision)
			if err != nil {
				t.Fatal(err)
			}
			if err = s.ProcessNextTask(); err != nil {
				t.Fatal(err)
			}
			view, err = s.PluginRun("user", view.ID)
			if err != nil || view.Status != "succeeded" || calls.Load() != 1 {
				t.Fatalf("worker status=%s message=%s err=%v calls=%d", view.Status, view.FailureMessage, err, calls.Load())
			}
			if _, err = s.ResourceFrame(context.Background(), "other", "video-one", 0); err == nil {
				t.Fatal("foreign frame allowed")
			}
			if _, err = s.ResourceFrame(context.Background(), "user", "video-one", probe.DurationMs+1000); err == nil {
				t.Fatal("out of bounds frame allowed")
			}
			if err = db.Model(&model.PluginRun{}).Where("id=?", view.ID).Update("parent_run_id", "provenance-parent").Error; err != nil {
				t.Fatal(err)
			}
			resource, err := s.repo.ResourceForUser("user", "video-one")
			if err != nil {
				t.Fatal(err)
			}
			source, audio, err := pluginReportProvenance(s.repo, "user", "provenance-parent", resource)
			if err != nil || source != report.Source || !audio {
				t.Fatalf("provenance lost: %+v %v %v", source, audio, err)
			}
			if err = os.WriteFile(filepath.Join(dir, "invalid.mp4"), []byte("broken"), 0600); err != nil {
				t.Fatal(err)
			}
			if err = db.Model(&model.Resource{}).Where("id=?", "video-one").Update("object_key", "invalid.mp4").Error; err != nil {
				t.Fatal(err)
			}
			resource, _ = s.repo.ResourceForUser("user", "video-one")
			if _, _, err = pluginReportProvenance(s.repo, "user", "provenance-parent", resource); err == nil {
				t.Fatal("changed source accepted")
			}
			out, err = s.InvokePluginOperation("user", "invalid-probe-worker", p11Request(release, "analyze", report))
			if err != nil {
				t.Fatal(err)
			}
			view, err = s.PluginRun("user", out.RunID)
			if err != nil {
				t.Fatal(err)
			}
			view, err = s.DecidePluginRun("user", view.ID, view.ApprovalID, "approve", view.Revision)
			if err != nil {
				t.Fatal(err)
			}
			if err = s.ProcessNextTask(); err == nil {
				t.Fatal("broken media did not report preparation failure")
			}
			view, err = s.PluginRun("user", view.ID)
			if err != nil || view.Status != "failed" || calls.Load() != 1 {
				t.Fatalf("broken media reached provider: %s %v %d", view.Status, err, calls.Load())
			}
			var account model.CreditAccount
			if err = db.First(&account, "user_id=?", "user").Error; err != nil {
				t.Fatal(err)
			}
			if account.AvailableMicrocredits != 9900 {
				t.Fatalf("failed probe charged: %d", account.AvailableMicrocredits)
			}
		})
	}
}
