package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"infinite-canvas/backend/internal/service"
)

func decodeAgentThreadBody(c *gin.Context, target any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 128<<10)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		fail(c, http.StatusBadRequest, err)
		return false
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		fail(c, http.StatusBadRequest, errors.New("请求必须只包含一个 JSON 对象"))
		return false
	}
	return true
}
func registerAgentThreadRoutes(r *gin.RouterGroup, svc *service.Service) {
	r.GET("/agent/threads", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
		if err != nil {
			fail(c, 400, err)
			return
		}
		rows, err := svc.ListAgentThreads(user.ID, c.Query("canvasId"), offset)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"threads": rows, "hasMore": len(rows) == 50})
	})
	r.POST("/agent/threads", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var req service.AgentThreadCreate
		if !decodeAgentThreadBody(c, &req) {
			return
		}
		row, err := svc.CreateAgentThread(user.ID, req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"thread": row})
	})
	r.GET("/agent/threads/:id", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		before, err := strconv.ParseInt(c.DefaultQuery("before", "0"), 10, 64)
		if err != nil {
			fail(c, 400, err)
			return
		}
		view, err := svc.GetAgentThread(user.ID, c.Param("id"), before)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, view)
	})
	r.PATCH("/agent/threads/:id/context", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var req service.AgentThreadBinding
		if !decodeAgentThreadBody(c, &req) {
			return
		}
		row, err := svc.BindAgentThread(user.ID, c.Param("id"), req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"thread": row})
	})
	r.POST("/agent/threads/:id/messages", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		policy, available := loadRuntimePolicy(c, svc)
		if !available || !enforceRateLimit(c, "tasks:"+user.ID, policy.Request.TaskCreatePerMinute, time.Minute) {
			return
		}
		var req service.AgentThreadMessage
		if !decodeAgentThreadBody(c, &req) {
			return
		}
		result, err := svc.AppendAgentThreadMessage(user.ID, c.Param("id"), req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, result)
	})
	r.POST("/agent/threads/:id/references", func(c *gin.Context) {
		user, err := currentUser(c, svc)
		if err != nil {
			failService(c, err)
			return
		}
		var req service.AgentThreadReference
		if !decodeAgentThreadBody(c, &req) {
			return
		}
		row, err := svc.AppendAgentThreadReference(user.ID, c.Param("id"), req)
		if err != nil {
			failService(c, err)
			return
		}
		ok(c, gin.H{"thread": row})
	})
}
