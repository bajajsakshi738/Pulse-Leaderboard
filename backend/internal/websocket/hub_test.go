package websocket

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
)

func TestOriginAllowed(t *testing.T) {
	allowed := []string{"https://pulse.vercel.app", "http://localhost:4200/"}

	cases := []struct {
		name   string
		origin string
		want   bool
	}{
		{"exact match", "https://pulse.vercel.app", true},
		{"configured with trailing slash", "http://localhost:4200", true},
		{"case differs", "HTTPS://PULSE.VERCEL.APP", true},
		{"other website", "https://evil.example.com", false},
		{"same host different scheme", "http://pulse.vercel.app", false},
		{"same host different port", "http://localhost:3000", false},
		{"suffix trick", "https://pulse.vercel.app.evil.com", false},
		{"no origin header (non-browser client)", "", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := OriginAllowed(tc.origin, allowed); got != tc.want {
				t.Fatalf("OriginAllowed(%q) = %v, want %v", tc.origin, got, tc.want)
			}
		})
	}
}

func TestOriginAllowedEmptyList(t *testing.T) {
	if OriginAllowed("https://anything.com", nil) {
		t.Fatal("with no allowed origins, a browser origin must be rejected")
	}
}

// TestServeWSEnforcesOriginWithoutCORS uses a bare Gin router (no CORS) to
// show the hub checks the origin by itself.
func TestServeWSEnforcesOriginWithoutCORS(t *testing.T) {
	gin.SetMode(gin.TestMode)
	hub := NewHub([]string{"https://pulse.vercel.app"})
	go hub.Run()

	r := gin.New()
	r.GET("/ws", hub.ServeWS)
	srv := httptest.NewServer(r)
	defer srv.Close()
	url := "ws" + strings.TrimPrefix(srv.URL, "http") + "/ws"

	// allowed origin connects.
	conn, resp, err := gorillaws.DefaultDialer.Dial(url, http.Header{"Origin": {"https://pulse.vercel.app"}})
	if err != nil {
		t.Fatalf("allowed origin should connect: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("allowed origin: status %d", resp.StatusCode)
	}
	conn.Close()

	// another website gets 403.
	_, resp, err = gorillaws.DefaultDialer.Dial(url, http.Header{"Origin": {"https://evil.example.com"}})
	if err == nil {
		t.Fatal("evil origin should be refused")
	}
	if resp == nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("evil origin: want 403, got %+v", resp)
	}
}
