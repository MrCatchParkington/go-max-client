package maxclient

import (
	"encoding/json"
	"testing"
)

func TestPacketJSON(t *testing.T) {
	raw := `{"ver":11,"cmd":0,"seq":1,"opcode":128,"payload":{"chatId":0,"message":{"id":"abc","text":"hello"}}}`
	var pkt Packet
	if err := json.Unmarshal([]byte(raw), &pkt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if pkt.Ver != 11 {
		t.Errorf("ver = %d, want 11", pkt.Ver)
	}
	if pkt.Opcode != OpcodeMessageEvent {
		t.Errorf("opcode = %d, want %d", pkt.Opcode, OpcodeMessageEvent)
	}
	if pkt.Seq != 1 {
		t.Errorf("seq = %d, want 1", pkt.Seq)
	}

	// Re-marshal should produce valid JSON
	out, err := json.Marshal(pkt)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("marshal produced empty output")
	}
}

func TestParseMessageEvent(t *testing.T) {
	raw := `{"chatId":42,"message":{"id":"msg-1","text":"hello","status":"NORMAL"}}`
	pkt := Packet{Opcode: OpcodeMessageEvent, Payload: json.RawMessage(raw)}

	evt, err := ParseMessageEvent(&pkt)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.ChatID != 42 {
		t.Errorf("chatId = %d, want 42", evt.ChatID)
	}
	if evt.Message.ID != "msg-1" {
		t.Errorf("message.id = %q, want %q", evt.Message.ID, "msg-1")
	}
	if evt.Message.Text != "hello" {
		t.Errorf("message.text = %q, want %q", evt.Message.Text, "hello")
	}
	if evt.Message.Status != "NORMAL" {
		t.Errorf("message.status = %q, want %q", evt.Message.Status, "NORMAL")
	}
}

func TestParseMessageEventWrongOpcode(t *testing.T) {
	pkt := Packet{Opcode: OpcodeKeepalive, Payload: json.RawMessage(`{}`)}
	_, err := ParseMessageEvent(&pkt)
	if err == nil {
		t.Fatal("expected error for wrong opcode")
	}
}

func TestParseMessageEventSender(t *testing.T) {
	raw := `{"chatId":90001,"message":{"sender":90002,"id":"msg-1","text":"hello","type":"USER"}}`
	pkt := Packet{Opcode: OpcodeMessageEvent, Payload: json.RawMessage(raw)}

	evt, err := ParseMessageEvent(&pkt)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if evt.Message.Sender != 90002 {
		t.Errorf("Sender = %d, want 90002", evt.Message.Sender)
	}
	if evt.ChatID != 90001 {
		t.Errorf("ChatID = %d, want 90001", evt.ChatID)
	}
}
