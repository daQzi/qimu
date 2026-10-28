package protocol

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestLxmoneTextAndAgentContracts(t *testing.T) {
	for _, tc := range []struct{ id, path, payload, text string }{
		{"lxmone-chat", "/v1/chat/completions", `{"choices":[{"message":{"content":"answer","reasoning_content":"reason"}}]}`, "answer"},
		{"lxmone-claude", "/v1/messages", `{"content":[{"type":"text","text":"answer"}]}`, "answer"},
	} {
		t.Run(tc.id, func(t *testing.T) {
			adapter := officialPackageAdapter(t, "lxmone-video-suite.yingce-plugin", tc.id)
			spec, err := adapter.BuildCreate(context.Background(), RequestContext{Request: GenerationRequest{Model: "model-test", Prompt: "hello", Instructions: "system", Extra: map[string]any{"max_tokens": 512}}})
			if err != nil {
				t.Fatal(err)
			}
			body := manifestTestBody(t, spec)
			if spec.Path != tc.path || body["model"] != "model-test" || body["max_tokens"] != float64(512) {
				t.Fatalf("request = %#v, body = %#v", spec, body)
			}
			messages := body["messages"].([]any)
			if tc.id == "lxmone-chat" && len(messages) != 2 || tc.id == "lxmone-claude" && (len(messages) != 1 || body["system"] != "system" || spec.Auth.Type != "anthropic" || spec.Headers["anthropic-version"] == "") {
				t.Fatalf("system instructions/auth not mapped: %#v, %#v", body, spec)
			}
			created, err := adapter.ParseCreate(context.Background(), []byte(tc.payload))
			if err != nil || created.Status != StatusSucceeded || created.Result == nil || created.Result.Text != tc.text {
				t.Fatalf("response = %#v, %v", created, err)
			}
			if !adapter.(AgentCapability).AgentAvailable() {
				t.Fatal("text provider must expose the Agent contract")
			}
			agent := adapter.(AgentAdapter)
			spec, err = agent.BuildAgent(context.Background(), AgentRequestContext{Model: "model-test", Request: map[string]any{
				"chatCompletion": map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hello"}}, "tools": []any{map[string]any{"type": "function"}}},
				"claude":         map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hello"}}, "max_tokens": 512, "tools": []any{map[string]any{"name": "search"}}},
			}})
			if err != nil || spec.Path != tc.path || manifestTestBody(t, spec)["tools"] == nil {
				t.Fatalf("agent request = %#v, %v", spec, err)
			}
		})
	}
}

func TestLxmoneImageOperations(t *testing.T) {
	for _, id := range []string{"lxmone-images", "lxmone-images-async"} {
		t.Run(id, func(t *testing.T) {
			adapter := officialPackageAdapter(t, "lxmone-video-suite.yingce-plugin", id)
			suffix := ""
			if id == "lxmone-images-async" {
				suffix = "/async"
			}
			request := GenerationRequest{Model: "gpt-image-2a", Prompt: "poster"}
			spec, err := adapter.BuildCreate(context.Background(), RequestContext{Request: request})
			if err != nil || spec.Path != "/v1/images/generations"+suffix || spec.ContentType != "application/json" || len(spec.Files) != 0 {
				t.Fatalf("generation = %#v, %v", spec, err)
			}
			body := manifestTestBody(t, spec)
			if body["size"] != "1024x1024" || body["n"] != float64(1) || body["output_format"] != nil {
				t.Fatalf("generation body = %#v", body)
			}
			request.Images = []MediaReference{{URL: "https://cdn.example/b.png", Order: 2}, {DataURL: "data:image/png;base64,QUJD", Order: 1, MIMEType: "image/png"}}
			spec, err = adapter.BuildCreate(context.Background(), RequestContext{Request: request})
			if err != nil || spec.Path != "/v1/images/edits"+suffix || spec.ContentType != "multipart/form-data" || len(spec.Files) != 2 {
				t.Fatalf("edit = %#v, %v", spec, err)
			}
			if spec.Files[0].Name != "image[]" || spec.Files[0].Reference.DataURL != request.Images[1].DataURL || spec.Files[1].Reference.URL != request.Images[0].URL {
				t.Fatalf("edit files out of order: %#v", spec.Files)
			}
			request.Images = append(request.Images, request.Images...)
			if _, err := adapter.BuildCreate(context.Background(), RequestContext{Request: request}); err == nil {
				t.Fatal("accepted more than three source images")
			}
			request.Images = nil
			request.Model, request.ImageCount = "Seedream-5-Pro", 2
			if _, err := adapter.BuildCreate(context.Background(), RequestContext{Request: request}); err == nil {
				t.Fatal("accepted n=2 for a single-image-only model")
			}
		})
	}
}

func TestLxmoneImageResponses(t *testing.T) {
	adapter := officialPackageAdapter(t, "lxmone-video-suite.yingce-plugin", "lxmone-images")
	created, err := adapter.ParseCreate(context.Background(), []byte(`{"data":[{"url":"https://cdn.example/a.png"},{"b64_json":"QUJD"}]}`))
	if err != nil || created.Status != StatusSucceeded || created.Result == nil || len(created.Result.Images) != 2 || created.Result.Images[1].DataURL != "data:image/png;base64,QUJD" {
		t.Fatalf("sync images = %#v, %v", created, err)
	}
	async := officialPackageAdapter(t, "lxmone-video-suite.yingce-plugin", "lxmone-images-async")
	created, err = async.ParseCreate(context.Background(), []byte(`{"task_id":"imgtask-1","status":"processing"}`))
	if err != nil || created.TaskID != "imgtask-1" || created.Status != StatusProcessing {
		t.Fatalf("async create = %#v, %v", created, err)
	}
	spec, err := async.BuildPoll(context.Background(), PollContext{TaskID: created.TaskID})
	if err != nil || spec.Path != "/v1/images/tasks/imgtask-1" {
		t.Fatalf("async poll = %#v, %v", spec, err)
	}
	for _, payload := range []string{
		`{"status":"completed","image_url":"https://cdn.example/a.png","result":{"data":[{"url":"https://cdn.example/a.png"},{"url":"https://cdn.example/b.png"}]}}`,
		`{"status":"completed","image_url":"https://cdn.example/a.png"}`,
	} {
		polled, err := async.ParsePoll(context.Background(), PollContext{TaskID: "imgtask-1"}, []byte(payload))
		if err != nil || polled.Status != StatusSucceeded || polled.Result == nil || polled.Result.Images[0].URL != "https://cdn.example/a.png" || !polled.Result.Images[0].Ephemeral {
			t.Fatalf("async result = %#v, %v", polled, err)
		}
	}
	for _, id := range []string{"lxmone-chat", "lxmone-claude", "lxmone-images", "lxmone-images-async", "lxmone-video-current"} {
		adapter := officialPackageAdapter(t, "lxmone-video-suite.yingce-plugin", id)
		failed, err := adapter.ParseCreate(context.Background(), []byte(`{"error":{"message":"denied"}}`))
		if err != nil || failed.Status != StatusFailed || failed.Message != "denied" {
			t.Fatalf("%s swallowed error: %#v, %v", id, failed, err)
		}
	}
}

// Fixtures are public request examples from the upstream docs catalog, not live paid calls.
func TestLxmoneCurrentVideoMatchesPublishedExamples(t *testing.T) {
	data, err := os.ReadFile("testdata/lxmone-video-examples.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Model   string         `json:"model"`
		Path    string         `json:"path"`
		Payload map[string]any `json:"payload"`
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	adapter := officialPackageAdapter(t, "lxmone-video-suite.yingce-plugin", "lxmone-video-current")
	for _, fixture := range fixtures {
		t.Run(fixture.Model, func(t *testing.T) {
			body := fixture.Payload
			request := GenerationRequest{Model: fixture.Model, Prompt: "test"}
			for _, key := range []string{"duration", "duration_seconds", "seconds"} {
				if value, ok := body[key].(float64); ok {
					request.Duration = int(value)
				}
			}
			request.Resolution, _ = body["resolution"].(string)
			request.AspectRatio, _ = body["aspect_ratio"].(string)
			if value, ok := body["ratio"].(string); ok {
				request.AspectRatio = value
			}
			request.GenerateAudio, _ = body["generate_audio"].(bool)
			for _, entry := range []struct {
				keys []string
				out  *[]MediaReference
			}{
				{[]string{"reference_images", "referenceImages", "image_urls"}, &request.Images},
				{[]string{"reference_videos", "referenceVideos", "video_urls", "videos"}, &request.Videos},
				{[]string{"reference_audios", "referenceAudios", "audio_urls", "audios"}, &request.Audios},
			} {
				for _, key := range entry.keys {
					for _, raw := range arrayValue(body[key]) {
						value, _ := raw.(string)
						if obj, ok := raw.(map[string]any); ok {
							value, _ = obj["url"].(string)
						}
						*entry.out = append(*entry.out, MediaReference{URL: value})
					}
				}
			}
			for _, raw := range arrayValue(body["assets"]) {
				asset := raw.(map[string]any)
				value := MediaReference{URL: asset["url"].(string)}
				if asset["category"] == "image" {
					request.Images = append(request.Images, value)
				} else {
					request.Audios = append(request.Audios, value)
				}
			}
			if input, ok := body["input"].(map[string]any); ok {
				for _, raw := range arrayValue(input["media"]) {
					item := raw.(map[string]any)
					value := MediaReference{URL: item["url"].(string)}
					switch item["type"] {
					case "reference_image":
						request.Images = append(request.Images, value)
					case "reference_video":
						request.Videos = append(request.Videos, value)
					case "reference_audio":
						request.Audios = append(request.Audios, value)
					}
				}
			}
			spec, err := adapter.BuildCreate(context.Background(), RequestContext{Request: request})
			if err != nil {
				t.Fatal(err)
			}
			actual := manifestTestBody(t, spec)
			if spec.Path != fixture.Path {
				t.Fatalf("path = %s, want %s", spec.Path, fixture.Path)
			}
			for key, expected := range body {
				if key == "prompt" || key == "input" {
					continue
				}
				if !reflect.DeepEqual(actual[key], expected) {
					t.Fatalf("%s = %#v, want %#v", key, actual[key], expected)
				}
			}
		})
	}
}

func TestLxmoneCurrentVideoRejectsUnsupportedInputsAndDownloadsWithAuth(t *testing.T) {
	adapter := officialPackageAdapter(t, "lxmone-video-suite.yingce-plugin", "lxmone-video-current")
	for _, request := range []GenerationRequest{
		{Model: "unknown-model"},
		{Model: "minimax-h3-b", Prompt: "missing image"},
		{Model: "grok-imagine-video-1.5-lite", Videos: []MediaReference{{URL: "https://cdn.example/a.mp4"}}},
		{Model: "seedance2.5", Duration: 31},
		{Model: "sd2.0-480p", Resolution: "1080p"},
	} {
		if _, err := adapter.BuildCreate(context.Background(), RequestContext{Request: request}); err == nil {
			t.Fatalf("accepted unsupported request: %#v", request)
		}
	}
	polled, err := adapter.ParsePoll(context.Background(), PollContext{TaskID: "video-1"}, []byte(`{"status":"completed","metadata":{"url":"/v1/videos/video-1/content"}}`))
	if err != nil || polled.Status != StatusSucceeded || polled.Result != nil {
		t.Fatalf("relative URL must use authenticated result: %#v, %v", polled, err)
	}
	spec, err := adapter.(ResultAdapter).BuildResult(context.Background(), PollContext{TaskID: "video-1"})
	if err != nil || spec.Path != "/v1/videos/video-1/content" || spec.Auth.Type != "bearer" {
		t.Fatalf("result = %#v, %v", spec, err)
	}
}
