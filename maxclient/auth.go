package maxclient

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"
)

const defaultKeepaliveInterval = 30 * time.Second

// newUUID generates a UUID v4 string using crypto/rand (no external dependency).
func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("maxclient: crypto/rand failed: " + err.Error())
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 2
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// withKeepaliveInterval overrides the keepalive interval (for testing).
func withKeepaliveInterval(d time.Duration) Option {
	return func(c *Client) { c.keepaliveInterval = d }
}

// buildLoginPayload constructs the login_by_token payload.
func buildLoginPayload(token string) map[string]any {
	return map[string]any{
		"interactive":  true,
		"token":        token,
		"chatsCount":   40,
		"chatsSync":    0,
		"contactsSync": 0,
		"presenceSync": -1,
		"draftsSync":   0,
	}
}

// buildHelloPayload constructs the hello payload for the given device ID.
func (c *Client) buildHelloPayload(deviceID string) map[string]any {
	return map[string]any{
		"userAgent": map[string]any{
			"deviceType":      "WEB",
			"locale":          "ru",
			"deviceLocale":    "ru",
			"osVersion":       "Linux",
			"deviceName":      "Chrome",
			"headerUserAgent": c.userAgent,
			"appVersion":      AppVersion,
			"screen":          "1080x1920 1.0x",
			"timezone":        "Europe/Moscow",
		},
		"deviceId": deviceID,
	}
}

// sendHello sends the hello packet (opcode 6) with device info.
func (c *Client) sendHello(ctx context.Context, deviceID string) error {
	if deviceID == "" {
		deviceID = newUUID()
	}
	c.deviceID = deviceID
	_, err := c.InvokeMethod(ctx, OpcodeHello, c.buildHelloPayload(deviceID))
	return err
}

// AuthToken authenticates using a previously obtained token and device ID.
// Sends hello -> login_by_token -> starts keepalive.
func (c *Client) AuthToken(ctx context.Context, token, deviceID string) error {
	if err := c.sendHello(ctx, deviceID); err != nil {
		return fmt.Errorf("hello: %w", err)
	}

	resp, err := c.InvokeMethod(ctx, OpcodeLoginByToken, buildLoginPayload(token))
	if err != nil {
		return fmt.Errorf("login_by_token: %w", err)
	}

	// Check for error in response
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(resp.Payload, &payload); err == nil {
		if _, hasErr := payload["error"]; hasErr {
			var errMsg string
			json.Unmarshal(payload["error"], &errMsg)
			return fmt.Errorf("login_by_token: server error: %s", errMsg)
		}
	}

	// Extract own user ID from profile
	if raw, ok := payload["profile"]; ok {
		var profileData struct {
			Contact struct {
				ID int64 `json:"id"`
			} `json:"contact"`
		}
		if json.Unmarshal(raw, &profileData) == nil {
			c.ownUserID = profileData.Contact.ID
		}
	}

	c.token = token
	c.log.Info("authenticated by token")

	c.startKeepalive()
	return nil
}

// StartQRAuth begins QR-code authentication.
// Sends hello -> requests QR code (opcode 288) -> stores trackId internally -> returns the QR link.
// Consumer should display the link as a QR code, then call WaitQRAuth.
func (c *Client) StartQRAuth(ctx context.Context) (string, error) {
	deviceID := newUUID()
	if err := c.sendHello(ctx, deviceID); err != nil {
		return "", fmt.Errorf("hello: %w", err)
	}

	// Request QR code
	qrResp, err := c.InvokeMethod(ctx, OpcodeGetQR, map[string]any{})
	if err != nil {
		return "", fmt.Errorf("get QR: %w", err)
	}

	var qrPayload struct {
		QRLink          string `json:"qrLink"`
		TrackID         string `json:"trackId"`
		PollingInterval int    `json:"pollingInterval"`
		ExpiresAt       int64  `json:"expiresAt"`
	}
	if err := json.Unmarshal(qrResp.Payload, &qrPayload); err != nil {
		return "", fmt.Errorf("parse QR response: %w", err)
	}

	// Store QR state internally for WaitQRAuth
	c.qrTrackID = qrPayload.TrackID
	c.qrPollInterval = time.Duration(qrPayload.PollingInterval) * time.Millisecond
	if c.qrPollInterval == 0 {
		c.qrPollInterval = 3 * time.Second
	}
	c.qrExpiresAt = qrPayload.ExpiresAt

	return qrPayload.QRLink, nil
}

// WaitQRAuth waits for the QR code to be scanned, then exchanges for a token.
// Blocks until the QR code is scanned or the context is cancelled.
// Must be called after StartQRAuth.
func (c *Client) WaitQRAuth(ctx context.Context) (*QRAuth, error) {
	if c.qrTrackID == "" {
		return nil, fmt.Errorf("maxclient: StartQRAuth must be called first")
	}

	// Poll until scanned
	for {
		timer := time.NewTimer(c.qrPollInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}

		if c.qrExpiresAt > 0 && time.Now().UnixMilli() > c.qrExpiresAt {
			return nil, ErrQRExpired
		}

		statusResp, err := c.InvokeMethod(ctx, OpcodeGetQRStatus, map[string]any{
			"trackId": c.qrTrackID,
		})
		if err != nil {
			return nil, fmt.Errorf("QR status: %w", err)
		}

		var statusPayload struct {
			Status struct {
				LoginAvailable bool `json:"loginAvailable"`
			} `json:"status"`
		}
		if err := json.Unmarshal(statusResp.Payload, &statusPayload); err != nil {
			continue
		}
		if statusPayload.Status.LoginAvailable {
			break
		}
	}

	// Exchange trackId for token (opcode 291)
	loginResp, err := c.InvokeMethod(ctx, OpcodeLoginByQR, map[string]any{
		"trackId": c.qrTrackID,
	})
	if err != nil {
		return nil, fmt.Errorf("login by QR: %w", err)
	}

	var loginPayload struct {
		TokenAttrs struct {
			Login struct {
				Token string `json:"token"`
			} `json:"LOGIN"`
		} `json:"tokenAttrs"`
		Chats []struct {
			ID           int64          `json:"id"`
			Type         string         `json:"type"`
			Participants map[string]any `json:"participants"`
		} `json:"chats"`
	}
	if err := json.Unmarshal(loginResp.Payload, &loginPayload); err != nil {
		return nil, fmt.Errorf("parse login response: %w", err)
	}

	token := loginPayload.TokenAttrs.Login.Token
	c.token = token

	// After QR exchange, call loginByToken to fully initialize the session
	// with chat subscriptions. Without this, the server closes the connection
	// after the first message.
	lbtResp, err := c.InvokeMethod(ctx, OpcodeLoginByToken, buildLoginPayload(token))
	if err != nil {
		return nil, fmt.Errorf("login_by_token after QR: %w", err)
	}

	// Extract own user ID from profile
	var lbtPayload map[string]json.RawMessage
	if json.Unmarshal(lbtResp.Payload, &lbtPayload) == nil {
		if raw, ok := lbtPayload["profile"]; ok {
			var profileData struct {
				Contact struct {
					ID int64 `json:"id"`
				} `json:"contact"`
			}
			if json.Unmarshal(raw, &profileData) == nil {
				c.ownUserID = profileData.Contact.ID
			}
		}
	}

	// Resolve Favorites chat ID (single-participant DIALOG chat)
	chatID := int64(0)
	for _, chat := range loginPayload.Chats {
		if chat.Type == "DIALOG" && len(chat.Participants) == 1 {
			chatID = chat.ID
			break
		}
	}

	c.log.Info("authenticated by QR")
	c.startKeepalive()

	return &QRAuth{
		Token:    token,
		DeviceID: c.deviceID,
		ChatID:   chatID,
	}, nil
}

// startKeepalive starts the keepalive goroutine idempotently.
// If a keepalive goroutine is already running, it is cancelled before starting a new one.
func (c *Client) startKeepalive() {
	// Cancel existing keepalive if running
	if c.keepaliveCancel != nil {
		c.keepaliveCancel()
	}

	// Derive keepalive context from lifecycleCtx
	keepCtx, keepCancel := context.WithCancel(c.lifecycleCtx)
	c.keepaliveCancel = keepCancel

	go c.keepaliveLoop(keepCtx)
}

// keepaliveLoop sends keepalive packets periodically.
// When keepalive fails, force-closes the WebSocket connection to trigger recvLoop disconnect.
func (c *Client) keepaliveLoop(ctx context.Context) {
	interval := c.keepaliveInterval
	if interval == 0 {
		interval = defaultKeepaliveInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.mu.Lock()
			conn := c.conn
			c.mu.Unlock()
			if conn == nil {
				return
			}

			invokeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			_, err := c.InvokeMethod(invokeCtx, OpcodeKeepalive, map[string]any{"interactive": false})
			cancel()

			if err != nil {
				c.log.Warn("keepalive failed, force closing connection", "err", err)
				// Force close the connection to trigger recvLoop disconnect
				c.mu.Lock()
				if c.conn != nil {
					c.conn.CloseNow()
				}
				c.mu.Unlock()
				return
			}
		case <-ctx.Done():
			return
		}
	}
}
