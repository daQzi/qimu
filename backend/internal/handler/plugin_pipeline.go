package handler

import (
	"encoding/json"
	"fmt"
	"github.com/gin-gonic/gin"
	"infinite-canvas/backend/internal/service"
	"net/http"
	"strconv"
	"time"
)

func registerPluginPipelineRoutes(api *gin.RouterGroup, svc *service.Service) {
	api.GET("/plugin-runs", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		offset := 0
		if value := c.Query("offset"); value != "" {
			offset, err = strconv.Atoi(value)
			if err != nil {
				fail(c, 400, fmt.Errorf("无效游标"))
				return
			}
		}
		rows, err := svc.ListPluginRuns(user.ID, offset)
		if err != nil {
			failService(c, err)
			return
		}
		c.Header("Cache-Control", "private, no-store")
		ok(c, rows)
	})
	api.PUT("/plugin-runs/:id/inputs/:inputId", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 68<<10)
		var req service.PluginInputUpdate
		if err = bindApplicationJSON(c, &req, []string{"revision", "mode", "value"}); err != nil {
			fail(c, 400, err)
			return
		}
		result, err := svc.UpdatePluginInput(user.ID, c.Param("id"), c.Param("inputId"), c.GetHeader("Idempotency-Key"), req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, result)
	})
	api.GET("/plugin-runs/:id/events", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		after := int64(0)
		cursor := c.GetHeader("Last-Event-ID")
		if cursor == "" {
			cursor = c.Query("after")
		}
		if cursor != "" {
			after, err = strconv.ParseInt(cursor, 10, 64)
			if err != nil || after < 0 {
				fail(c, 400, fmt.Errorf("无效事件游标"))
				return
			}
		}
		rows, err := svc.PluginRunEvents(user.ID, c.Param("id"), after)
		if err != nil {
			failService(c, err)
			return
		}
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "private, no-cache, no-store")
		c.Header("X-Accel-Buffering", "no")
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		deadline := time.NewTimer(25 * time.Second)
		defer deadline.Stop()
		for {
			for _, event := range rows {
				data, _ := json.Marshal(map[string]any{"runId": event.RunID, "sequence": event.Sequence, "type": event.Type, "data": json.RawMessage(event.PayloadJSON)})
				if _, err = fmt.Fprintf(c.Writer, "id: %d\nevent: plugin-run\ndata: %s\n\n", event.Sequence, data); err != nil {
					return
				}
				after = event.Sequence
			}
			if _, err = fmt.Fprint(c.Writer, ": keepalive\n\n"); err != nil {
				return
			}
			c.Writer.Flush()
			select {
			case <-c.Request.Context().Done():
				return
			case <-deadline.C:
				return
			case <-ticker.C:
			}
			rows, err = svc.PluginRunEvents(user.ID, c.Param("id"), after)
			if err != nil {
				return
			}
		}
	})
}
