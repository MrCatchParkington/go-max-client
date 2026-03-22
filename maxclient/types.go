package maxclient

import (
	"encoding/json"
	"fmt"
)

// Packet is the wire-level MAX protocol message.
type Packet struct {
	Ver     int             `json:"ver"`
	Cmd     int             `json:"cmd"`
	Seq     int             `json:"seq"`
	Opcode  int             `json:"opcode"`
	Payload json.RawMessage `json:"payload"`
}

// Message represents a chat message.
type Message struct {
	ID     string `json:"id"`
	ChatID int64  `json:"chatId,omitempty"`
	Text   string `json:"text"`
	Status string `json:"status,omitempty"`
	Sender int64  `json:"sender,omitempty"`
}

// MessageEvent is the payload of opcode 128 packets.
type MessageEvent struct {
	ChatID  int64    `json:"chatId"`
	Message *Message `json:"message"`
}

// ParseMessageEvent extracts a MessageEvent from a Packet with opcode 128.
func ParseMessageEvent(p *Packet) (*MessageEvent, error) {
	if p.Opcode != OpcodeMessageEvent {
		return nil, fmt.Errorf("expected opcode %d, got %d", OpcodeMessageEvent, p.Opcode)
	}
	var evt MessageEvent
	if err := json.Unmarshal(p.Payload, &evt); err != nil {
		return nil, fmt.Errorf("unmarshal message event: %w", err)
	}
	return &evt, nil
}

// QRAuth contains the result of a successful QR authentication.
type QRAuth struct {
	Token    string
	DeviceID string
	ChatID   int64
}

// SendMessageOpts contains optional parameters for SendMessage.
type SendMessageOpts struct {
	ReplyTo  string       // empty = no reply
	Attaches []Attachment
}

// Attachment represents a media attachment in a message.
// VideoID and FileID use json.Number so they serialize as numbers (server requirement).
type Attachment struct {
	Type       string      `json:"_type"`
	PhotoToken string      `json:"photoToken,omitempty"`
	VideoID    json.Number `json:"videoId,omitempty"`
	FileID     json.Number `json:"fileId,omitempty"`
	Token      string      `json:"token,omitempty"`
}

// User represents a MAX user.
type User struct {
	ID         int64  `json:"id"`
	ExternalID int64  `json:"externalId,omitempty"`
	FirstName  string `json:"firstName,omitempty"`
	LastName   string `json:"lastName,omitempty"`
	Phone      string `json:"phone,omitempty"`
}

// Group represents a MAX group chat.
type Group struct {
	ID    int64  `json:"id"`
	Title string `json:"title,omitempty"`
}

// Channel represents a MAX channel.
type Channel struct {
	ID    int64  `json:"id"`
	Title string `json:"title,omitempty"`
}

// ProfileOpts contains optional parameters for SetProfile.
type ProfileOpts struct {
	FirstName *string
	LastName  *string
	Bio       *string
}

// SettingsOpts contains optional parameters for SetSettings.
type SettingsOpts struct {
	Settings map[string]any
}
