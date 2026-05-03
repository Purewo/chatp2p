package realtime

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

type Event struct {
	Type   string    `json:"type"`
	Data   any       `json:"data"`
	SentAt time.Time `json:"sentAt"`
}

type InboundMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type ServeOptions struct {
	UserID       string
	Conn         *websocket.Conn
	OnConnect    func(context.Context, string, bool)
	OnDisconnect func(context.Context, string, bool)
	OnMessage    func(context.Context, string, InboundMessage)
}

type Hub struct {
	mu      sync.RWMutex
	clients map[string]map[*client]struct{}
}

type client struct {
	userID string
	conn   *websocket.Conn
	send   chan Event
}

func NewHub() *Hub {
	return &Hub{
		clients: make(map[string]map[*client]struct{}),
	}
}

func (h *Hub) Serve(ctx context.Context, opts ServeOptions) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	c := &client{
		userID: opts.UserID,
		conn:   opts.Conn,
		send:   make(chan Event, 32),
	}

	firstConnection := h.register(c)
	if opts.OnConnect != nil {
		opts.OnConnect(ctx, opts.UserID, firstConnection)
	}
	defer func() {
		lastConnection := h.unregister(c)
		if opts.OnDisconnect != nil {
			opts.OnDisconnect(context.Background(), opts.UserID, lastConnection)
		}
	}()
	defer opts.Conn.Close(websocket.StatusNormalClosure, "connection closed")

	errCh := make(chan error, 2)
	go c.writeLoop(ctx, errCh)
	go c.readLoop(ctx, errCh, opts.OnMessage)

	<-errCh
}

func (h *Hub) Publish(userIDs []string, event Event) {
	if event.SentAt.IsZero() {
		event.SentAt = time.Now().UTC()
	}

	seen := make(map[string]struct{}, len(userIDs))
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, userID := range userIDs {
		if _, ok := seen[userID]; ok {
			continue
		}
		seen[userID] = struct{}{}

		for c := range h.clients[userID] {
			select {
			case c.send <- event:
			default:
			}
		}
	}
}

func (h *Hub) register(c *client) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	firstConnection := len(h.clients[c.userID]) == 0
	if h.clients[c.userID] == nil {
		h.clients[c.userID] = make(map[*client]struct{})
	}
	h.clients[c.userID][c] = struct{}{}
	return firstConnection
}

func (h *Hub) unregister(c *client) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.clients[c.userID] == nil {
		return false
	}
	delete(h.clients[c.userID], c)
	close(c.send)
	if len(h.clients[c.userID]) == 0 {
		delete(h.clients, c.userID)
		return true
	}
	return false
}

func (c *client) writeLoop(ctx context.Context, errCh chan<- error) {
	for {
		select {
		case <-ctx.Done():
			errCh <- ctx.Err()
			return
		case event, ok := <-c.send:
			if !ok {
				errCh <- nil
				return
			}
			if err := wsjson.Write(ctx, c.conn, event); err != nil {
				errCh <- err
				return
			}
		}
	}
}

func (c *client) readLoop(ctx context.Context, errCh chan<- error, onMessage func(context.Context, string, InboundMessage)) {
	for {
		var message InboundMessage
		if err := wsjson.Read(ctx, c.conn, &message); err != nil {
			errCh <- err
			return
		}
		if onMessage != nil {
			onMessage(ctx, c.userID, message)
		}
	}
}
