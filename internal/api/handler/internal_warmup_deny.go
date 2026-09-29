package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// InternalGetWarmupDeny reads the durable mailbox flag for a worker about to
// execute a warmup command. The route is protected by InternalAuthMiddleware.
// Missing rows and database errors are not permissions to send.
func (h *Handler) InternalGetWarmupDeny(c *gin.Context) {
	id, err := uuid.Parse(c.Param("emailID"))
	if err != nil || id == uuid.Nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid email ID"})
		return
	}
	if h.WarmupDenyEmails == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "warmup status unavailable"})
		return
	}
	mailbox, lookupErr := h.WarmupDenyEmails.GetByID(c.Request.Context(), id)
	if lookupErr != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "warmup status unavailable"})
		return
	}
	if mailbox == nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"email_id": mailbox.ID,

		"warmup_denied": mailbox.WarmupDenied,
	})
}
