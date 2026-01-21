package jsonrpc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

// Client manages JSON-RPC 2.0 communication over stdin/stdout.
type Client struct {
	mu sync.Mutex

	writer  io.Writer
	encoder *json.Encoder

	// pending tracks in-flight requests awaiting responses
	pending map[int64]chan *Response

	// nextID is atomically incremented for request IDs
	nextID atomic.Int64

	// notifications receives server-initiated notifications
	notifications chan *Notification

	// done signals the read pump to stop
	done chan struct{}

	// closed indicates the client has been shut down
	closed bool

	// readErr stores any error from the read pump
	readErr error
}

// NewClient creates a new JSON-RPC client from a reader/writer pair.
// Typically the reader is stdout from the subprocess and writer is stdin.
func NewClient(r io.Reader, w io.Writer) *Client {
	c := &Client{
		writer:        w,
		encoder:       json.NewEncoder(w),
		pending:       make(map[int64]chan *Response),
		notifications: make(chan *Notification, 100), // Buffer notifications
		done:          make(chan struct{}),
	}

	// Start the read pump
	go c.readPump(r)

	return c
}

// readPump continuously reads messages from the reader and dispatches them.
func (c *Client) readPump(r io.Reader) {
	scanner := bufio.NewScanner(r)
	// Allow large messages (10MB max)
	scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)

	for scanner.Scan() {
		select {
		case <-c.done:
			return
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var msg Message
		if err := json.Unmarshal(line, &msg); err != nil {
			// Log parse error but continue - could be garbage or partial line
			continue
		}

		if msg.IsResponse() {
			c.handleResponse(msg.AsResponse())
		} else if msg.IsNotification() {
			c.handleNotification(msg.AsNotification())
		}
		// Ignore unknown message types
	}

	// Store scanner error
	if err := scanner.Err(); err != nil {
		c.mu.Lock()
		c.readErr = err
		c.mu.Unlock()
	}

	// Close all pending requests with error
	c.mu.Lock()
	for id, ch := range c.pending {
		close(ch)
		delete(c.pending, id)
	}
	c.mu.Unlock()

	// Close notifications channel to signal EOF
	close(c.notifications)
}

// handleResponse dispatches a response to its waiting caller.
func (c *Client) handleResponse(resp *Response) {
	if resp.ID == nil {
		return
	}

	c.mu.Lock()
	ch, ok := c.pending[*resp.ID]
	if ok {
		delete(c.pending, *resp.ID)
	}
	c.mu.Unlock()

	if ok {
		ch <- resp
		close(ch)
	}
}

// handleNotification sends a notification to the notifications channel.
func (c *Client) handleNotification(notif *Notification) {
	select {
	case c.notifications <- notif:
	default:
		// Drop notification if channel is full - shouldn't happen with buffering
	}
}

// Call makes a synchronous JSON-RPC call and waits for the response.
func (c *Client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("client is closed")
	}

	id := c.nextID.Add(1)
	req := NewRequest(id, method, params)

	// Create response channel before sending
	respChan := make(chan *Response, 1)
	c.pending[id] = respChan
	c.mu.Unlock()

	// Send the request
	c.mu.Lock()
	err := c.encoder.Encode(req)
	c.mu.Unlock()

	if err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	// Wait for response or context cancellation
	select {
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()

	case resp, ok := <-respChan:
		if !ok {
			// Channel closed - read pump exited
			c.mu.Lock()
			err := c.readErr
			c.mu.Unlock()
			if err != nil {
				return nil, fmt.Errorf("connection closed: %w", err)
			}
			return nil, errors.New("connection closed")
		}

		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	}
}

// Notifications returns the channel for receiving server notifications.
// The channel is closed when the connection ends.
func (c *Client) Notifications() <-chan *Notification {
	return c.notifications
}

// Close shuts down the client.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	close(c.done)

	return nil
}

// Err returns any error from the read pump.
func (c *Client) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.readErr
}
