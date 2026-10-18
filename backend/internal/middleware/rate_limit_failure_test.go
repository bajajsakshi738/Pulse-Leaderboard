package middleware

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// no Redis needed: the client points at a port nothing listens on, which is
// what a Redis outage looks like to the limiter.
func brokenRedis() *redis.Client {
	return redis.NewClient(&redis.Options{
		Addr:        "127.0.0.1:1",
		DialTimeout: 100 * time.Millisecond,
		MaxRetries:  -1,
	})
}

func TestFailOpenLetsRequestsThroughWhenRedisIsDown(t *testing.T) {
	rl := NewRateLimiter(brokenRedis(), "", false)
	r := newRouter(rl, 1, time.Minute, rl.ByIP)

	if w := do(r, "1.1.1.1", nil); w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (fail open)", w.Code)
	}
}

func TestFailClosedRefusesRequestsWhenRedisIsDown(t *testing.T) {
	rl := NewRateLimiter(brokenRedis(), "", false)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.GET("/x", rl.LimitFailClosed("t", 1, time.Minute, rl.ByIP), func(c *gin.Context) { c.String(200, "ok") })

	if w := do(r, "1.1.1.1", nil); w.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d, want 503 (fail closed)", w.Code)
	}
}

func TestBypassStillWorksWhenFailingClosed(t *testing.T) {
	rl := NewRateLimiter(brokenRedis(), "secret-token", false)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.GET("/x", rl.LimitFailClosed("t", 1, time.Minute, rl.ByIP), func(c *gin.Context) { c.String(200, "ok") })

	w := do(r, "1.1.1.1", map[string]string{BypassHeader: "secret-token"})
	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (the bot generator must keep working)", w.Code)
	}
}
