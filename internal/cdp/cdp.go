package cdp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Request is one CDP command. SessionID targets an attached page (flat sessions).
type Request struct {
	ID        int64  `json:"id"`
	Method    string `json:"method"`
	Params    any    `json:"params,omitempty"`
	SessionID string `json:"sessionId,omitempty"`
}

// message is any incoming CDP message: a response (ID set) or an event (Method set).
type message struct {
	ID        int64           `json:"id,omitempty"`
	Method    string          `json:"method,omitempty"`
	Params    json.RawMessage `json:"params,omitempty"`
	Result    json.RawMessage `json:"result,omitempty"`
	Error     *Error          `json:"error,omitempty"`
	SessionID string          `json:"sessionId,omitempty"`
}

// Error is a CDP error response.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    string `json:"data,omitempty"`
}

func (e *Error) Error() string {
	if e.Data != "" {
		return fmt.Sprintf("%s (%d): %s", e.Message, e.Code, e.Data)
	}
	return fmt.Sprintf("%s (%d)", e.Message, e.Code)
}

// Client talks CDP over one WebSocket, synchronously: Call sends a command and reads
// until its response; events read on the way are kept for WaitEvent.
type Client struct {
	ws     *wsConn
	nextID int64
	events []message
}

// Dial connects to a DevTools WebSocket URL, for example the browser's
// ws://127.0.0.1:PORT/devtools/browser/ID.
func Dial(ctx context.Context, wsURL string) (*Client, error) {
	ws, err := dialWS(ctx, wsURL)
	if err != nil {
		return nil, err
	}
	return &Client{ws: ws}, nil
}

// Close closes the connection.
func (c *Client) Close() error { return c.ws.Close() }

// encode builds the JSON for the next command.
func (c *Client) encode(session, method string, params any) (int64, []byte, error) {
	c.nextID++
	b, err := json.Marshal(Request{ID: c.nextID, Method: method, Params: params, SessionID: session})
	return c.nextID, b, err
}

// deadline applies ctx's deadline to the connection and interrupts a blocked read when
// ctx is cancelled; call the returned stop when done.
func (c *Client) deadline(ctx context.Context) (stop func() bool) {
	dl, _ := ctx.Deadline()
	_ = c.ws.SetDeadline(dl)
	return context.AfterFunc(ctx, func() { _ = c.ws.SetDeadline(time.Now()) })
}

func (c *Client) read() (message, error) {
	b, err := c.ws.ReadMessage()
	if err != nil {
		return message{}, err
	}
	var m message
	if err := json.Unmarshal(b, &m); err != nil {
		return message{}, fmt.Errorf("cdp: bad message: %w", err)
	}
	return m, nil
}

// Call sends method with params to session ("" for the browser) and decodes the
// result into result unless it is nil.
func (c *Client) Call(ctx context.Context, session, method string, params, result any) error {
	defer c.deadline(ctx)()
	id, b, err := c.encode(session, method, params)
	if err != nil {
		return err
	}
	if err := c.ws.WriteText(b); err != nil {
		return fmt.Errorf("%s: %w", method, err)
	}
	for {
		m, err := c.read()
		if err != nil {
			return fmt.Errorf("%s: %w", method, err)
		}
		if m.ID == 0 {
			c.events = append(c.events, m)
			continue
		}
		if m.ID != id {
			continue
		}
		if m.Error != nil {
			return fmt.Errorf("%s: %w", method, m.Error)
		}
		if result == nil || len(m.Result) == 0 {
			return nil
		}
		return json.Unmarshal(m.Result, result)
	}
}

// DropEvents forgets the events read so far, so WaitEvent only sees newer ones.
func (c *Client) DropEvents() { c.events = nil }

// WaitEvent returns the params of the first event named method on session.
func (c *Client) WaitEvent(ctx context.Context, session, method string) (json.RawMessage, error) {
	for i, m := range c.events {
		if m.Method == method && m.SessionID == session {
			c.events = c.events[i+1:]
			return m.Params, nil
		}
	}
	c.events = nil
	defer c.deadline(ctx)()
	for {
		m, err := c.read()
		if err != nil {
			return nil, fmt.Errorf("waiting for %s: %w", method, err)
		}
		if m.Method == method && m.SessionID == session {
			return m.Params, nil
		}
	}
}
