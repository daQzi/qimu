package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"yingce/backend/internal/protocol"
)

func TestLxmonePluginTextImageAndVideoTransport(t *testing.T) {
	allowLoopbackProviderTest(t)
	seen := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen[r.URL.Path]++
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing upstream authentication")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/chat/completions":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["model"] != "deepseek-v4.1-flash" {
				t.Errorf("text request = %#v, %v", body, err)
			}
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"hello"}}]}`)
		case "/v1/images/edits":
			if err := r.ParseMultipartForm(1 << 20); err != nil {
				t.Error(err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if r.FormValue("model") != "gpt-image-2a" || r.FormValue("n") != "1" || r.FormValue("size") != "1024x1024" || len(r.MultipartForm.File["image[]"]) != 2 {
				t.Errorf("multipart values = %#v, files = %#v", r.MultipartForm.Value, r.MultipartForm.File)
			}
			for i, file := range r.MultipartForm.File["image[]"] {
				f, err := file.Open()
				if err != nil {
					t.Error(err)
					continue
				}
				data, err := io.ReadAll(f)
				_ = f.Close()
				if err != nil || string(data) != []string{"hello", "world"}[i] {
					t.Errorf("image %d = %q, %v", i, data, err)
				}
			}
			_, _ = io.WriteString(w, `{"data":[{"b64_json":"aGVsbG8="}]}`)
		case "/v1/videos":
			_, _ = io.WriteString(w, `{"id":"video-1","status":"queued"}`)
		case "/v1/videos/video-1":
			_, _ = io.WriteString(w, `{"id":"video-1","status":"completed","metadata":{"url":"/v1/videos/video-1/content"}}`)
		case "/v1/videos/video-1/content":
			w.Header().Set("Content-Type", "video/mp4")
			_, _ = io.WriteString(w, "video-bytes")
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	for _, tc := range []struct{ mode, provider, model string }{
		{"text", "lxmone-chat", "deepseek-v4.1-flash"},
		{"image", "lxmone-images", "gpt-image-2a"},
		{"video", "lxmone-video-current", "seedance2.5"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			adapter, ok := loadOfficialFallbackRegistry().Resolve(tc.provider)
			if !ok {
				t.Fatal("missing packaged provider")
			}
			input := canvasGenerationInput{Mode: tc.mode, Prompt: "test", Config: providerConfig{BaseURL: server.URL + "/v1", APIKey: "test-key", Model: tc.model, InterfaceType: tc.provider}}
			if tc.mode == "image" {
				input.ReferenceImages = []providerMedia{{DataURL: "data:image/png;base64,aGVsbG8=", Type: "image/png"}, {DataURL: "data:image/png;base64,d29ybGQ=", Type: "image/png"}}
			}
			result, err := runProtocolAdapterTaskWithPolicy(context.Background(), input, adapter, fastVideoPollPolicy())
			if err != nil || result["mode"] != tc.mode {
				t.Fatalf("result = %#v, %v", result, err)
			}
			if tc.mode == "video" {
				video, _ := result["video"].(map[string]interface{})
				dataURL, _ := video["dataUrl"].(string)
				if !strings.HasPrefix(dataURL, "data:video/mp4;base64,") {
					t.Fatalf("authenticated video result = %#v", result)
				}
			}
		})
	}
	for _, path := range []string{"/v1/chat/completions", "/v1/images/edits", "/v1/videos", "/v1/videos/video-1", "/v1/videos/video-1/content"} {
		if seen[path] != 1 {
			t.Errorf("%s calls = %d, want 1", path, seen[path])
		}
	}
}

func TestLxmoneAgentUsesKnownWireOutputLimits(t *testing.T) {
	allowLoopbackProviderTest(t)
	for _, id := range []string{"lxmone-chat", "lxmone-claude"} {
		t.Run(id, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["max_tokens"] != float64(128) {
					t.Errorf("Agent output limit was not applied: %#v, %v", body, err)
				}
				w.Header().Set("Content-Type", "application/json")
				if id == "lxmone-chat" {
					_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"done","tool_calls":[{"id":"call-1","function":{"name":"search","arguments":"{}"}}]}}]}`)
				} else {
					if r.Header.Get("x-api-key") != "test-key" || r.Header.Get("anthropic-version") != "2023-06-01" {
						t.Error("missing Claude authentication headers")
					}
					_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"done"},{"type":"tool_use","id":"call-1","name":"search","input":{}}]}`)
				}
			}))
			defer server.Close()
			adapter, ok := loadOfficialFallbackRegistry().Resolve(id)
			if !ok {
				t.Fatal("missing Agent provider")
			}
			input := canvasGenerationInput{
				Config:      providerConfig{BaseURL: server.URL + "/v1", APIKey: "test-key", Model: "model-test", InterfaceType: id},
				TextOptions: canvasTextOptions{MaxOutputTokens: 128},
				AgentRequests: &agentToolRequests{
					ChatCompletion: map[string]interface{}{"messages": []any{}, "max_tokens": 512},
					Claude:         map[string]interface{}{"messages": []any{}, "max_tokens": 512},
				},
			}
			result, err := runDeclarativeAgentTask(context.Background(), input, adapter.(protocol.AgentAdapter))
			if err != nil || result["text"] != "done" || calls != 1 {
				t.Fatalf("Agent result = %#v, %v; calls = %d", result, err, calls)
			}
		})
	}
}
