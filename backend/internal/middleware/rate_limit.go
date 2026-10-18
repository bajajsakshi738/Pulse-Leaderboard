package middleware

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// header the bot generator sends to skip rate limiting. it only works if
// RATE_LIMIT_BYPASS_TOKEN is set on the server and matches.
const BypassHeader = "X-RateLimit-Bypass"

// counts a request and sets an expiry in one step, so a key can never be
// left without one. returns {count, ttl in ms}.
var incrScript = redis.NewScript(`
local c = redis.call('INCR', KEYS[1])
local ttl = redis.call('PTTL', KEYS[1])
if c == 1 or ttl < 0 then
  redis.call('PEXPIRE', KEYS[1], ARGV[1])
  ttl = tonumber(ARGV[1])
end
return {c, ttl}
`)

// RateLimiter limits requests per client. the counters live in Redis, so
// every server instance shares them.
type RateLimiter struct {
	rdb         *redis.Client
	bypassToken string
	trustProxy  bool
}

// NewRateLimiter makes a limiter.
// bypassToken: requests with this token in BypassHeader are never limited
// (the bot generator uses it). empty means no bypass.
// trustProxy: only true behind a proxy like Render that adds the real IP to
// X-Forwarded-For. when false we use the TCP address, so it can't be faked.
func NewRateLimiter(rdb *redis.Client, bypassToken string, trustProxy bool) *RateLimiter {
	return &RateLimiter{rdb: rdb, bypassToken: bypassToken, trustProxy: trustProxy}
}

// KeyFunc picks what a limit counts against (IP, user). "" means no limit.
type KeyFunc func(c *gin.Context) string

// ByIP counts per client IP.
func (rl *RateLimiter) ByIP(c *gin.Context) string {
	return rl.clientIP(c)
}

// ByUser counts per logged in user. RequireAuth has to run first.
func (rl *RateLimiter) ByUser(c *gin.Context) string {
	if id, ok := c.Get(ContextUserIDKey); ok {
		if s, ok := id.(string); ok {
			return s
		}
	}
	return ""
}

// Limit allows `limit` requests per `window` for each key. scope names the
// counter so endpoints don't share a budget.
// fixed window: simple, but allows a small burst at the window edge.
// if Redis is down the request goes through (fail open). used for scores.
func (rl *RateLimiter) Limit(scope string, limit int, window time.Duration, keyFn KeyFunc) gin.HandlerFunc {
	return rl.limit(scope, limit, window, keyFn, false)
}

// LimitFailClosed is the same, but returns 503 when Redis is down.
// used for login and register: with no counters there is no protection
// against password guessing, so we refuse instead. nobody can log in
// during a Redis outage.
func (rl *RateLimiter) LimitFailClosed(scope string, limit int, window time.Duration, keyFn KeyFunc) gin.HandlerFunc {
	return rl.limit(scope, limit, window, keyFn, true)
}

func (rl *RateLimiter) limit(scope string, limit int, window time.Duration, keyFn KeyFunc, failClosed bool) gin.HandlerFunc {
	windowMs := window.Milliseconds()

	return func(c *gin.Context) {
		if rl.bypass(c) {
			c.Next()
			return
		}

		id := keyFn(c)
		if id == "" {
			c.Next()
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 500*time.Millisecond)
		defer cancel()

		key := fmt.Sprintf("ratelimit:%s:%s", scope, id)
		res, err := incrScript.Run(ctx, rl.rdb, []string{key}, windowMs).Int64Slice()
		if err != nil || len(res) != 2 {
			if failClosed {
				log.Printf("rate limiter: redis error for scope %q (refusing request): %v", scope, err)
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
					"error": "service temporarily unavailable, please try again shortly",
				})
				return
			}
			log.Printf("rate limiter: redis error for scope %q (allowing request): %v", scope, err)
			c.Next()
			return
		}

		count, ttlMs := res[0], res[1]
		remaining := int64(limit) - count
		if remaining < 0 {
			remaining = 0
		}
		c.Header("X-RateLimit-Limit", strconv.Itoa(limit))
		c.Header("X-RateLimit-Remaining", strconv.FormatInt(remaining, 10))

		if count > int64(limit) {
			retryAfter := int(math.Ceil(float64(ttlMs) / 1000))
			if retryAfter < 1 {
				retryAfter = 1
			}
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": fmt.Sprintf("too many requests, please try again in %d seconds", retryAfter),
			})
			return
		}

		c.Next()
	}
}

// bypass is true if the request carries the right secret token.
func (rl *RateLimiter) bypass(c *gin.Context) bool {
	if rl.bypassToken == "" {
		return false
	}
	got := c.GetHeader(BypassHeader)
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(rl.bypassToken)) == 1
}

// clientIP is the address we rate limit on.
func (rl *RateLimiter) clientIP(c *gin.Context) string {
	if rl.trustProxy {
		// the last X-Forwarded-For entry is the one our proxy added. earlier
		// entries come from the client and can be faked.
		if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if last := strings.TrimSpace(parts[len(parts)-1]); last != "" {
				return last
			}
		}
	}
	// SetTrustedProxies(nil) in main.go means Gin gives us the TCP address.
	return c.ClientIP()
}
