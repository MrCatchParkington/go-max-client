package maxclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"

	"github.com/coder/websocket"
)

// mockHandler is an opcode→handler function map.
type mockHandler func(ctx context.Context, conn *websocket.Conn, pkt Packet) error

// mockWSServer creates a test WebSocket server with configurable opcode handlers.
// Returns the server and its ws:// URL.
func mockWSServer(handlers map[int]mockHandler) (*httptest.Server, string) {
	var mu sync.Mutex

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true,
		})
		if err != nil {
			return
		}
		defer conn.CloseNow()

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

			mu.Lock()
			h, ok := handlers[pkt.Opcode]
			mu.Unlock()

			if ok {
				if err := h(ctx, conn, pkt); err != nil {
					return
				}
			} else {
				// Default: echo back with same seq
				resp := Packet{
					Ver:     RPCVersion,
					Cmd:     0,
					Seq:     pkt.Seq,
					Opcode:  pkt.Opcode,
					Payload: json.RawMessage(`{}`),
				}
				respData, _ := json.Marshal(resp)
				if err := conn.Write(ctx, websocket.MessageText, respData); err != nil {
					return
				}
			}
		}
	}))

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	return srv, wsURL
}

// respondOK creates a handler that responds with the given payload JSON.
func respondOK(payloadJSON string) mockHandler {
	return func(ctx context.Context, conn *websocket.Conn, pkt Packet) error {
		resp := Packet{
			Ver:     RPCVersion,
			Cmd:     0,
			Seq:     pkt.Seq,
			Opcode:  pkt.Opcode,
			Payload: json.RawMessage(payloadJSON),
		}
		data, _ := json.Marshal(resp)
		return conn.Write(ctx, websocket.MessageText, data)
	}
}
