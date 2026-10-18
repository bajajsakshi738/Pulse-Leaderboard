package middleware

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/bajajsakshi738/leaderboard-backend/internal/auth"
	"github.com/gin-gonic/gin"
)

const (
	ContextUserIDKey      = "userID"
	ContextUsernameKey    = "username"
	ContextTokenIDKey     = "tokenID"
	ContextTokenExpiryKey = "tokenExpiry"
)

// RequireAuth checks the Authorization: Bearer <token> header and puts the
// user's ID and username in the Gin context. if revoked is not nil, tokens
// that were logged out are refused too.
//
// if Redis can't be reached, the logout check is skipped (fails open) so
// score submits keep working, which is how the rest of the app behaves when
// Redis is down. in that case a logged out token works until it expires.
func RequireAuth(tm *auth.TokenManager, revoked auth.RevocationChecker) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			return
		}

		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header format"})
			return
		}

		claims, err := tm.ParseToken(parts[1])
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			return
		}

		// tokens made before the logout feature have no ID and can't be blocked.
		if revoked != nil && claims.ID != "" {
			isRevoked, err := revoked.IsRevoked(c.Request.Context(), claims.ID)
			if err != nil {
				log.Printf("could not check if token was logged out, letting it through: %v", err)
			} else if isRevoked {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
				return
			}
		}

		c.Set(ContextUserIDKey, claims.UserID)
		c.Set(ContextUsernameKey, claims.Username)
		c.Set(ContextTokenIDKey, claims.ID)
		if claims.ExpiresAt != nil {
			c.Set(ContextTokenExpiryKey, claims.ExpiresAt.Time)
		}
		c.Next()
	}
}

// TokenExpiry reads the expiry stored by RequireAuth.
func TokenExpiry(c *gin.Context) (time.Time, bool) {
	v, ok := c.Get(ContextTokenExpiryKey)
	if !ok {
		return time.Time{}, false
	}
	t, ok := v.(time.Time)
	return t, ok
}
