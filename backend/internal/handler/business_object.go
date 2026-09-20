package handler

import (
	"errors"
	"github.com/gin-gonic/gin"
	"infinite-canvas/backend/internal/service"
	"net/http"
	"strconv"
)

func registerBusinessObjectRoutes(api *gin.RouterGroup, svc *service.Service) {
	api.GET("/business-objects", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		offset := 0
		if c.Query("offset") != "" {
			offset, err = strconv.Atoi(c.Query("offset"))
			if err != nil {
				fail(c, 400, err)
				return
			}
		}
		if v := c.Query("archived"); v != "" && v != "true" && v != "false" {
			fail(c, 400, errors.New("archived 必须为 true 或 false"))
			return
		}
		rows, err := svc.ListBusinessObjects(user.ID, c.Query("q"), c.Query("archived") == "true", offset)
		if err != nil {
			failService(c, err)
			return
		}
		c.Header("Cache-Control", "private, no-store")
		ok(c, rows)
	})
	api.GET("/business-objects/:id/versions/:version", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		version, err := strconv.Atoi(c.Param("version"))
		if err != nil {
			fail(c, 400, err)
			return
		}
		view, err := svc.ReadBusinessObject(user.ID, c.Param("id"), version)
		if err != nil {
			failService(c, err)
			return
		}
		c.Header("Cache-Control", "private, no-store")
		ok(c, view)
	})
	api.POST("/business-objects", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<10)
		var req service.BusinessObjectWrite
		if err = bindApplicationJSON(c, &req, []string{"expectedVersion", "clientKey", "brand"}); err != nil {
			fail(c, 400, err)
			return
		}
		view, err := svc.SaveBusinessObject(user.ID, req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, view)
	})
	api.PATCH("/business-objects/:id/archive", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
		var req struct {
			ExpectedVersion int  `json:"expectedVersion"`
			Archived        bool `json:"archived"`
		}
		if err = bindApplicationJSON(c, &req, []string{"expectedVersion", "archived"}); err != nil {
			fail(c, 400, err)
			return
		}
		if err = svc.ArchiveBusinessObject(user.ID, c.Param("id"), req.ExpectedVersion, req.Archived); err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"archived": req.Archived})
	})
}
