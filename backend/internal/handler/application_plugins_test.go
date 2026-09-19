package handler

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"infinite-canvas/backend/internal/auth"
	"infinite-canvas/backend/internal/database"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/repository"
	"infinite-canvas/backend/internal/service"
)

func TestP01ApplicationPluginHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Open(database.Config{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "api.db")})
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	t.Cleanup(func() { sql.Close() })
	if err = database.MigrateSchema(db); err != nil {
		t.Fatal(err)
	}
	for _, user := range []model.User{{ID: "admin", Username: "admin", Role: model.UserRoleAdmin, Status: model.UserStatusActive}, {ID: "alice", Username: "alice", Role: model.UserRoleUser, Status: model.UserStatusActive}, {ID: "bob", Username: "bob", Role: model.UserRoleUser, Status: model.UserStatusActive}} {
		if err = db.Create(&user).Error; err != nil {
			t.Fatal(err)
		}
		if err = db.Create(&model.AuthSession{ID: user.ID, UserID: user.ID, TokenHash: auth.HashToken("test-token"), ExpiresAt: time.Now().Add(time.Hour)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := service.New(repository.New(db), t.TempDir())
	router := gin.New()
	api := router.Group("/api")
	RegisterPluginRoutes(api, svc)
	RegisterSkillRoutes(api, svc)
	request := func(method, p, user string, body []byte) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, p, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if user != "" {
			req.AddCookie(&http.Cookie{Name: service.SessionCookieName, Value: user + ".test-token"})
		}
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	root := "../plugins/contracts/testdata/resource-helper"
	err = filepath.WalkDir(root, func(name string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		b, e := os.ReadFile(name)
		if e != nil {
			return e
		}
		entry, e := zw.Create(strings.TrimPrefix(filepath.ToSlash(name), root+"/"))
		if e != nil {
			return e
		}
		_, e = entry.Write(b)
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	zw.Close()
	for _, user := range []string{"", "alice"} {
		response := request("POST", "/api/plugins", user, archive.Bytes())
		if response.Code != 401 && response.Code != 403 {
			t.Fatalf("unauthorized install: %d %s", response.Code, response.Body.String())
		}
	}
	response := request("POST", "/api/plugins", "admin", archive.Bytes())
	if response.Code != 200 {
		t.Fatalf("install: %d %s", response.Code, response.Body.String())
	}
	var release model.PluginRelease
	if err = db.First(&release, "plugin_id=?", "resource-helper").Error; err != nil {
		t.Fatal(err)
	}
	response = request("GET", "/api/plugins/applications", "alice", nil)
	if response.Code != 200 || !strings.Contains(response.Body.String(), release.ID) {
		t.Fatalf("catalog: %s", response.Body.String())
	}
	body, _ := json.Marshal(map[string]any{"releaseId": release.ID, "enabled": true, "grantedPermissions": []string{"media.read"}, "revision": 0})
	for _, bad := range []string{`{"releaseId":"` + release.ID + `","enabled":true,"grantedPermissions":[],"revision":0,"userId":"bob"}`, `{"releaseId":"` + release.ID + `","enabled":true,"grantedPermissions":[]}`, `{"releaseId":"` + release.ID + `","enabled":true,"enabled":false,"grantedPermissions":[],"revision":0}`} {
		response = request("PUT", "/api/plugins/applications/resource-helper/activation", "alice", []byte(bad))
		if response.Code != 400 {
			t.Fatalf("invalid activation accepted %d %s", response.Code, response.Body.String())
		}
	}
	response = request("PUT", "/api/plugins/applications/resource-helper/activation", "alice", body)
	if response.Code != 200 {
		t.Fatalf("activate: %s", response.Body.String())
	}
	response = request("PUT", "/api/plugins/applications/resource-helper/activation", "alice", body)
	if response.Code != 409 {
		t.Fatalf("stale activation: %d", response.Code)
	}
	response = request("GET", "/api/skills/added", "alice", nil)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "check-source") {
		t.Fatalf("skills %d %s", response.Code, response.Body.String())
	}
	if err = db.Create(&model.Resource{ID: "video-api", UserID: "alice", Kind: "video", Status: model.ResourceStatusReady, MimeType: "video/mp4", Width: 720}).Error; err != nil {
		t.Fatal(err)
	}
	response = request("GET", "/api/plugin-operations?q=inspect&hostSurface=agent-home", "alice", nil)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "inspect-video") {
		t.Fatalf("operation search %d %s", response.Code, response.Body.String())
	}
	response = request("GET", "/api/plugin-operations/resource-helper/inspect-video?releaseId="+release.ID+"&hostSurface=agent-home", "alice", nil)
	if response.Code != 200 || !strings.Contains(response.Body.String(), "contractHash") {
		t.Fatalf("operation describe %d %s", response.Code, response.Body.String())
	}
	invoke, _ := json.Marshal(map[string]any{"operation": "resource-helper.inspect-video", "releaseId": release.ID, "input": map[string]any{"resourceId": "video-api"}})
	response = request("POST", "/api/plugin-invocations", "alice", invoke)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"kind":"inline"`) {
		t.Fatalf("operation invoke %d %s", response.Code, response.Body.String())
	}
	response = request("POST", "/api/plugin-invocations", "bob", invoke)
	if response.Code != 403 {
		t.Fatalf("foreign invocation %d %s", response.Code, response.Body.String())
	}
	var binding model.PluginSkillBinding
	db.First(&binding, "release_id=?", release.ID)
	response = request("GET", "/api/skills/"+binding.SkillID+"/file?path=SKILL.md", "bob", nil)
	if response.Code != 403 {
		t.Fatalf("cross-account file %d %s", response.Code, response.Body.String())
	}
	response = request("PUT", "/api/admin/plugins/applications/resource-helper", "alice", []byte(`{"action":"uninstall","revision":1}`))
	if response.Code != 403 {
		t.Fatal("non-admin management accepted")
	}
	response = request("PUT", "/api/admin/plugins/applications/resource-helper", "admin", []byte(`{"action":"availability","available":false,"revision":1}`))
	if response.Code != 200 {
		t.Fatalf("disable %s", response.Body.String())
	}
	response = request("GET", "/api/skills/added", "alice", nil)
	if response.Code != 200 || strings.Contains(response.Body.String(), "check-source") {
		t.Fatalf("revoked discovery %s", response.Body.String())
	}
	response = request("POST", "/api/plugin-invocations", "alice", []byte(`{}`))
	if response.Code != 400 {
		t.Fatal("malformed plugin invocation was not rejected")
	}
}
