package maxclient

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/coder/websocket"
)

func TestResolveChannel(t *testing.T) {
	var payload map[string]any
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeResolveChannel: func(ctx context.Context, conn *websocket.Conn, pkt Packet) error {
			json.Unmarshal(pkt.Payload, &payload)
			return respondOK(`{}`)(ctx, conn, pkt)
		},
	})
	defer srv.Close()
	c := New(withWSURL(wsURL))
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()
	_, err := c.ResolveChannel(ctx, 42)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	chatIDs := payload["chatIds"].([]any)
	if int64(chatIDs[0].(float64)) != 42 {
		t.Errorf("chatIds[0] = %v, want 42", chatIDs[0])
	}
}

func TestResolveByLink(t *testing.T) {
	var payload map[string]any
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeResolveByLink: func(ctx context.Context, conn *websocket.Conn, pkt Packet) error {
			json.Unmarshal(pkt.Payload, &payload)
			return respondOK(`{}`)(ctx, conn, pkt)
		},
	})
	defer srv.Close()
	c := New(withWSURL(wsURL))
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()
	_, err := c.ResolveByLink(ctx, "https://max.ru/mychannel")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if payload["link"] != "https://max.ru/mychannel" {
		t.Errorf("link = %v, want https://max.ru/mychannel", payload["link"])
	}
}

func TestResolveUsers(t *testing.T) {
	var payload map[string]any
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeResolveUsers: func(ctx context.Context, conn *websocket.Conn, pkt Packet) error {
			json.Unmarshal(pkt.Payload, &payload)
			return respondOK(`{}`)(ctx, conn, pkt)
		},
	})
	defer srv.Close()
	c := New(withWSURL(wsURL))
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()
	_, err := c.ResolveUsers(ctx, []int64{111, 222})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	ids := payload["contactIds"].([]any)
	if len(ids) != 2 {
		t.Errorf("len = %d, want 2", len(ids))
	}
}

func TestSetProfile(t *testing.T) {
	var payload map[string]any
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeChangeProfile: func(ctx context.Context, conn *websocket.Conn, pkt Packet) error {
			json.Unmarshal(pkt.Payload, &payload)
			return respondOK(`{}`)(ctx, conn, pkt)
		},
	})
	defer srv.Close()
	c := New(withWSURL(wsURL))
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()
	first := "John"
	err := c.SetProfile(ctx, ProfileOpts{FirstName: &first})
	if err != nil {
		t.Fatalf("set profile: %v", err)
	}
	if payload["firstName"] != "John" {
		t.Errorf("firstName = %v, want John", payload["firstName"])
	}
}

func TestSetSettings(t *testing.T) {
	srv, wsURL := mockWSServer(map[int]mockHandler{OpcodeSetSettings: respondOK(`{}`)})
	defer srv.Close()
	c := New(withWSURL(wsURL))
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()
	err := c.SetSettings(ctx, SettingsOpts{Settings: map[string]any{"user": map[string]any{"HIDDEN": true}}})
	if err != nil {
		t.Fatalf("set settings: %v", err)
	}
}
