package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"infinite-canvas/backend/internal/auth"
	"infinite-canvas/backend/internal/database"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"
	"infinite-canvas/backend/internal/service"
)

func TestP03ProjectionHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Open(database.Config{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "http.db")})
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	t.Cleanup(func() { sql.Close() })
	if err = database.MigrateSchema(db); err != nil {
		t.Fatal(err)
	}
	for _, user := range []model.User{{ID: "admin", Username: "p03-admin", Role: model.UserRoleAdmin, Status: model.UserStatusActive}, {ID: "alice", Username: "p03-alice", Role: model.UserRoleUser, Status: model.UserStatusActive}, {ID: "bob", Username: "p03-bob", Role: model.UserRoleUser, Status: model.UserStatusActive}} {
		if err = db.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
		if err = db.Create(&model.AuthSession{ID: user.ID, UserID: user.ID, TokenHash: auth.HashToken("p03-token"), ExpiresAt: time.Now().Add(time.Hour)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, row := range []any{&model.Resource{ID: "video", UserID: "alice", Kind: "video", Status: model.ResourceStatusReady, MimeType: "video/mp4"}, &model.CanvasProject{ID: "canvas", UserID: "alice", Title: "P03", PayloadJSON: `{"id":"canvas","nodes":[],"connections":[]}`}} {
		if err = db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := service.New(repository.New(db), t.TempDir())
	router := gin.New()
	RegisterPluginRoutes(router.Group("/api"), svc)
	sequence := 0
	request := func(method, path, user string, body []byte, status int) map[string]any {
		t.Helper()
		sequence++
		req := httptest.NewRequest(method, "/api"+path, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", fmt.Sprintf("p03-http-%d", sequence))
		if user != "" {
			req.AddCookie(&http.Cookie{Name: service.SessionCookieName, Value: user + ".p03-token"})
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, res.Code, res.Body.String())
		}
		var envelope struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(res.Body.Bytes(), &envelope)
		return envelope.Data
	}
	encode := func(v any) []byte {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	pkg, err := os.ReadFile("../../../plugin-packages/application-examples/resource-helper-1.3.0.yingce-plugin")
	if err != nil {
		t.Fatal(err)
	}
	request("POST", "/plugins", "admin", pkg, 200)
	release, err := repository.New(db).PluginReleaseByVersion("resource-helper", "1.3.0")
	if err != nil || release == nil {
		t.Fatal(err)
	}
	request("PUT", "/plugins/applications/resource-helper/activation", "alice", encode(map[string]any{"releaseId": release.ID, "enabled": true, "grantedPermissions": []string{"media.read", "resource.create", "canvas.read", "canvas.write"}, "revision": 0}), 200)
	invoke := func(op string, input map[string]any) map[string]any {
		return request("POST", "/plugin-invocations", "alice", encode(map[string]any{"operation": "resource-helper." + op, "releaseId": release.ID, "input": input, "context": map[string]any{"hostSurface": "canvas", "canvasId": "canvas"}}), 200)
	}
	approve := func(out map[string]any) map[string]any {
		return request("POST", fmt.Sprintf("/plugin-runs/%s/approvals/%s", out["runId"], out["approvalId"]), "alice", encode(map[string]any{"decision": "approve", "revision": out["revision"]}), 200)
	}
	read := invoke("inspect-video", map[string]any{"resourceId": "video"})
	saved := approve(invoke("snapshot-video", map[string]any{"resourceId": "video", "expectedDigest": read["digest"]}))
	if saved["view"] == nil {
		t.Fatal("result view missing")
	}
	snapshot := request("GET", "/plugin-canvases/canvas/snapshot", "alice", nil, 200)
	request("GET", "/plugin-canvases/canvas/snapshot", "", nil, 401)
	request("GET", "/plugin-canvases/canvas/snapshot", "bob", nil, 404)
	request("GET", "/plugin-runs/"+saved["id"].(string), "bob", nil, 404)
	ref := saved["resultRef"].(map[string]any)
	projected := approve(invoke("place-result", map[string]any{"runId": saved["id"], "resultDigest": ref["digest"], "blueprintId": "inspect-board", "snapshotHash": snapshot["snapshotHash"], "instanceKey": "default"}))
	if projected["status"] != "succeeded" || projected["projectionStatus"] != "applied" {
		t.Fatalf("projection failed: %v", projected)
	}
	canvas, err := repository.New(db).CanvasProjectForUser("alice", "canvas")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Nodes []struct {
			Type string `json:"type"`
		} `json:"nodes"`
	}
	if err = json.Unmarshal([]byte(canvas.PayloadJSON), &doc); err != nil || len(doc.Nodes) != 1 || doc.Nodes[0].Type != "plugin-result" {
		t.Fatalf("node not saved: %s %v", canvas.PayloadJSON, err)
	}
}
