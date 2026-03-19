package maxclient

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestConnectAndClose(t *testing.T) {
	srv, wsURL := mockWSServer(nil)
	defer srv.Close()

	c := New(withWSURL(wsURL))
	ctx := context.Background()

	if err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	if err := c.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
}

func TestInvokeMethod(t *testing.T) {
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeKeepalive: respondOK(`{"status":"ok"}`),
	})
	defer srv.Close()

	c := New(withWSURL(wsURL))
	ctx := context.Background()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	resp, err := c.InvokeMethod(ctx, OpcodeKeepalive, map[string]any{"interactive": false})
	if err != nil {
		t.Fatalf("invoke: %v", err)
	}
	if resp.Opcode != OpcodeKeepalive {
		t.Errorf("opcode = %d, want %d", resp.Opcode, OpcodeKeepalive)
	}
}

func TestInvokeMethodTimeout(t *testing.T) {
	// Server that never responds
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeKeepalive: func(ctx context.Context, conn *websocket.Conn, pkt Packet) error {
			<-ctx.Done()
			return ctx.Err()
		},
	})
	defer srv.Close()

	c := New(withWSURL(wsURL))
	ctx := context.Background()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()

	_, err := c.InvokeMethod(ctx, OpcodeKeepalive, map[string]any{})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestPacketsChannel(t *testing.T) {
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeKeepalive: func(ctx context.Context, conn *websocket.Conn, pkt Packet) error {
			// Respond to the request
			resp, _ := json.Marshal(Packet{Ver: RPCVersion, Seq: pkt.Seq, Opcode: pkt.Opcode, Payload: json.RawMessage(`{}`)})
			if err := conn.Write(ctx, websocket.MessageText, resp); err != nil {
				return err
			}
			// Then send an unsolicited message event
			evt, _ := json.Marshal(Packet{
				Ver: RPCVersion, Seq: 0, Opcode: OpcodeMessageEvent,
				Payload: json.RawMessage(`{"chatId":0,"message":{"id":"m1","text":"hi","status":"NORMAL"}}`),
			})
			return conn.Write(ctx, websocket.MessageText, evt)
		},
	})
	defer srv.Close()

	c := New(withWSURL(wsURL))
	ctx := context.Background()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	// Trigger the server to send the unsolicited event
	if _, err := c.InvokeMethod(ctx, OpcodeKeepalive, map[string]any{}); err != nil {
		t.Fatalf("invoke: %v", err)
	}

	select {
	case pkt := <-c.Packets():
		if pkt.Opcode != OpcodeMessageEvent {
			t.Errorf("opcode = %d, want %d", pkt.Opcode, OpcodeMessageEvent)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for packet")
	}
}

func TestInvokeMethodNotConnected(t *testing.T) {
	c := New()
	_, err := c.InvokeMethod(context.Background(), OpcodeKeepalive, nil)
	if err != ErrNotConnected {
		t.Errorf("err = %v, want ErrNotConnected", err)
	}
}
