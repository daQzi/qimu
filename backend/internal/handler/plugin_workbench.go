package handler

import (
	"github.com/gin-gonic/gin"
	"infinite-canvas/backend/internal/service"
	"net/http"
)

func registerPluginWorkbenchRoutes(api *gin.RouterGroup, svc *service.Service) {
	api.GET("/plugin-workbenches", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		result, err := svc.PluginWorkbenches(user.ID)
		if err != nil {
			failService(c, err)
			return
		}
		c.Header("Cache-Control", "private, no-store")
		ok(c, result)
	})
	api.GET("/plugin-workbenches/:id", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		ctx := service.PluginOperationContext{HostSurface: c.Query("hostSurface"), CanvasID: c.Query("canvasId")}
		result, err := svc.PluginWorkbench(user.ID, c.Param("id"), c.Query("releaseId"), ctx)
		if err != nil {
			failService(c, err)
			return
		}
		c.Header("Cache-Control", "private, no-store")
		ok(c, result)
	})
	api.POST("/plugin-workbench-previews", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
		var req service.PluginWorkbenchComposeRequest
		if err = bindApplicationJSON(c, &req, []string{"id", "releaseId", "recipeIds", "input", "context"}); err != nil {
			fail(c, 400, err)
			return
		}
		result, err := svc.ComposePluginWorkbench(user.ID, req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, result)
	})
}
