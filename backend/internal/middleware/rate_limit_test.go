package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// these need a real Redis. set TEST_REDIS_ADDR (e.g. localhost:6379), they
// are skipped without it.
func testRedis(t *testing.T) *redis.Client {
	t.Helper()
	addr := os.Getenv("TEST_REDIS_ADDR")
	if addr == "" {
		t.Skip("TEST_REDIS_ADDR not set")
	}
	rdb := redis.NewClient(&redis.Options{Addr: addr, DB: 15})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("redis not reachable: %v", err)
	}
	rdb.FlushDB(context.Background())
	t.Cleanup(func() { rdb.FlushDB(context.Background()); rdb.Close() })
	return rdb
}

func newRouter(rl *RateLimiter, limit int, window time.Duration, key KeyFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.GET("/x", rl.Limit("t", limit, window, key), func(c *gin.Context) { c.String(200, "ok") })
	return r
}

func do(r *gin.Engine, remote string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = remote + ":5555"
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestLimitBlocksAfterLimitAndRecovers(t *testing.T) {
	rl := NewRateLimiter(testRedis(t), "", false)
	r := newRouter(rl, 3, 800*time.Millisecond, rl.ByIP)

	for i := 1; i <= 3; i++ {
		if w := do(r, "1.1.1.1", nil); w.Code != 200 {
			t.Fatalf("request %d: got %d, want 200", i, w.Code)
		}
	}
	w := do(r, "1.1.1.1", nil)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("4th request: got %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("429 response is missing Retry-After")
	}

	// a different IP has its own budget.
	if w := do(r, "2.2.2.2", nil); w.Code != 200 {
		t.Fatalf("other IP: got %d, want 200", w.Code)
	}

	time.Sleep(900 * time.Millisecond)
	if w := do(r, "1.1.1.1", nil); w.Code != 200 {
		t.Fatalf("after window: got %d, want 200", w.Code)
	}
}

func TestSpoofedForwardedForIgnoredWhenProxyNotTrusted(t *testing.T) {
	rl := NewRateLimiter(testRedis(t), "", false)
	r := newRouter(rl, 1, time.Minute, rl.ByIP)

	do(r, "3.3.3.3", map[string]string{"X-Forwarded-For": "9.9.9.1"})
	w := do(r, "3.3.3.3", map[string]string{"X-Forwarded-For": "9.9.9.2"})
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("faking X-Forwarded-For must not dodge the limit: got %d", w.Code)
	}
}

func TestTrustedProxyUsesLastForwardedEntry(t *testing.T) {
	rl := NewRateLimiter(testRedis(t), "", true)
	r := newRouter(rl, 1, time.Minute, rl.ByIP)

	// the client forges the first entry, our proxy added the real one last.
	do(r, "10.0.0.1", map[string]string{"X-Forwarded-For": "7.7.7.1, 5.5.5.5"})
	w := do(r, "10.0.0.1", map[string]string{"X-Forwarded-For": "7.7.7.2, 5.5.5.5"})
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("forged leading entry must not dodge the limit: got %d", w.Code)
	}
	// a different client behind the same proxy is separate.
	if w := do(r, "10.0.0.1", map[string]string{"X-Forwarded-For": "6.6.6.6"}); w.Code != 200 {
		t.Fatalf("different real client: got %d, want 200", w.Code)
	}
}

func TestBypassToken(t *testing.T) {
	rl := NewRateLimiter(testRedis(t), "s3cret", false)
	r := newRouter(rl, 1, time.Minute, rl.ByIP)

	for i := 0; i < 5; i++ {
		if w := do(r, "4.4.4.4", map[string]string{BypassHeader: "s3cret"}); w.Code != 200 {
			t.Fatalf("with correct token request %d got %d", i, w.Code)
		}
	}
	// a wrong token is limited like anyone else.
	do(r, "4.4.4.4", map[string]string{BypassHeader: "wrong"})
	if w := do(r, "4.4.4.4", map[string]string{BypassHeader: "wrong"}); w.Code != http.StatusTooManyRequests {
		t.Fatalf("wrong token must not bypass: got %d", w.Code)
	}
}

func TestBypassDisabledWhenNoTokenConfigured(t *testing.T) {
	rl := NewRateLimiter(testRedis(t), "", false)
	r := newRouter(rl, 1, time.Minute, rl.ByIP)

	// an empty header must never match an empty configured token.
	do(r, "8.8.4.4", map[string]string{BypassHeader: ""})
	if w := do(r, "8.8.4.4", map[string]string{BypassHeader: ""}); w.Code != http.StatusTooManyRequests {
		t.Fatalf("empty token must not bypass: got %d", w.Code)
	}
}

func TestFailsOpenWhenRedisDown(t *testing.T) {
	gin.SetMode(gin.TestMode)
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 100 * time.Millisecond, MaxRetries: -1})
	rl := NewRateLimiter(rdb, "", false)
	r := newRouter(rl, 1, time.Minute, rl.ByIP)

	for i := 0; i < 3; i++ {
		if w := do(r, "5.5.5.5", nil); w.Code != 200 {
			t.Fatalf("redis down should allow requests, got %d", w.Code)
		}
	}
}
