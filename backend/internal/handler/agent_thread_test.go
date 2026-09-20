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
	"path/filepath"
	"testing"
	"time"
)

func TestAgentThreadHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Open(database.Config{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "thread.db")})
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	t.Cleanup(func() { sql.Close() })
	if err = database.MigrateSchema(db); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"alice", "bob"} {
		if err = db.Create(&model.User{ID: id, Username: id, Role: model.UserRoleUser, Status: model.UserStatusActive}).Error; err != nil {
			t.Fatal(err)
		}
		if err = db.Create(&model.AuthSession{ID: id, UserID: id, TokenHash: auth.HashToken("thread-test-token"), ExpiresAt: time.Now().Add(time.Hour)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err = db.Create(&model.CanvasProject{ID: "canvas", UserID: "alice", PayloadJSON: `{"nodes":[]}`}).Error; err != nil {
		t.Fatal(err)
	}
	svc := service.New(repository.New(db), t.TempDir())
	previousRuntime := runtimeService
	ConfigureRuntime(svc)
	t.Cleanup(func() { ConfigureRuntime(previousRuntime) })
	router := gin.New()
	RegisterAgentRoutes(router.Group("/api"), svc)
	request := func(method, path, user, body string, status int) map[string]any {
		t.Helper()
		req := httptest.NewRequest(method, "/api"+path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		if user != "" {
			req.AddCookie(&http.Cookie{Name: service.SessionCookieName, Value: user + ".thread-test-token"})
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, res.Code, res.Body.String())
		}
		var envelope struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		return envelope.Data
	}
	request("GET", "/agent/threads", "", "", 401)
	request("POST", "/agent/threads", "alice", `{"clientKey":"http-thread","unexpected":1}`, 400)
	created := request("POST", "/agent/threads", "alice", `{"clientKey":"http-thread"}`, 200)
	id := created["thread"].(map[string]any)["id"].(string)
	path := "/agent/threads/" + id
	request("GET", path, "bob", "", 404)
	request("PATCH", path+"/context", "bob", `{"revision":1,"canvasId":"canvas"}`, 404)
	request("PATCH", path+"/context", "alice", `{"revision":1,"canvasId":"canvas"}`, 200)
	request("PATCH", path+"/context", "alice", `{"revision":1,"canvasId":""}`, 409)
	request("POST", path+"/messages", "alice", `{"revision":2,"request":{"threadId":"forged"}}`, 400)
	request("POST", path+"/messages", "alice", `{"revision":1,"request":{"idempotencyKey":"http-message"}}`, 409)
	request("POST", path+"/references", "alice", `{"revision":2,"clientKey":"foreign-ref","kind":"creation","runId":"missing"}`, 404)
	request("GET", path+"?before=bad", "alice", "", 400)
	view := request("GET", path, "alice", "", 200)
	if len(view["entries"].([]any)) != 0 || view["thread"].(map[string]any)["revision"].(float64) != 2 {
		t.Fatal("rejected writes changed thread")
	}
	request("POST", "/agent/threads", "alice", `{"clientKey":"http-thread","canvasId":"canvas"}`, 409)
}
