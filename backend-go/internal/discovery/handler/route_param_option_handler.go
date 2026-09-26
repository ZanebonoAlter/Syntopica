package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/discovery/service"
	"syntopica-backend/internal/platform/database"
)

// ── 路由参数可选值字典 CRUD（feed-param-options）──
// admin 维护 RSSHub 路由参数可选值（人工 manual / 文档抓取 scraped）。
// source 永不接受 llm（design D5 铁律，service 层校验）。

func newRouteParamOptionService() *service.RouteParamOptionService {
	return service.NewRouteParamOptionService(database.DB)
}

// ListRouteParamOptions GET /api/admin/route-param-options[?route_id=N] — 字典列表，可按 route_id 过滤。
func ListRouteParamOptions(c *gin.Context) {
	svc := newRouteParamOptionService()
	var routeID *uint
	if raw := c.Query("route_id"); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid route_id"})
			return
		}
		uid := uint(id)
		routeID = &uid
	}
	opts, err := svc.List(c.Request.Context(), routeID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": opts})
}

// CreateRouteParamOption POST /api/admin/route-param-options — 新建字典条目。
func CreateRouteParamOption(c *gin.Context) {
	var in service.RouteParamOptionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid request body"})
		return
	}
	svc := newRouteParamOptionService()
	opt, err := svc.Create(c.Request.Context(), in)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": opt})
}

// UpdateRouteParamOption PUT /api/admin/route-param-options/:id — 更新字典条目。
func UpdateRouteParamOption(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}
	var in service.RouteParamOptionInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid request body"})
		return
	}
	svc := newRouteParamOptionService()
	opt, err := svc.Update(c.Request.Context(), uint(id), in)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": opt})
}

// DeleteRouteParamOption DELETE /api/admin/route-param-options/:id — 删除字典条目。
func DeleteRouteParamOption(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid id"})
		return
	}
	svc := newRouteParamOptionService()
	if err := svc.Delete(c.Request.Context(), uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "deleted"})
}
