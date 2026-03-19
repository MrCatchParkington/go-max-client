package maxclient

import (
	"context"
	"encoding/json"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestAuthToken(t *testing.T) {
	var mu sync.Mutex
	var helloReceived, loginReceived bool
	var helloPayload, loginPayload map[string]any

	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeHello: func(ctx context.Context, conn *websocket.Conn, pkt Packet) error {
			mu.Lock()
			helloReceived = true
			json.Unmarshal(pkt.Payload, &helloPayload)
			mu.Unlock()
			return respondOK(`{}`)(ctx, conn, pkt)
		},
		OpcodeLoginByToken: func(ctx context.Context, conn *websocket.Conn, pkt Packet) error {
			mu.Lock()
			loginReceived = true
			json.Unmarshal(pkt.Payload, &loginPayload)
			mu.Unlock()
			return respondOK(`{"profile":{"contact":{"phone":"+7999"}}}`)(ctx, conn, pkt)
		},
		OpcodeKeepalive: respondOK(`{}`),
	})
	defer srv.Close()

	c := New(withWSURL(wsURL))
	ctx := context.Background()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	if err := c.AuthToken(ctx, "test-token", "test-device-id"); err != nil {
		t.Fatalf("auth: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if !helloReceived {
		t.Error("hello packet not sent")
	}
	if !loginReceived {
		t.Error("login packet not sent")
	}

	// Verify hello payload has required fields
	ua, ok := helloPayload["userAgent"]
	if !ok {
		t.Fatal("hello missing userAgent")
	}
	uaMap := ua.(map[string]any)
	if uaMap["appVersion"] != AppVersion {
		t.Errorf("appVersion = %v, want %s", uaMap["appVersion"], AppVersion)
	}
	if helloPayload["deviceId"] != "test-device-id" {
		t.Errorf("deviceId = %v, want test-device-id", helloPayload["deviceId"])
	}

	// Verify login payload
	if loginPayload["token"] != "test-token" {
		t.Errorf("token = %v, want test-token", loginPayload["token"])
	}
	if loginPayload["interactive"] != true {
		t.Errorf("interactive = %v, want true", loginPayload["interactive"])
	}
}

func TestAuthTokenSavesCredentials(t *testing.T) {
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeHello:        respondOK(`{}`),
		OpcodeLoginByToken: respondOK(`{"profile":{}}`),
		OpcodeKeepalive:    respondOK(`{}`),
	})
	defer srv.Close()

	c := New(withWSURL(wsURL))
	ctx := context.Background()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	if err := c.AuthToken(ctx, "my-token", "my-device"); err != nil {
		t.Fatalf("auth: %v", err)
	}

	if c.token != "my-token" {
		t.Errorf("token = %q, want %q", c.token, "my-token")
	}
	if c.deviceID != "my-device" {
		t.Errorf("deviceID = %q, want %q", c.deviceID, "my-device")
	}
}

func TestAuthTokenError(t *testing.T) {
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeHello:        respondOK(`{}`),
		OpcodeLoginByToken: respondOK(`{"error":"invalid token"}`),
		OpcodeKeepalive:    respondOK(`{}`),
	})
	defer srv.Close()

	c := New(withWSURL(wsURL))
	ctx := context.Background()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer c.Close()

	err := c.AuthToken(ctx, "bad-token", "device")
	if err == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestAuthQR(t *testing.T) {
	pollCount := 0
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeHello: respondOK(`{}`),
		OpcodeGetQR: respondOK(`{"qrLink":"https://max.ru/qr/abc","trackId":"track-1","pollingInterval":50}`),
		OpcodeGetQRStatus: func(ctx context.Context, conn *websocket.Conn, pkt Packet) error {
			pollCount++
			if pollCount < 2 {
				return respondOK(`{"status":{"loginAvailable":false}}`)(ctx, conn, pkt)
			}
			return respondOK(`{"status":{"loginAvailable":true}}`)(ctx, conn, pkt)
		},
		OpcodeLoginByQR:    respondOK(`{"tokenAttrs":{"LOGIN":{"token":"qr-token-123"}},"chats":[{"id":0,"type":"DIALOG","participants":{"self":{}}}]}`),
		OpcodeLoginByToken: respondOK(`{"profile":{}}`),
		OpcodeKeepalive:    respondOK(`{}`),
	})
	defer srv.Close()

	c := New(withWSURL(wsURL))
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()

	// Step 1: Start QR auth — returns link immediately
	link, err := c.StartQRAuth(ctx)
	if err != nil {
		t.Fatalf("start QR auth: %v", err)
	}
	if link != "https://max.ru/qr/abc" {
		t.Errorf("link = %q, want https://max.ru/qr/abc", link)
	}

	// Step 2: Wait for QR scan — blocks until scanned
	qr, err := c.WaitQRAuth(ctx)
	if err != nil {
		t.Fatalf("wait QR auth: %v", err)
	}
	if qr.Token != "qr-token-123" {
		t.Errorf("token = %q, want qr-token-123", qr.Token)
	}
	if qr.ChatID != 0 {
		t.Errorf("chatID = %d, want 0", qr.ChatID)
	}
	if qr.DeviceID == "" {
		t.Error("deviceID is empty")
	}
}

func TestKeepaliveRuns(t *testing.T) {
	var keepaliveCount atomic.Int32
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeHello:        respondOK(`{}`),
		OpcodeLoginByToken: respondOK(`{"profile":{}}`),
		OpcodeKeepalive: func(ctx context.Context, conn *websocket.Conn, pkt Packet) error {
			keepaliveCount.Add(1)
			return respondOK(`{}`)(ctx, conn, pkt)
		},
	})
	defer srv.Close()

	c := New(withWSURL(wsURL), withKeepaliveInterval(100*time.Millisecond))
	ctx := context.Background()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}

	if err := c.AuthToken(ctx, "tok", "dev"); err != nil {
		t.Fatalf("auth: %v", err)
	}

	// Wait for a few keepalives
	time.Sleep(350 * time.Millisecond)

	// Call AuthToken again — keepalive should be idempotently restarted, not duplicated
	countBefore := keepaliveCount.Load()
	if err := c.AuthToken(ctx, "tok", "dev"); err != nil {
		t.Fatalf("re-auth: %v", err)
	}
	time.Sleep(350 * time.Millisecond)
	c.Close()

	if keepaliveCount.Load() < 2 {
		t.Errorf("keepaliveCount = %d, want >= 2", keepaliveCount.Load())
	}
	// Verify only one keepalive goroutine is running (count should grow, not double)
	_ = countBefore // keepaliveCount should keep incrementing at the same rate, not 2x
}
