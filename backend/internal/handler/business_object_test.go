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
	"infinite-canvas/backend/internal/repository"
	"infinite-canvas/backend/internal/service"
)

func TestBusinessObjectHTTP(t *testing.T) {
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

	request("GET", "/business-objects", "", "", 401)
	body := `{"expectedVersion":0,"clientKey":"http-brand-one","brand":{"name":"品牌","audience":"创作者","positioning":"定位","claims":[],"restrictions":[]}}`
	response := request("POST", "/business-objects", "alice", body, 200)
	var envelope struct {
		Data service.BusinessObjectView `json:"data"`
	}
	if err = json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	id := envelope.Data.Reference.ObjectID
	readPath := "/business-objects/" + id + "/versions/1"
	response = request("GET", readPath, "alice", "", 200)
	if response.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatal("private object cached")
	}
	request("GET", readPath, "bob", "", 404)
	request("PATCH", "/business-objects/"+id+"/archive", "bob", `{"expectedVersion":1,"archived":true}`, 404)
	request("POST", "/business-objects", "alice", body, 200)
	for _, bad := range []string{`{}`, `{"clientKey":"a","clientKey":"b"}`, body[:len(body)-1] + `,"userId":"bob"}`, `{"expectedVersion":null,"clientKey":"http-brand-one","brand":null}`} {
		request("POST", "/business-objects", "alice", bad, 400)
	}
	request("GET", "/business-objects?archived=wat", "alice", "", 400)
	request("GET", "/business-objects?offset=-1", "alice", "", 400)
	request("GET", "/business-objects/"+id+"/versions/101", "alice", "", 400)
	request("PATCH", "/business-objects/"+id+"/archive", "alice", `{"expectedVersion":1,"archived":true}`, 200)
	request("GET", readPath, "alice", "", 200)
	request("DELETE", "/business-objects/"+id, "alice", "", 404)
}
