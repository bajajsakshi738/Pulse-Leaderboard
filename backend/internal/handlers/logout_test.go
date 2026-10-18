package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/bajajsakshi738/leaderboard-backend/internal/auth"
	"github.com/bajajsakshi738/leaderboard-backend/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// no services needed. the Redis client points at a port nothing listens on.

func logoutRouter(tokenID string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	dead := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: -1})
	h := NewAuthHandler(nil, nil, nil, auth.NewDenylist(dead))

	// stands in for RequireAuth.
	asToken := func(c *gin.Context) {
		c.Set(middleware.ContextTokenIDKey, tokenID)
		c.Set(middleware.ContextTokenExpiryKey, time.Now().Add(time.Hour))
	}
	r := gin.New()
	r.POST("/logout", asToken, h.Logout)
	return r
}

func TestLogoutTellsTheClientWhenRedisIsDown(t *testing.T) {
	w := send(logoutRouter("some-token-id"), http.MethodPost, "/logout", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503, so the client doesn't think the token is dead", w.Code)
	}
}

func TestLogoutOfOldTokenWithoutIDDoesNothing(t *testing.T) {
	// an old token has no ID. nothing to block, and Redis is not touched.
	w := send(logoutRouter(""), http.MethodPost, "/logout", "")
	if w.Code != http.StatusNoContent {
		t.Fatalf("got %d, want 204", w.Code)
	}
}
