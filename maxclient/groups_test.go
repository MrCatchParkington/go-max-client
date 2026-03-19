package maxclient

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/coder/websocket"
)

func TestCreateGroup(t *testing.T) {
	var payload map[string]any
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeSendMessage: func(ctx context.Context, conn *websocket.Conn, pkt Packet) error {
			json.Unmarshal(pkt.Payload, &payload)
			return respondOK(`{}`)(ctx, conn, pkt)
		},
	})
	defer srv.Close()

	c := New(withWSURL(wsURL))
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()

	_, err := c.CreateGroup(ctx, "Test Group", []int64{111, 222})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}

	msg := payload["message"].(map[string]any)
	attaches := msg["attaches"].([]any)
	att := attaches[0].(map[string]any)
	if att["_type"] != "CONTROL" {
		t.Errorf("type = %v, want CONTROL", att["_type"])
	}
	if att["chatType"] != "CHAT" {
		t.Errorf("chatType = %v, want CHAT", att["chatType"])
	}
	if att["title"] != "Test Group" {
		t.Errorf("title = %v, want Test Group", att["title"])
	}
}

func TestGetGroupMembers(t *testing.T) {
	var payload map[string]any
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeGetMembers: func(ctx context.Context, conn *websocket.Conn, pkt Packet) error {
			json.Unmarshal(pkt.Payload, &payload)
			return respondOK(`{"members":[]}`)(ctx, conn, pkt)
		},
	})
	defer srv.Close()

	c := New(withWSURL(wsURL))
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()

	_, err := c.GetGroupMembers(ctx, 42)
	if err != nil {
		t.Fatalf("get members: %v", err)
	}
	chatID := int64(payload["chatId"].(float64))
	if chatID != 42 {
		t.Errorf("chatId = %d, want 42", chatID)
	}
}

func TestJoinGroup(t *testing.T) {
	srv, wsURL := mockWSServer(map[int]mockHandler{OpcodeJoinChannel: respondOK(`{}`)})
	defer srv.Close()

	c := New(withWSURL(wsURL))
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()

	if err := c.JoinGroup(ctx, 42); err != nil {
		t.Fatalf("join: %v", err)
	}
}

func TestLeaveGroup(t *testing.T) {
	srv, wsURL := mockWSServer(map[int]mockHandler{OpcodeGroupOps: respondOK(`{}`)})
	defer srv.Close()

	c := New(withWSURL(wsURL))
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()

	if err := c.LeaveGroup(ctx, 42); err != nil {
		t.Fatalf("leave: %v", err)
	}
}
