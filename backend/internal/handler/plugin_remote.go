package handler

import (
	"github.com/gin-gonic/gin"
	"infinite-canvas/backend/internal/service"
	"net/http"
)

func registerPluginRemoteRoutes(api *gin.RouterGroup, svc *service.Service) {
	api.GET("/plugin-connections", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		rows, err := svc.ApplicationPluginConnections(user.ID)
		if err != nil {
			failService(c, err)
			return
		}
		c.Header("Cache-Control", "private, no-store")
		ok(c, gin.H{"connections": rows})
	})
	api.PUT("/plugin-connections", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
		var input service.PluginConnectionInput
		if err = bindApplicationJSON(c, &input, []string{"pluginId", "connectorId", "baseUrl", "enabled", "revision"}); err != nil {
			fail(c, 400, err)
			return
		}
		result, err := svc.SaveApplicationPluginConnection(user.ID, input)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, result)
	})
	api.GET("/admin/plugin-operation-prices/:releaseId/:operationId", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		result, err := svc.PluginOperationPriceForAdmin(user, c.Param("releaseId"), c.Param("operationId"))
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, result)
	})
	api.PUT("/admin/plugin-operation-prices", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
		var input service.PluginOperationPriceInput
		if err = bindApplicationJSON(c, &input, []string{"releaseId", "operationId", "feeMicrocredits", "revision"}); err != nil {
			fail(c, 400, err)
			return
		}
		result, err := svc.SavePluginOperationPrice(user, input)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, result)
	})
	api.POST("/plugin-runs/:id/resume", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
		var input struct {
			Revision      int64  `json:"revision"`
			Action        string `json:"action"`
			ProviderJobID string `json:"providerJobId"`
		}
		if err = bindApplicationJSON(c, &input, []string{"revision", "action"}); err != nil {
			fail(c, 400, err)
			return
		}
		result, err := svc.ResumePluginRun(user.ID, c.Param("id"), input.Revision, input.Action, input.ProviderJobID)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, result)
	})
}
