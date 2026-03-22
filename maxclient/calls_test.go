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
