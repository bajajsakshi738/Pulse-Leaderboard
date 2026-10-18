package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// BodyLimit caps the size of a request body. reading past the cap makes the
// JSON decoder fail with *http.MaxBytesError, and the handlers answer 413.
// every body this API takes is tiny, so real clients never hit it.
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}
