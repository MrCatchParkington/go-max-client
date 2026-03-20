package maxclient

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

// Option configures a Client.
type Option func(*Client)

// WithAutoReconnect enables automatic reconnection on disconnect.
func WithAutoReconnect(enabled bool) Option {
	return func(c *Client) { c.autoReconnect = enabled }
}

// WithReconnectBackoff sets the min and max backoff durations for reconnection.
func WithReconnectBackoff(min, max time.Duration) Option {
	return func(c *Client) {
		c.reconnectMin = min
		c.reconnectMax = max
	}
}

// WithUserAgent sets a custom User-Agent header.
func WithUserAgent(ua string) Option {
	return func(c *Client) { c.userAgent = ua }
}

// WithLogger sets a custom logger.
func WithLogger(l *slog.Logger) Option {
	return func(c *Client) { c.log = l }
}

// WithPacketBufferSize sets the buffer size for the Packets channel.
func WithPacketBufferSize(n int) Option {
	return func(c *Client) { c.packetBufSize = n }
}

// withWSURL overrides the WebSocket URL (for testing).
func withWSURL(url string) Option {
	return func(c *Client) { c.wsURL = url }
}

// Client is a MAX messenger API client.
type Client struct {
	// config
	wsURL             string
	userAgent         string
	log               *slog.Logger
	packetBufSize     int
	autoReconnect     bool
	reconnectMin      time.Duration
	reconnectMax      time.Duration
	keepaliveInterval time.Duration

	// state
	conn    *websocket.Conn
	seq     seqCounter
	pending sync.Map // seq -> chan *Packet

	// upload completion pending maps
	videoPending sync.Map // videoId (string) -> chan struct{}
	filePending  sync.Map // fileId (string) -> chan struct{}

	// auth state (saved for reconnect)
	token    string
	deviceID string

	// own profile ID (extracted from login response)
	ownUserID int64

	// channels
	packets chan *Packet
	errors  chan error

	// lifecycle
	lifecycleCtx    context.Context
	lifecycleCancel context.CancelFunc
	cancel          context.CancelFunc
	done            chan struct{}
	mu              sync.Mutex

	// keepalive
	keepaliveCancel context.CancelFunc

	// QR auth state (between StartQRAuth and WaitQRAuth)
	qrTrackID      string
	qrPollInterval time.Duration
	qrExpiresAt    int64

	// reconnect
	reconnecting     sync.RWMutex
	reconnectRunning atomic.Bool
}

// New creates a new Client with the given options.
func New(opts ...Option) *Client {
	c := &Client{
		wsURL:         WSURL,
		userAgent:     DefaultUserAgent,
		log:           slog.Default(),
		packetBufSize: 1024,
		reconnectMin:  time.Second,
		reconnectMax:  30 * time.Second,
	}
	for _, o := range opts {
		o(c)
	}
	c.lifecycleCtx, c.lifecycleCancel = context.WithCancel(context.Background())
	c.packets = make(chan *Packet, c.packetBufSize)
	c.errors = make(chan error, 8)
	return c
}

// Packets returns a channel of incoming unsolicited packets.
func (c *Client) Packets() <-chan *Packet {
	return c.packets
}

// Errors returns a channel of connection errors.
func (c *Client) Errors() <-chan error {
	return c.errors
}

// Connect establishes a WebSocket connection to the MAX server.
// Does not authenticate — call AuthToken or StartQRAuth/WaitQRAuth after connecting.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		return fmt.Errorf("maxclient: already connected")
	}

	conn, _, err := websocket.Dial(ctx, c.wsURL, &websocket.DialOptions{
		HTTPHeader: http.Header{
			"Origin":     []string{DefaultOrigin},
			"User-Agent": []string{c.userAgent},
		},
	})
	if err != nil {
		return fmt.Errorf("maxclient: connect: %w", err)
	}

	conn.SetReadLimit(10 * 1024 * 1024) // 10 MB
	c.conn = conn
	c.done = make(chan struct{})
	var recvCtx context.Context
	recvCtx, c.cancel = context.WithCancel(context.Background())
	go c.recvLoop(recvCtx)

	c.log.Info("connected", "url", c.wsURL)
	return nil
}

// Close gracefully shuts down the client. This is terminal — the client cannot be reused after Close.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Cancel lifecycle context — terminates keepalive, reconnect, and all background goroutines
	c.lifecycleCancel()

	if c.conn == nil {
		return nil
	}

	c.cancel()
	c.conn.Close(websocket.StatusNormalClosure, "")
	<-c.done

	// Drain all pending requests with ErrDisconnected
	c.pending.Range(func(key, value any) bool {
		ch := value.(chan *Packet)
		select {
		case ch <- nil: // signal the waiter
		default:
		}
		c.pending.Delete(key)
		return true
	})

	c.conn = nil
	c.log.Info("disconnected")
	return nil
}

// InvokeMethod sends a request and waits for the matching response.
// If auto-reconnect is in progress, blocks until reconnection completes (respecting ctx).
func (c *Client) InvokeMethod(ctx context.Context, opcode int, payload any) (*Packet, error) {
	// Block if reconnecting — reconnecting.Lock() is held during reconnect,
	// RLock() will block until reconnect completes
	c.reconnecting.RLock()
	defer c.reconnecting.RUnlock()

	return c.invokeInternal(ctx, opcode, payload)
}

// invokeInternal is the core of InvokeMethod without the reconnecting RLock.
// Used by reconnectLoop (which already holds the write lock) to avoid deadlock.
func (c *Client) invokeInternal(ctx context.Context, opcode int, payload any) (*Packet, error) {
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()

	if conn == nil {
		return nil, ErrNotConnected
	}

	data, seq, err := buildRequest(&c.seq, opcode, payload)
	if err != nil {
		return nil, err
	}
	ch := make(chan *Packet, 1)
	c.pending.Store(seq, ch)
	defer c.pending.Delete(seq)

	if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
		return nil, fmt.Errorf("maxclient: write: %w", err)
	}

	select {
	case pkt := <-ch:
		if pkt == nil {
			return nil, ErrDisconnected
		}
		return pkt, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// recvLoop reads packets from the WebSocket and dispatches them.
func (c *Client) recvLoop(ctx context.Context) {
	defer close(c.done)

	for {
		_, data, err := c.conn.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return // intentional shutdown
			}
			c.log.Warn("read error", "err", err)

			// Drain all pending requests with ErrDisconnected before triggering reconnect
			c.pending.Range(func(key, value any) bool {
				ch := value.(chan *Packet)
				select {
				case ch <- nil: // signal the waiter (nil means disconnected)
				default:
				}
				c.pending.Delete(key)
				return true
			})

			select {
			case c.errors <- ErrDisconnected:
			default:
			}

			if c.autoReconnect && c.token != "" {
				c.mu.Lock()
				c.conn = nil
				c.mu.Unlock()
				go c.reconnectLoop()
			}
			return
		}

		var pkt Packet
		if err := json.Unmarshal(data, &pkt); err != nil {
			c.log.Warn("invalid packet", "err", err)
			continue
		}

		// Check if this is a response to a pending request (seq > 0 to skip unsolicited events)
		if pkt.Seq > 0 {
			if ch, ok := c.pending.LoadAndDelete(pkt.Seq); ok {
				ch.(chan *Packet) <- &pkt
				continue
			}
		}

		// Check for upload completion (opcode 136)
		if pkt.Opcode == OpcodeUploadComplete {
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(pkt.Payload, &payload); err == nil {
				if videoID, ok := payload["videoId"]; ok {
					if id := unmarshalStringOrNumber(videoID); id != "" {
						if ch, ok := c.videoPending.LoadAndDelete(id); ok {
							close(ch.(chan struct{}))
						}
					}
				}
				if fileID, ok := payload["fileId"]; ok {
					if id := unmarshalStringOrNumber(fileID); id != "" {
						if ch, ok := c.filePending.LoadAndDelete(id); ok {
							close(ch.(chan struct{}))
						}
					}
				}
			}
		}

		// Deliver to packets channel
		select {
		case c.packets <- &pkt:
		default:
			c.log.Warn("packet channel full, dropping packet", "opcode", pkt.Opcode)
		}
	}
}
