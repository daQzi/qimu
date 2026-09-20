package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"infinite-canvas/backend/internal/auth"
	"infinite-canvas/backend/internal/database"
	"infinite-canvas/backend/internal/model"
	"infinite-canvas/backend/internal/plugins/authoring"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/repository"
	"infinite-canvas/backend/internal/service"
)

func TestWorkbenchHTTP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.Open(database.Config{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "workbench.db")})
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
		if err = db.Create(&model.AuthSession{ID: id, UserID: id, TokenHash: auth.HashToken("workbench-test"), ExpiresAt: time.Now().Add(time.Hour)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	svc := service.New(repository.New(db), t.TempDir())
	t.Cleanup(func() { svc.Close() })
	files, err := authoring.ReadDirectory("../../../examples/plugins/brand-workbench")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := authoring.Pack(files, contracts.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.InstallManagedPluginForAdmin(&model.User{ID: "admin", Role: model.UserRoleAdmin}, raw, "brand.yingce-plugin"); err != nil {
		t.Fatal(err)
	}
	release, err := repository.New(db).PluginReleaseByVersion("brand-workbench", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if err = svc.ActivateApplicationPlugin(&model.User{ID: "alice", Role: model.UserRoleUser}, "brand-workbench", service.ApplicationPluginActivation{ReleaseID: release.ID, Enabled: true, GrantedPermissions: []string{}}); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	RegisterPluginRoutes(router.Group("/api"), svc)
	request := func(method, path, user, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "/api"+path, bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		if user != "" {
			req.AddCookie(&http.Cookie{Name: service.SessionCookieName, Value: user + ".workbench-test"})
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		if res.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, res.Code, res.Body.String())
		}
		return res
	}
	request("GET", "/plugin-workbenches", "", "", 401)
	request("GET", "/plugin-workbenches", "alice", "", 200)
	response := request("GET", "/plugin-workbenches/brand-workbench.compose?hostSurface=agent-home", "alice", "", 200)
	if response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("private data cached")
	}
	request("GET", "/plugin-workbenches/brand-workbench.compose?hostSurface=agent-home", "bob", "", 403)
	body := `{"id":"brand-workbench.compose","releaseId":"` + release.ID + `","recipeIds":["social"],"input":{"brand":"真实品牌","audience":"作者"},"context":{"hostSurface":"agent-home"}}`
	response = request("POST", "/plugin-workbench-previews", "alice", body, 200)
	var envelope struct {
		Data service.PluginWorkbenchPreview `json:"data"`
	}
	if err = json.Unmarshal(response.Body.Bytes(), &envelope); err != nil || !envelope.Data.Valid {
		t.Fatal(response.Body.String(), err)
	}
	for _, bad := range []string{`{}`, `{"id":"a.b","id":"c.d"}`, body[:len(body)-1] + `,"userId":"bob"}`, `{"id":"brand-workbench.compose","releaseId":"` + release.ID + `","recipeIds":null,"input":{},"context":{"hostSurface":"agent-home"}}`} {
		request("POST", "/plugin-workbench-previews", "alice", bad, 400)
	}
}
