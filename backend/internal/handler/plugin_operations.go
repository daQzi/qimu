package handler

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"infinite-canvas/backend/internal/service"
	"net/http"
	"strconv"
)

func registerPluginOperationRoutes(api *gin.RouterGroup, svc *service.Service) {
	api.GET("/plugin-canvases/:id/snapshot", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		result, err := svc.PluginCanvasSnapshot(user.ID, c.Param("id"))
		if err != nil {
			failService(c, err)
			return
		}
		c.Header("Cache-Control", "private, no-store")
		ok(c, result)
	})
	api.GET("/plugin-operations", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		offset := 0
		if value := c.Query("cursor"); value != "" {
			offset, err = strconv.Atoi(value)
			if err != nil {
				fail(c, 400, fmt.Errorf("无效游标"))
				return
			}
		}
		ctx := service.PluginOperationContext{HostSurface: c.Query("hostSurface"), CanvasID: c.Query("canvasId"), ProjectID: c.Query("projectId")}
		items, next, err := svc.SearchPluginOperations(user.ID, c.Query("q"), offset, ctx)
		if err != nil {
			failService(c, err)
			return
		}
		c.Header("Cache-Control", "private, no-store")
		ok(c, gin.H{"operations": items, "nextCursor": next})
	})
	api.GET("/plugin-operations/:pluginId/:operationId", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		ctx := service.PluginOperationContext{HostSurface: c.Query("hostSurface"), CanvasID: c.Query("canvasId"), ProjectID: c.Query("projectId")}
		item, err := svc.DescribePluginOperation(user.ID, c.Param("pluginId")+"."+c.Param("operationId"), c.Query("releaseId"), ctx)
		if err != nil {
			failService(c, err)
			return
		}
		c.Header("Cache-Control", "private, no-store")
		ok(c, item)
	})
	api.POST("/plugin-invocations", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
		var req service.PluginOperationInvocation
		if err = bindApplicationJSON(c, &req, []string{"operation", "releaseId", "input"}); err != nil {
			fail(c, 400, err)
			return
		}
		output, err := svc.InvokePluginOperation(user.ID, c.GetHeader("Idempotency-Key"), req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, output)
	})
	api.GET("/plugin-runs/:id", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		run, err := svc.PluginRun(user.ID, c.Param("id"), c.Query("viewId"))
		if err != nil {
			failService(c, err)
			return
		}
		c.Header("Cache-Control", "private, no-store")
		ok(c, run)
	})
	api.POST("/plugin-runs/:id/approvals/:approvalId", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
		var req struct {
			Decision string `json:"decision"`
			Revision int64  `json:"revision"`
		}
		if err = bindApplicationJSON(c, &req, []string{"decision", "revision"}); err != nil {
			fail(c, 400, err)
			return
		}
		run, err := svc.DecidePluginRun(user.ID, c.Param("id"), c.Param("approvalId"), req.Decision, req.Revision)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, run)
	})
	api.POST("/plugin-runs/:id/cancel", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
		var req struct {
			Revision int64 `json:"revision"`
		}
		if err = bindApplicationJSON(c, &req, []string{"revision"}); err != nil {
			fail(c, 400, err)
			return
		}
		run, err := svc.CancelPluginRun(user.ID, c.Param("id"), req.Revision)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, run)
	})
}
