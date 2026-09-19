package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"infinite-canvas/backend/internal/plugins/contracts"
	"infinite-canvas/backend/internal/service"
)

func registerApplicationPluginRoutes(r *gin.RouterGroup, svc *service.Service) {
	routes := r.Group("/plugins/applications")
	routes.Use(requirePluginCenterAccess(svc))
	routes.GET("", func(c *gin.Context) {
		c.Header("Cache-Control", "private, no-store")
		actor, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		items, err := svc.ApplicationPlugins(actor)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"applications": items})
	})
	routes.PUT("/:id/activation", func(c *gin.Context) {
		actor, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
		var req service.ApplicationPluginActivation
		if err = bindApplicationJSON(c, &req, []string{"releaseId", "enabled", "grantedPermissions", "revision"}); err != nil {
			fail(c, 400, err)
			return
		}
		if err = svc.ActivateApplicationPlugin(actor, c.Param("id"), req); err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"updated": true})
	})
	admin := r.Group("/admin/plugins/applications")
	admin.PUT("/:id", func(c *gin.Context) {
		actor, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
		var req service.ApplicationPluginManagement
		if err = bindApplicationJSON(c, &req, []string{"action", "revision"}); err != nil {
			fail(c, 400, err)
			return
		}
		if err = svc.ManageApplicationPlugin(actor, c.Param("id"), req); err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"updated": true})
	})
	admin.POST("/orphans/preview", func(c *gin.Context) {
		actor, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		items, err := svc.PruneApplicationPluginOrphans(actor, true)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"packages": items, "dryRun": true})
	})
	admin.POST("/orphans/prune", func(c *gin.Context) {
		actor, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		items, err := svc.PruneApplicationPluginOrphans(actor, false)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"packages": items, "recoverable": false})
	})
}

func bindApplicationJSON(c *gin.Context, target any, required []string) error {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return fmt.Errorf("请求正文过大或无法读取")
	}
	value, err := contracts.Decode(raw)
	if err != nil {
		return fmt.Errorf("请求必须为有效、无重复字段的 JSON")
	}
	object, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("请求必须为 JSON 对象")
	}
	if _, management := target.(*service.ApplicationPluginManagement); management {
		if object["action"] == "availability" {
			required = append(required, "available")
		}
		if object["action"] == "revoke" {
			required = append(required, "releaseId")
		}
	}
	for _, field := range required {
		if object[field] == nil {
			return fmt.Errorf("缺少必填字段 %s", field)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("请求包含未知字段或字段类型错误")
	}
	return nil
}
