package handler

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"infinite-canvas/backend/internal/auth"
	"infinite-canvas/backend/internal/database"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"
	"infinite-canvas/backend/internal/service"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestP04RemoteHTTP(t *testing.T) {
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	t.Setenv("REDIS_URL", "")
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"message":"real HTTP output"}`)) }))
	defer upstream.Close()
	db, err := database.Open(database.Config{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "p04.db")})
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	t.Cleanup(func() { sql.Close() })
	if err = database.MigrateSchema(db); err != nil {
		t.Fatal(err)
	}
	for _, u := range []model.User{{ID: "admin", Username: "admin", Role: model.UserRoleAdmin, Status: model.UserStatusActive}, {ID: "alice", Username: "alice", Role: model.UserRoleUser, Status: model.UserStatusActive}, {ID: "bob", Username: "bob", Role: model.UserRoleUser, Status: model.UserStatusActive}} {
		if err = db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
		if err = db.Create(&model.AuthSession{ID: u.ID, UserID: u.ID, TokenHash: auth.HashToken("p04-test"), ExpiresAt: time.Now().Add(time.Hour)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := service.New(repository.New(db), t.TempDir())
	t.Cleanup(func() { _ = svc.Close() })
	router := gin.New()
	RegisterPluginRoutes(router.Group("/api"), svc)
	encode := func(v any) []byte { raw, _ := json.Marshal(v); return raw }
	request := func(method, path, user string, body []byte, status int) map[string]any {
		t.Helper()
		req := httptest.NewRequest(method, "/api"+path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "p04-http-echo")
		if user != "" {
			req.AddCookie(&http.Cookie{Name: service.SessionCookieName, Value: user + ".p04-test"})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("%s %s got %d: %s", method, path, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "secret-never-return") {
			t.Fatal("credential leaked")
		}
		var envelope struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &envelope)
		return envelope.Data
	}
	pkg, err := os.ReadFile("../../../plugin-packages/application-examples/remote-helper-1.0.0.yingce-plugin")
	if err != nil {
		t.Fatal(err)
	}
	request("POST", "/plugins", "admin", pkg, 200)
	release, err := repository.New(db).PluginReleaseByVersion("remote-helper", "1.0.0")
	if err != nil || release == nil {
		t.Fatal(err)
	}
	request("PUT", "/plugins/applications/remote-helper/activation", "alice", encode(map[string]any{"releaseId": release.ID, "enabled": true, "grantedPermissions": []string{"connection.use", "resource.create"}, "revision": 0}), 200)
	request("PUT", "/plugin-connections", "alice", encode(map[string]any{"pluginId": "remote-helper", "connectorId": "api", "name": "Test", "baseUrl": upstream.URL, "credential": "secret-never-return", "enabled": true, "revision": 0}), 200)
	request("GET", "/plugin-connections", "", nil, 401)
	data := request("GET", "/plugin-connections", "bob", nil, 200)
	if len(data["connections"].([]any)) != 0 {
		t.Fatal("foreign connection visible")
	}
	request("PUT", "/admin/plugin-operation-prices", "alice", encode(map[string]any{"releaseId": release.ID, "operationId": "echo", "feeMicrocredits": 0, "revision": 0}), 403)
	call := encode(map[string]any{"releaseId": release.ID, "operation": "remote-helper.echo", "input": map[string]any{"prompt": "hello"}})
	created := request("POST", "/plugin-invocations", "alice", call, 200)
	duplicate := request("POST", "/plugin-invocations", "alice", call, 200)
	if created["runId"] != duplicate["runId"] {
		t.Fatal("HTTP replay duplicated run")
	}
	id := created["runId"].(string)
	request("GET", "/plugin-runs/"+id, "bob", nil, 404)
	request("POST", "/plugin-runs/"+id+"/approvals/"+created["approvalId"].(string), "alice", encode(map[string]any{"decision": "approve", "revision": created["revision"]}), 200)
	if err = svc.ProcessNextTask(); err != nil {
		t.Fatal(err)
	}
	result := request("GET", "/plugin-runs/"+id, "alice", nil, 200)
	if result["status"] != "succeeded" || result["result"].(map[string]any)["message"] != "real HTTP output" {
		t.Fatalf("result=%v", result)
	}
}
