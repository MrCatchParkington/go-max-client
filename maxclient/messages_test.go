package maxclient

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/coder/websocket"
)

func TestSendMessage(t *testing.T) {
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

	_, err := c.SendMessage(ctx, 42, "hello world")
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	chatID := int64(payload["chatId"].(float64))
	if chatID != 42 {
		t.Errorf("chatId = %d, want 42", chatID)
	}

	msg := payload["message"].(map[string]any)
	if msg["text"] != "hello world" {
		t.Errorf("text = %v, want 'hello world'", msg["text"])
	}
	if _, ok := msg["cid"]; !ok {
		t.Error("cid not set")
	}
}

func TestSendMessageWithReply(t *testing.T) {
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

	_, err := c.SendMessage(ctx, 1, "reply text", SendMessageOpts{ReplyTo: "msg-123"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}

	msg := payload["message"].(map[string]any)
	link := msg["link"].(map[string]any)
	if link["type"] != "REPLY" {
		t.Errorf("link.type = %v, want REPLY", link["type"])
	}
	if link["messageId"] != "msg-123" {
		t.Errorf("link.messageId = %v, want msg-123", link["messageId"])
	}
}

func TestDeleteMessage(t *testing.T) {
	var payload map[string]any
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeDeleteMessage: func(ctx context.Context, conn *websocket.Conn, pkt Packet) error {
			json.Unmarshal(pkt.Payload, &payload)
			return respondOK(`{}`)(ctx, conn, pkt)
		},
	})
	defer srv.Close()

	c := New(withWSURL(wsURL))
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()

	err := c.DeleteMessage(ctx, 42, []string{"id1", "id2"}, false)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}

	chatID := int64(payload["chatId"].(float64))
	if chatID != 42 {
		t.Errorf("chatId = %d, want 42", chatID)
	}
	if payload["forMe"] != false {
		t.Errorf("forMe = %v, want false", payload["forMe"])
	}
}

func TestEditMessage(t *testing.T) {
	var payload map[string]any
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeEditMessage: func(ctx context.Context, conn *websocket.Conn, pkt Packet) error {
			json.Unmarshal(pkt.Payload, &payload)
			return respondOK(`{}`)(ctx, conn, pkt)
		},
	})
	defer srv.Close()

	c := New(withWSURL(wsURL))
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()

	err := c.EditMessage(ctx, 42, "msg-1", "new text")
	if err != nil {
		t.Fatalf("edit: %v", err)
	}

	if payload["messageId"] != "msg-1" {
		t.Errorf("messageId = %v, want msg-1", payload["messageId"])
	}
	if payload["text"] != "new text" {
		t.Errorf("text = %v, want 'new text'", payload["text"])
	}
}

func TestEditMessageWithAttaches(t *testing.T) {
	var payload map[string]any
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeEditMessage: func(ctx context.Context, conn *websocket.Conn, pkt Packet) error {
			json.Unmarshal(pkt.Payload, &payload)
			return respondOK(`{}`)(ctx, conn, pkt)
		},
	})
	defer srv.Close()

	c := New(withWSURL(wsURL))
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()

	att := Attachment{Type: "PHOTO", PhotoToken: "tok-1"}
	err := c.EditMessage(ctx, 42, "msg-1", "new text", att)
	if err != nil {
		t.Fatalf("edit: %v", err)
	}

	attaches, ok := payload["attachments"]
	if !ok {
		t.Fatal("attachments not set in payload")
	}
	arr := attaches.([]any)
	if len(arr) != 1 {
		t.Errorf("len(attachments) = %d, want 1", len(arr))
	}
}

func TestPinMessage(t *testing.T) {
	var payload map[string]any
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodePinMessage: func(ctx context.Context, conn *websocket.Conn, pkt Packet) error {
			json.Unmarshal(pkt.Payload, &payload)
			return respondOK(`{}`)(ctx, conn, pkt)
		},
	})
	defer srv.Close()

	c := New(withWSURL(wsURL))
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()

	err := c.PinMessage(ctx, 42, "msg-1", true)
	if err != nil {
		t.Fatalf("pin: %v", err)
	}

	if payload["notifyPin"] != true {
		t.Errorf("notifyPin = %v, want true", payload["notifyPin"])
	}
}

func TestGetHistory(t *testing.T) {
	var payload map[string]any
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeGetHistory: func(ctx context.Context, conn *websocket.Conn, pkt Packet) error {
			json.Unmarshal(pkt.Payload, &payload)
			return respondOK(`{}`)(ctx, conn, pkt)
		},
	})
	defer srv.Close()

	c := New(withWSURL(wsURL))
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()

	_, err := c.GetHistory(ctx, 42, 30)
	if err != nil {
		t.Fatalf("history: %v", err)
	}

	chatID := int64(payload["chatId"].(float64))
	if chatID != 42 {
		t.Errorf("chatId = %d, want 42", chatID)
	}
	forward := int(payload["forward"].(float64))
	if forward != 30 {
		t.Errorf("forward = %d, want 30", forward)
	}
}
