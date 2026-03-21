package maxclient

import (
	"context"
	"fmt"
	"time"

	"github.com/coder/websocket"
)

// calcBackoff returns the backoff duration for the given attempt number.
func calcBackoff(attempt int, min, max time.Duration) time.Duration {
	d := min
	for i := 0; i < attempt; i++ {
		d *= 2
		if d > max {
			return max
		}
	}
	return d
}

// reconnectAuth performs hello + login_by_token using invokeInternal
// (which does not wait on reconnectCh) to avoid blocking,
// since reconnectLoop has set reconnectCh to a blocked channel.
func (c *Client) reconnectAuth(ctx context.Context, token, deviceID string) error {
	// Send hello
	if _, err := c.invokeInternal(ctx, OpcodeHello, c.buildHelloPayload(deviceID)); err != nil {
		return fmt.Errorf("hello: %w", err)
	}

	// Login by token
	resp, err := c.invokeInternal(ctx, OpcodeLoginByToken, buildLoginPayload(token))
	if err != nil {
		return fmt.Errorf("login_by_token: %w", err)
	}

	// Check for error in response
	if err := checkResponseError(resp); err != nil {
		return fmt.Errorf("login_by_token: %w", err)
	}

	c.log.Info("authenticated by token")
	c.startKeepalive()
	return nil
}

// reconnectLoop attempts to re-establish the connection with exponential backoff.
// reconnectDone is created by recvLoop; closing it unblocks InvokeMethod waiters.
// Uses lifecycleCtx so that Close() can terminate the loop.
func (c *Client) reconnectLoop(reconnectDone chan struct{}) {
	// Ensure only one reconnectLoop runs at a time
	if !c.reconnectRunning.CompareAndSwap(false, true) {
		close(reconnectDone) // unblock waiters if CAS fails
		return
	}
	defer c.reconnectRunning.Store(false)
	defer close(reconnectDone) // unblock all InvokeMethod waiters

	for attempt := 0; ; attempt++ {
		backoff := calcBackoff(attempt, c.reconnectMin, c.reconnectMax)
		c.log.Info("reconnecting", "attempt", attempt+1, "backoff", backoff)

		select {
		case c.errors <- ErrReconnecting:
		default:
		}

		timer := time.NewTimer(backoff)
		select {
		case <-timer.C:
		case <-c.lifecycleCtx.Done():
			timer.Stop()
			return
		}

		// Wait for old recv loop to fully terminate before reconnecting
		if c.done != nil {
			<-c.done
		}

		// Attempt reconnect
		if err := c.Connect(c.lifecycleCtx); err != nil {
			c.log.Warn("reconnect failed", "err", err)
			continue
		}

		// Re-authenticate using internal path (avoids blocking on reconnectCh)
		if c.token != "" && c.deviceID != "" {
			if err := c.reconnectAuth(c.lifecycleCtx, c.token, c.deviceID); err != nil {
				c.log.Warn("re-auth failed", "err", err)
				// Close the just-opened connection so Connect() can be called again.
				// Do NOT call c.Close() — that would cancel lifecycleCtx and prevent further attempts.
				c.mu.Lock()
				if c.conn != nil {
					c.cancel()
					c.conn.Close(websocket.StatusNormalClosure, "")
					<-c.done
					c.conn = nil
				}
				c.mu.Unlock()
				continue
			}
		}

		c.log.Info("reconnected successfully")
		select {
		case c.errors <- ErrReconnected:
		default:
		}
		return
	}
}
