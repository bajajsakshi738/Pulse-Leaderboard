package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bajajsakshi738/leaderboard-backend/internal/auth"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// fake checker, so these tests don't need Redis.
type fakeChecker struct {
	revoked map[string]bool
	err     error
}

func (f fakeChecker) IsRevoked(_ context.Context, id string) (bool, error) {
	return f.revoked[id], f.err
}

const testSecret = "test-secret-test-secret-test-secret"

func authRouter(tm *auth.TokenManager, checker auth.RevocationChecker) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/private", RequireAuth(tm, checker), func(c *gin.Context) { c.Status(http.StatusOK) })
	return r
}

func callWith(r *gin.Engine, token string) int {
	req, _ := http.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func tokenID(t *testing.T, tm *auth.TokenManager, token string) string {
	t.Helper()
	claims, err := tm.ParseToken(token)
	if err != nil {
		t.Fatal(err)
	}
	return claims.ID
}

func TestEachTokenGetsItsOwnID(t *testing.T) {
	tm := auth.NewTokenManager(testSecret, time.Hour)
	uid := uuid.New()
	a, _ := tm.GenerateToken(uid, "sam")
	b, _ := tm.GenerateToken(uid, "sam")
	idA, idB := tokenID(t, tm, a), tokenID(t, tm, b)
	if idA == "" || idB == "" || idA == idB {
		t.Fatalf("token IDs should be set and different, got %q and %q", idA, idB)
	}
}

func TestLoggedOutTokenIsRefused(t *testing.T) {
	tm := auth.NewTokenManager(testSecret, time.Hour)
	uid := uuid.New()
	out, _ := tm.GenerateToken(uid, "sam")
	other, _ := tm.GenerateToken(uid, "sam")

	r := authRouter(tm, fakeChecker{revoked: map[string]bool{tokenID(t, tm, out): true}})

	if got := callWith(r, out); got != http.StatusUnauthorized {
		t.Errorf("logged out token: got %d, want 401", got)
	}
	// a different token of the same user is not affected.
	if got := callWith(r, other); got != http.StatusOK {
		t.Errorf("other token: got %d, want 200", got)
	}
}

func TestLogoutCheckFailsOpenWhenRedisIsDown(t *testing.T) {
	tm := auth.NewTokenManager(testSecret, time.Hour)
	token, _ := tm.GenerateToken(uuid.New(), "sam")
	r := authRouter(tm, fakeChecker{err: errors.New("redis down")})
	if got := callWith(r, token); got != http.StatusOK {
		t.Errorf("redis down: got %d, want 200 (fail open)", got)
	}
}

func TestNoCheckerMeansNoLogoutCheck(t *testing.T) {
	tm := auth.NewTokenManager(testSecret, time.Hour)
	token, _ := tm.GenerateToken(uuid.New(), "sam")
	r := authRouter(tm, nil)
	if got := callWith(r, token); got != http.StatusOK {
		t.Errorf("got %d, want 200", got)
	}
}

func TestOldTokenWithoutIDStillWorks(t *testing.T) {
	tm := auth.NewTokenManager(testSecret, time.Hour)
	uid := uuid.New()

	// a token like the ones made before logout existed: no ID.
	claims := auth.Claims{
		UserID:   uid.String(),
		Username: "sam",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   uid.String(),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	old, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatal(err)
	}

	// even a checker that says "everything is revoked" can't touch it.
	r := authRouter(tm, fakeChecker{revoked: map[string]bool{"": true}})
	if got := callWith(r, old); got != http.StatusOK {
		t.Errorf("old token: got %d, want 200", got)
	}
}
