package handler

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
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

func TestP05RoutesBackgroundWorkerAndReplay(t *testing.T) {
	t.Setenv("REDIS_URL", "")
	t.Setenv("CANVAS_ALLOWED_PRIVATE_UPSTREAM_HOSTS", "127.0.0.1")
	gin.SetMode(gin.TestMode)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(`{"message":"route lifecycle"}`)) }))
	defer upstream.Close()
	db, err := database.Open(database.Config{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "p05.db")})
	if err != nil {
		t.Fatal(err)
	}
	sql, _ := db.DB()
	defer sql.Close()
	if err = database.MigrateSchema(db); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"admin", "alice", "bob"} {
		role := model.UserRoleUser
		if id == "admin" {
			role = model.UserRoleAdmin
		}
		if err = db.Create(&model.User{ID: id, Username: id, Role: role, Status: model.UserStatusActive}).Error; err != nil {
			t.Fatal(err)
		}
		if err = db.Create(&model.AuthSession{ID: id, UserID: id, TokenHash: auth.HashToken("p05-session"), ExpiresAt: time.Now().Add(time.Hour)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err = db.Create(&model.Resource{ID: "video", UserID: "alice", Kind: "video", Status: model.ResourceStatusReady, MimeType: "video/mp4"}).Error; err != nil {
		t.Fatal(err)
	}
	svc := service.New(repository.New(db), t.TempDir())
	defer svc.Close()
	router := gin.New()
	RegisterPluginRoutes(router.Group("/api"), svc)
	request := func(method, path, user string, value any, status int) map[string]any {
		t.Helper()
		var raw []byte
		if data, ok := value.([]byte); ok {
			raw = data
		} else if value != nil {
			raw, _ = json.Marshal(value)
		}
		req := httptest.NewRequest(method, "/api"+path, bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "p05-route-stable")
		if strings.HasSuffix(path, "/derive") {
			req.Header.Set("Idempotency-Key", "p06-route-derive")
		}
		if user != "" {
			req.AddCookie(&http.Cookie{Name: service.SessionCookieName, Value: user + ".p05-session"})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != status {
			t.Fatalf("%s %s %d: %s", method, path, w.Code, w.Body.String())
		}
		var envelope struct {
			Data map[string]any `json:"data"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &envelope)
		return envelope.Data
	}
	raw, err := os.ReadFile("../../../plugin-packages/application-examples/pipeline-helper-1.0.0.yingce-plugin")
	if err != nil {
		t.Fatal(err)
	}
	request("POST", "/plugins", "admin", raw, 200)
	release, _ := repository.New(db).PluginReleaseByVersion("pipeline-helper", "1.0.0")
	request("PUT", "/plugins/applications/pipeline-helper/activation", "alice", map[string]any{"releaseId": release.ID, "enabled": true, "grantedPermissions": []string{"media.read", "connection.use"}, "revision": 0}, 200)
	request("PUT", "/plugin-connections", "alice", map[string]any{"pluginId": "pipeline-helper", "connectorId": "api", "name": "test", "baseUrl": upstream.URL, "credential": "test-secret", "enabled": true, "revision": 0}, 200)
	created := request("POST", "/plugin-invocations", "alice", map[string]any{"operation": "pipeline-helper.process", "releaseId": release.ID, "input": map[string]any{"resourceId": "video"}}, 200)
	id := created["runId"].(string)
	svc.StartWorker()
	wait := func(status string) map[string]any {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for time.Now().Before(deadline) {
			v := request("GET", "/plugin-runs/"+id, "alice", nil, 200)
			if v["status"] == status {
				return v
			}
			time.Sleep(100 * time.Millisecond)
		}
		t.Fatal("worker did not reach", status)
		return nil
	}
	pending := wait("waiting_input")
	in := pending["pipeline"].(map[string]any)["inputs"].([]any)[0].(map[string]any)
	inputPath := "/plugin-runs/" + id + "/inputs/" + in["id"].(string)
	request("PUT", inputPath, "bob", map[string]any{"revision": 1, "mode": "submit", "value": map[string]any{"prompt": "foreign"}}, 404)
	request("PUT", inputPath, "alice", map[string]any{"revision": 1, "mode": "submit", "value": map[string]any{}}, 400)
	request("PUT", inputPath, "alice", map[string]any{"revision": 1, "mode": "draft", "value": map[string]any{"prompt": "draft"}}, 200)
	request("PUT", inputPath, "alice", map[string]any{"revision": 1, "mode": "submit", "value": map[string]any{"prompt": "stale"}}, 409)
	request("PUT", inputPath, "alice", map[string]any{"revision": 2, "mode": "submit", "value": map[string]any{"prompt": "confirmed"}}, 200)
	approved := wait("waiting_approval")
	quote := request("GET", "/plugin-runs/"+id+"/batch-quote", "alice", nil, 200)
	request("GET", "/plugin-runs/"+id+"/batch-quote", "bob", nil, 404)
	request("POST", "/plugin-runs/"+id+"/batch-approval", "alice", map[string]any{"digest": quote["digest"], "count": 1, "amountMicrocredits": quote["amountMicrocredits"], "expiresAt": quote["expiresAt"], "acceptExternalBilling": false}, 400)
	request("POST", "/plugin-runs/"+id+"/batch-approval", "alice", map[string]any{"digest": quote["digest"], "count": 1, "amountMicrocredits": quote["amountMicrocredits"], "expiresAt": quote["expiresAt"], "acceptExternalBilling": true}, 200)
	childID := approved["pipeline"].(map[string]any)["childRunId"].(string)
	child := request("GET", "/plugin-runs/"+childID, "alice", nil, 200)
	request("POST", fmt.Sprintf("/plugin-runs/%s/approvals/%s", childID, child["approvalId"]), "alice", map[string]any{"decision": "approve", "revision": child["revision"]}, 200)
	done := wait("succeeded")
	if done["result"].(map[string]any)["message"] != "route lifecycle" {
		t.Fatal(done)
	}
	request("GET", "/plugin-batch-diagnostics", "alice", nil, 200)
	request("POST", "/plugin-runs/"+id+"/derive", "bob", map[string]any{}, 404)
	derived := request("POST", "/plugin-runs/"+id+"/derive", "alice", map[string]any{"inputs": map[string]any{}, "forceSteps": []string{}, "reuseCompleted": false}, 200)
	if derived["derivedFromRunId"] != id {
		t.Fatal("missing derivation relationship", derived)
	}
	request("GET", "/plugin-runs/"+id+"/events?after=0", "bob", nil, 404)
	request("GET", "/plugin-runs/"+id+"/events?after=bad", "alice", nil, 400)
	server := httptest.NewServer(router)
	defer server.Close()
	req, _ := http.NewRequest("GET", server.URL+"/api/plugin-runs/"+id+"/events", nil)
	req.Header.Set("Last-Event-ID", "1")
	req.AddCookie(&http.Cookie{Name: service.SessionCookieName, Value: "alice.p05-session"})
	response, err := server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(response.Body)
	line, err := reader.ReadString('\n')
	response.Body.Close()
	if err != nil || response.StatusCode != 200 || strings.TrimSpace(line) != "id: 2" {
		t.Fatalf("replay=%q status=%d err=%v", line, response.StatusCode, err)
	}
}
