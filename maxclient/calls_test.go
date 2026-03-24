package maxclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	gorillaWs "github.com/gorilla/websocket"
	"github.com/pierrec/lz4/v4"
)

func TestDecodeVCP(t *testing.T) {
	original := `{"tkn":"test-token","wse":"wss://sig.example.com","stne":"stun:stun.example.com","trne":"turn:turn1.example.com,turn:turn2.example.com","trnu":"user","trnp":"pass"}`
	src := []byte(original)

	dst := make([]byte, lz4.CompressBlockBound(len(src)))
	n, err := lz4.CompressBlock(src, dst, nil)
	if err != nil {
		t.Fatal(err)
	}
	compressed := dst[:n]

	vcp := fmt.Sprintf("%d:%s", len(src), base64.StdEncoding.EncodeToString(compressed))

	decoded, err := decodeVCP(vcp)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.SignalingToken != "test-token" {
		t.Errorf("token = %q, want %q", decoded.SignalingToken, "test-token")
	}
	if decoded.TurnUser != "user" {
		t.Errorf("turnUser = %q, want %q", decoded.TurnUser, "user")
	}
	if decoded.TurnServers != "turn:turn1.example.com,turn:turn2.example.com" {
		t.Errorf("turnServers = %q", decoded.TurnServers)
	}
}

func TestCallsAPI_Login(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		r.ParseForm()
		if r.Form.Get("method") != "auth.anonymLogin" {
			t.Errorf("method param = %q", r.Form.Get("method"))
		}
		if r.Form.Get("application_key") != CallsAppKey {
			t.Errorf("app key = %q", r.Form.Get("application_key"))
		}
		json.NewEncoder(w).Encode(callsLoginResponse{
			UID:            "123",
			SessionKey:     "session-key",
			ExternalUserID: "ext-456",
		})
	}))
	defer srv.Close()

	api := &callsAPI{baseURL: srv.URL, httpClient: http.DefaultClient}
	resp, err := api.login(context.Background(), "test-token")
	if err != nil {
		t.Fatal(err)
	}
	if resp.UID != "123" || resp.SessionKey != "session-key" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestFastStartCall(t *testing.T) {
	// Build a mock response: fastStartResponse with nested JSON in internalCallerParams
	innerJSON := `{"turn":{"urls":["turn:turn.example.com"],"username":"u","credential":"p"},"stun":{"urls":["stun:stun.example.com"]},"endpoint":"wss://sig.example.com/ws?token=abc"}`
	outerJSON, _ := json.Marshal(fastStartResponse{InternalCallerParams: innerJSON})

	var gotPayload map[string]any
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeFastStartCall: func(ctx context.Context, conn *websocket.Conn, pkt Packet) error {
			json.Unmarshal(pkt.Payload, &gotPayload)
			return respondOK(string(outerJSON))(ctx, conn, pkt)
		},
	})
	defer srv.Close()
	c := New(withWSURL(wsURL))
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()

	resp, err := c.fastStartCall(ctx, 87654321)
	if err != nil {
		t.Fatalf("fastStartCall: %v", err)
	}

	// Verify request payload
	calleeIDs, _ := gotPayload["calleeIds"].([]any)
	if len(calleeIDs) != 1 || int64(calleeIDs[0].(float64)) != 87654321 {
		t.Errorf("calleeIds = %v, want [87654321]", calleeIDs)
	}
	if gotPayload["conversationId"] == "" {
		t.Error("conversationId is empty")
	}
	if gotPayload["internalParams"] == nil {
		t.Error("internalParams is nil")
	}

	// Verify parsed response (double unmarshal)
	if resp.Endpoint != "wss://sig.example.com/ws?token=abc" {
		t.Errorf("Endpoint = %q, want wss://sig.example.com/ws?token=abc", resp.Endpoint)
	}
	if len(resp.TurnServer.Urls) != 1 || resp.TurnServer.Urls[0] != "turn:turn.example.com" {
		t.Errorf("TurnServer.Urls = %v", resp.TurnServer.Urls)
	}
	if resp.TurnServer.Username != "u" {
		t.Errorf("TurnServer.Username = %q, want u", resp.TurnServer.Username)
	}
	if len(resp.StunServer.Urls) != 1 || resp.StunServer.Urls[0] != "stun:stun.example.com" {
		t.Errorf("StunServer.Urls = %v", resp.StunServer.Urls)
	}
}

func TestFastStartCallEmptyInternalParams(t *testing.T) {
	outerJSON, _ := json.Marshal(fastStartResponse{InternalCallerParams: ""})
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeFastStartCall: respondOK(string(outerJSON)),
	})
	defer srv.Close()
	c := New(withWSURL(wsURL))
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()

	_, err := c.fastStartCall(ctx, 12345678)
	if err == nil {
		t.Fatal("expected error for empty internalCallerParams")
	}
}

func TestFastStartCallEmptyEndpoint(t *testing.T) {
	innerJSON := `{"turn":{},"stun":{},"endpoint":""}`
	outerJSON, _ := json.Marshal(fastStartResponse{InternalCallerParams: innerJSON})
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeFastStartCall: respondOK(string(outerJSON)),
	})
	defer srv.Close()
	c := New(withWSURL(wsURL))
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()

	_, err := c.fastStartCall(ctx, 12345678)
	if err == nil {
		t.Fatal("expected error for empty endpoint")
	}
}

func TestSignalingClient_ExchangeData(t *testing.T) {
	upgrader := gorillaWs.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer ws.Close()

		hello := signalingServerHello{}
		hello.Conversation.Participants = []signalingParticipant{
			{ID: 42, ExternalID: struct{ ID string `json:"id"` }{ID: "ext-42"}},
		}
		ws.WriteJSON(hello)

		var msg signalingTransmitData
		ws.ReadJSON(&msg)

		ws.WriteJSON(map[string]any{
			"type":         "notification",
			"notification": "transmitted-data",
			"data":         msg.Data,
		})
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	sc, err := newSignalingClient(context.Background(), wsURL, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer sc.close()

	hello, err := sc.receiveServerHello()
	if err != nil {
		t.Fatal(err)
	}
	if len(hello.Conversation.Participants) != 1 || hello.Conversation.Participants[0].ID != 42 {
		t.Errorf("unexpected hello: %+v", hello)
	}

	err = sc.sendData(42, iceCredentials{UFrag: "test", Password: "pass"})
	if err != nil {
		t.Fatal(err)
	}

	var creds iceCredentials
	err = sc.receiveData(&creds)
	if err != nil {
		t.Fatal(err)
	}
	if creds.UFrag != "test" {
		t.Errorf("ufrag = %q, want %q", creds.UFrag, "test")
	}
}

func TestSignalingClient_ReceiveDataReturnsOnContextCancel(t *testing.T) {
	upgrader := gorillaWs.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer ws.Close()

		hello := signalingServerHello{}
		hello.Conversation.Participants = []signalingParticipant{
			{ID: 42, ExternalID: struct{ ID string `json:"id"` }{ID: "ext-42"}},
		}
		if err := ws.WriteJSON(hello); err != nil {
			return
		}
		select {}
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	sc, err := newSignalingClient(ctx, wsURL, slog.Default())
	if err != nil {
		t.Fatal(err)
	}
	defer sc.close()

	if _, err := sc.receiveServerHello(); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		var creds iceCredentials
		done <- sc.receiveData(&creds)
	}()

	cancel()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected receiveData to fail after context cancellation")
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("receiveData did not return after context cancellation")
	}
}
