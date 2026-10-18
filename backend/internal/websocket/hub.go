package websocket

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// Hub keeps track of connected WebSocket clients and sends a message to all
// of them. the broadcaster calls Broadcast on a timer.
type Hub struct {
	mu      sync.RWMutex
	clients map[*Client]bool

	register   chan *Client
	unregister chan *Client

	upgrader websocket.Upgrader
}

type Client struct {
	conn *websocket.Conn
	send chan []byte
}

// NewHub only accepts connections from the allowed origins (same list as CORS).
func NewHub(allowedOrigins []string) *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin: func(r *http.Request) bool {
				ok := OriginAllowed(r.Header.Get("Origin"), allowedOrigins)
				if !ok {
					log.Printf("websocket: rejected origin %q", r.Header.Get("Origin"))
				}
				return ok
			},
		},
	}
}

// Run handles register and unregister. start it once with `go hub.Run()`.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()
		}
	}
}

// Broadcast sends the message to every client. if a client's queue is full
// it misses this update instead of blocking everyone. clients that stop
// responding get disconnected by the deadlines in readPump and writePump.
func (h *Hub) Broadcast(payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("websocket broadcast marshal error: %v", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		select {
		case client.send <- data:
		default:
			// queue full, skip this client for this tick.
		}
	}
}

// ClientCount is how many clients are connected.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// OriginAllowed checks the Origin header of a WebSocket handshake. browsers
// always send it, so pages on other websites are rejected. no Origin means
// a non-browser client like curl, which could fake it anyway, so it is
// allowed. this stops other sites, it is not a security wall.
func OriginAllowed(origin string, allowed []string) bool {
	if origin == "" {
		return true
	}
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	for _, a := range allowed {
		if strings.EqualFold(strings.TrimRight(strings.TrimSpace(a), "/"), origin) {
			return true
		}
	}
	return false
}

// ServeWS upgrades the request to a WebSocket and registers the client.
// each client gets its own read and write goroutines.
func (h *Hub) ServeWS(c *gin.Context) {
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("websocket upgrade error: %v", err)
		return
	}

	client := &Client{
		conn: conn,
		send: make(chan []byte, 16),
	}

	h.register <- client

	go client.writePump()
	go client.readPump(h)
}

// readPump reads and throws away incoming messages. we only push, but
// reading is needed so ping, pong and close frames get handled. it also
// unregisters the client when the connection ends.
func (c *Client) readPump(h *Hub) {
	defer func() {
		h.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(512)
	_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		if _, _, err := c.conn.ReadMessage(); err != nil {
			break
		}
	}
}

const (
	writeWait  = 10 * time.Second
	pingPeriod = 30 * time.Second
)

// writePump sends queued messages and pings now and then to keep the
// connection alive through proxies.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
