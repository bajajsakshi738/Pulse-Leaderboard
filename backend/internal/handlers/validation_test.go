package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bajajsakshi738/leaderboard-backend/internal/middleware"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// no database or Redis needed. every case is rejected before the handler
// touches storage, so the handlers get nil dependencies. if a case ever
// reached storage the test would panic, which would show a missing check.

func authRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewAuthHandler(nil, nil, nil, nil)
	r := gin.New()
	r.Use(middleware.BodyLimit(1024))
	r.POST("/register", h.Register)
	r.POST("/login", h.Login)
	return r
}

func asUser(c *gin.Context) {
	c.Set(middleware.ContextUserIDKey, uuid.NewString())
	c.Set(middleware.ContextUsernameKey, "tester")
}

func scoreRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	h := NewLeaderboardHandler(nil, nil, nil)
	r := gin.New()
	r.Use(middleware.BodyLimit(1024))
	r.POST("/scores", asUser, h.SubmitScore)
	r.GET("/history", asUser, h.GetMyHistory)
	r.GET("/report", h.GetTopPlayersReport)
	r.GET("/leaderboard/:gameCode/top", h.GetTop)
	r.GET("/leaderboard/:gameCode/me", asUser, h.GetMyRank)
	r.GET("/leaderboard/:gameCode/rank/:userID", h.GetRankForUser)
	return r
}

func send(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func errorText(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var out struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not JSON with an error field: %q", w.Body.String())
	}
	return out.Error
}

type wantError struct {
	name   string
	body   string
	status int
	text   string
}

func runCases(t *testing.T, r *gin.Engine, method, path string, cases []wantError) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := send(r, method, path, tc.body)
			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tc.status, w.Body.String())
			}
			got := errorText(t, w)
			if got != tc.text {
				t.Fatalf("error = %q, want %q", got, tc.text)
			}
			// raw validator text must never reach the client.
			for _, leak := range []string{"Key:", "Error:Field", "validation for", "binding", "json:"} {
				if strings.Contains(got, leak) {
					t.Fatalf("error text leaks internals (%q): %q", leak, got)
				}
			}
		})
	}
}

func TestRegisterRejectsBadInput(t *testing.T) {
	long := strings.Repeat("a", 73)
	runCases(t, authRouter(), http.MethodPost, "/register", []wantError{
		{"short username", `{"username":"ab","email":"a@b.co","password":"secret1"}`, 400, "username must be 3 to 30 characters"},
		{"bad email", `{"username":"good_name","email":"nope","password":"secret1"}`, 400, "a valid email address is required"},
		{"short password", `{"username":"good_name","email":"a@b.co","password":"123"}`, 400, "password must be at least 6 characters"},
		{"missing password", `{"username":"good_name","email":"a@b.co"}`, 400, "password is required"},
		{"password over 72 bytes", `{"username":"good_name","email":"a@b.co","password":"` + long + `"}`, 400, "password must be 72 bytes or fewer"},
		{"bad username characters", `{"username":"bad name!","email":"a@b.co","password":"secret1"}`, 400, "username may only contain letters, numbers, and underscores"},
		{"not json", `not json`, 400, "request body is not valid JSON, or a field has the wrong type"},
		{"truncated json", `{`, 400, "request body is not valid JSON, or a field has the wrong type"},
		{"wrong field type", `{"username":5,"email":"a@b.co","password":"secret1"}`, 400, "request body is not valid JSON, or a field has the wrong type"},
		{"empty body", ``, 400, "request body is empty"},
		{"body too large", `{"username":"good_name","email":"a@b.co","password":"` + strings.Repeat("x", 2000) + `"}`, 413, "request body is too large"},
	})
}

func TestLoginRejectsBadInput(t *testing.T) {
	runCases(t, authRouter(), http.MethodPost, "/login", []wantError{
		{"missing email", `{"password":"secret1"}`, 400, "a valid email address is required"},
		{"bad email", `{"email":"nope","password":"secret1"}`, 400, "a valid email address is required"},
		{"missing password", `{"email":"a@b.co"}`, 400, "password is required"},
		{"not json", `nope`, 400, "request body is not valid JSON, or a field has the wrong type"},
	})
}

func TestSubmitScoreRejectsBadInput(t *testing.T) {
	const games = "game_code must be one of: speed-run, trivia, puzzle"
	runCases(t, scoreRouter(), http.MethodPost, "/scores", []wantError{
		{"unknown game", `{"game_code":"nope","points":10}`, 400, games},
		{"global board is not writable", `{"game_code":"global","points":10}`, 400, games},
		{"missing game", `{"points":10}`, 400, "game_code is required"},
		{"zero points", `{"game_code":"trivia","points":0}`, 400, "points must be a whole number from 1 to 10000"},
		{"negative points", `{"game_code":"trivia","points":-5}`, 400, "points must be a whole number from 1 to 10000"},
		{"too many points", `{"game_code":"trivia","points":10001}`, 400, "points must be a whole number from 1 to 10000"},
		{"points as text", `{"game_code":"trivia","points":"lots"}`, 400, "request body is not valid JSON, or a field has the wrong type"},
		{"not json", `}`, 400, "request body is not valid JSON, or a field has the wrong type"},
	})
}

func TestReadEndpointsRejectUnknownGames(t *testing.T) {
	r := scoreRouter()
	cases := []struct {
		name, path string
		status     int
	}{
		{"top", "/leaderboard/nope/top", 404},
		{"me", "/leaderboard/nope/me", 404},
		{"rank", "/leaderboard/nope/rank/" + uuid.NewString(), 404},
		{"history filter", "/history?game_code=nope", 400},
		{"report filter", "/report?game_code=nope", 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := send(r, http.MethodGet, tc.path, "")
			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d (body %s)", w.Code, tc.status, w.Body.String())
			}
		})
	}
}
