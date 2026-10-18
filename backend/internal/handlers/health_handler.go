package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// HealthCheck handles GET /health. hosts like Render use it to see if the
// service is up.
func HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
