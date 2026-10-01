package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/warmbly/warmbly/internal/app/sendauth"
)

// InternalSendAuthorization checks the durable task/account binding immediately
// before a provider attempt. Internal bearer authentication is enforced by the route.
func (h *Handler) InternalSendAuthorization(c *gin.Context) {
	var req sendauth.Request
	if err := c.ShouldBindJSON(&req); err != nil || h.SendAuthorization == nil {
		c.JSON(http.StatusForbidden, gin.H{"allowed": false})
		return
	}
	c.JSON(http.StatusOK, gin.H{"allowed": h.SendAuthorization.Allowed(c.Request.Context(), req)})
}
