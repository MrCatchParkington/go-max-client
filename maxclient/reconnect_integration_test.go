package maxclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestAutoReconnect(t *testing.T) {
	var mu sync.Mutex
	connectionCount := 0
	loginCount := 0

	// killConn is closed to tell the server to drop the first connection.
	killConn := make(chan struct{})
	var killed atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			return
		}
		defer conn.CloseNow()

		mu.Lock()
		connectionCount++
		connNum := connectionCount
		mu.Unlock()

		// For the first connection, monitor killConn and force-close when signaled
		if connNum == 1 {
			go func() {
				<-killConn
				if killed.CompareAndSwap(false, true) {
					conn.CloseNow()
				}
			}()
		}

		ctx := r.Context()
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}
			var pkt Packet
			if err := json.Unmarshal(data, &pkt); err != nil {
				continue
			}

			var resp Packet
			switch pkt.Opcode {
			case OpcodeHello:
				resp = Packet{Ver: RPCVersion, Seq: pkt.Seq, Opcode: pkt.Opcode, Payload: json.RawMessage(`{}`)}
			case OpcodeLoginByToken:
				mu.Lock()
				loginCount++
				mu.Unlock()
				resp = Packet{Ver: RPCVersion, Seq: pkt.Seq, Opcode: pkt.Opcode, Payload: json.RawMessage(`{"profile":{}}`)}
			case OpcodeKeepalive:
				resp = Packet{Ver: RPCVersion, Seq: pkt.Seq, Opcode: pkt.Opcode, Payload: json.RawMessage(`{}`)}
			default:
				resp = Packet{Ver: RPCVersion, Seq: pkt.Seq, Opcode: pkt.Opcode, Payload: json.RawMessage(`{}`)}
			}

			respData, _ := json.Marshal(resp)
			if err := conn.Write(ctx, websocket.MessageText, respData); err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	c := New(
		withWSURL(wsURL),
		WithAutoReconnect(true),
		WithReconnectBackoff(50*time.Millisecond, 200*time.Millisecond),
		withKeepaliveInterval(10*time.Second),
	)
	ctx := context.Background()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := c.AuthToken(ctx, "tok", "dev"); err != nil {
		t.Fatalf("auth: %v", err)
	}

	// Verify initial state
	mu.Lock()
	if connectionCount != 1 {
		t.Fatalf("initial connectionCount = %d, want 1", connectionCount)
	}
	if loginCount != 1 {
		t.Fatalf("initial loginCount = %d, want 1", loginCount)
	}
	mu.Unlock()

	// Kill the active connection to trigger disconnect + reconnect
	close(killConn)

	// Wait for reconnect to complete
	deadline := time.After(5 * time.Second)
	for {
		time.Sleep(100 * time.Millisecond)
		mu.Lock()
		cc := connectionCount
		lc := loginCount
		mu.Unlock()
		if cc >= 2 && lc >= 2 {
			break
		}
		select {
		case <-deadline:
			mu.Lock()
			t.Fatalf("timeout: connectionCount = %d, loginCount = %d", connectionCount, loginCount)
			mu.Unlock()
		default:
		}
	}

	c.Close()

	mu.Lock()
	defer mu.Unlock()
	if connectionCount < 2 {
		t.Errorf("connectionCount = %d, want >= 2", connectionCount)
	}
	if loginCount < 2 {
		t.Errorf("loginCount = %d, want >= 2", loginCount)
	}
}
