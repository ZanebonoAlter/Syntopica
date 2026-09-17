package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"syntopica-backend/internal/platform/notification"
)

// Notification HTTP handlers (add-notification-center). Single-user system:
// no auth scoping. Response envelope follows the project convention
// {"success": bool, "data"|"error": ...}.

// ListNotifications handles GET /api/notifications?limit=&offset=&unread=
func ListNotifications(c *gin.Context) {
	unreadOnly := false
	if v := c.Query("unread"); v == "true" || v == "1" {
		unreadOnly = true
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	rows, total, err := notification.List(unreadOnly, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"notifications": rows,
			"total":         total,
		},
	})
}

// GetUnreadCount handles GET /api/notifications/unread-count
func GetUnreadCount(c *gin.Context) {
	count, err := notification.UnreadCount()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"unread": count,
		},
	})
}

// MarkNotificationRead handles POST /api/notifications/:id/read
func MarkNotificationRead(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "invalid notification id"})
		return
	}
	notif, err := notification.MarkRead(uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	if notif == nil {
		// 越界引用：不存在的 id 返回 404，不误标他行（test-cases V6）
		c.JSON(http.StatusNotFound, gin.H{"success": false, "error": "notification not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "data": notif})
}

// MarkAllNotificationsRead handles POST /api/notifications/read-all
func MarkAllNotificationsRead(c *gin.Context) {
	affected, err := notification.MarkAllRead()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"affected": affected,
		},
	})
}

// ClearNotifications handles DELETE /api/notifications
func ClearNotifications(c *gin.Context) {
	affected, err := notification.ClearAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"affected": affected,
		},
	})
}
