package maxclient

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
)

const (
	WSURL      = "wss://ws-api.oneme.ru/websocket"
	RPCVersion = 11
	AppVersion = "26.2.2"

	DefaultUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/141.0.0.0 Safari/537.36"
	DefaultOrigin    = "https://web.max.ru"
)

// Opcodes for the MAX protocol.
const (
	OpcodeKeepalive        = 1
	OpcodeHello            = 6
	OpcodeChangeProfile    = 16
	OpcodeSendCode         = 17
	OpcodeSignIn           = 18
	OpcodeLoginByToken     = 19
	OpcodeSetSettings      = 22
	OpcodeResolveUsers     = 32
	OpcodeAddContact       = 34
	OpcodeResolveChannel   = 48
	OpcodeGetHistory       = 49
	OpcodePinMessage       = 55
	OpcodeJoinChannel      = 57
	OpcodeGetMembers       = 59
	OpcodeSendMessage      = 64
	OpcodePrepareAttach    = 65
	OpcodeDeleteMessage    = 66
	OpcodeEditMessage      = 67
	OpcodeSubscribeChat    = 75
	OpcodeGroupOps         = 77
	OpcodePhotoUploadURL   = 80
	OpcodeVideoUploadURL   = 82
	OpcodeVideoDownloadURL = 83
	OpcodeFileUploadURL    = 87
	OpcodeFileDownloadURL  = 88
	OpcodeResolveByLink    = 89
	OpcodeMessageEvent     = 128
	OpcodeUploadComplete   = 136
	OpcodeReactMessage     = 178
	OpcodeGetReactions     = 181
	OpcodeGetQR            = 288
	OpcodeGetQRStatus      = 289
	OpcodeLoginByQR        = 291
)

// seqCounter is a goroutine-safe sequence number generator.
type seqCounter struct {
	val atomic.Int64
}

func (s *seqCounter) Next() int {
	return int(s.val.Add(1))
}

// buildRequest creates a JSON-encoded request packet and returns it with the assigned seq number.
func buildRequest(seq *seqCounter, opcode int, payload any) ([]byte, int, error) {
	s := seq.Next()
	req := struct {
		Ver     int `json:"ver"`
		Cmd     int `json:"cmd"`
		Seq     int `json:"seq"`
		Opcode  int `json:"opcode"`
		Payload any `json:"payload"`
	}{
		Ver:     RPCVersion,
		Cmd:     0,
		Seq:     s,
		Opcode:  opcode,
		Payload: payload,
	}
	data, err := json.Marshal(req)
	if err != nil {
		return nil, s, fmt.Errorf("marshal request: %w", err)
	}
	return data, s, nil
}

// unmarshalStringOrNumber extracts a string from JSON that may be a string or number.
func unmarshalStringOrNumber(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var n json.Number
	if json.Unmarshal(raw, &n) == nil {
		return n.String()
	}
	return ""
}
