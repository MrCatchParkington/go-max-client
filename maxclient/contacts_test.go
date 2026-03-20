package maxclient

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/coder/websocket"
)

const addContactResponse = `{
	"new": true,
	"contact": {
		"accountStatus": 0,
		"names": [
			{"name": "Test", "type": "CUSTOM", "firstName": "Test"},
			{"name": "Alice", "firstName": "Alice", "lastName": "Smith", "type": "ONEME"}
		],
		"phone": 79991234567,
		"options": ["TT", "ONEME"],
		"updateTime": 1774012846713,
		"id": 90001
	}
}`

func TestAddContactByPhone(t *testing.T) {
	var payload map[string]any
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeAddContactByPhone: func(ctx context.Context, conn *websocket.Conn, pkt Packet) error {
			json.Unmarshal(pkt.Payload, &payload)
			return respondOK(addContactResponse)(ctx, conn, pkt)
		},
	})
	defer srv.Close()
	c := New(withWSURL(wsURL))
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()

	user, err := c.AddContactByPhone(ctx, "+79991234567", "Test")
	if err != nil {
		t.Fatalf("AddContactByPhone: %v", err)
	}

	// Verify request payload
	if payload["phone"] != "+79991234567" {
		t.Errorf("phone = %v, want +79991234567", payload["phone"])
	}
	if payload["firstName"] != "Test" {
		t.Errorf("firstName = %v, want Test", payload["firstName"])
	}

	// Verify parsed user
	if user.ID != 90001 {
		t.Errorf("ID = %d, want 90001", user.ID)
	}
	if user.Phone != "+79991234567" {
		t.Errorf("Phone = %q, want +79991234567", user.Phone)
	}
	if user.FirstName != "Alice" {
		t.Errorf("FirstName = %q, want Alice (ONEME name)", user.FirstName)
	}
	if user.LastName != "Smith" {
		t.Errorf("LastName = %q, want Smith (ONEME name)", user.LastName)
	}
}

func TestFindUserByPhone(t *testing.T) {
	const resolveResponse = `{"chats":[{"id":90001,"participants":{"55501":1,"100002":1}}]}`

	var payload map[string]any
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeAddContactByPhone: func(ctx context.Context, conn *websocket.Conn, pkt Packet) error {
			json.Unmarshal(pkt.Payload, &payload)
			return respondOK(addContactResponse)(ctx, conn, pkt)
		},
		OpcodeResolveChannel: respondOK(resolveResponse),
	})
	defer srv.Close()
	c := New(withWSURL(wsURL))
	c.ownUserID = 100002
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()

	user, err := c.FindUserByPhone(ctx, "+79991234567")
	if err != nil {
		t.Fatalf("FindUserByPhone: %v", err)
	}

	// Verify placeholder firstName
	if payload["firstName"] != "_" {
		t.Errorf("firstName = %v, want _ (placeholder)", payload["firstName"])
	}

	// user.ID should be the real userId (55501), not the contact record ID (90001)
	if user.ID != 55501 {
		t.Errorf("ID = %d, want 55501 (real userId from participants)", user.ID)
	}
}

func TestAddContactByPhoneNoONEMEName(t *testing.T) {
	resp := `{
		"new": true,
		"contact": {
			"names": [{"name": "Custom", "firstName": "Custom", "lastName": "Name", "type": "CUSTOM"}],
			"phone": 79991234567,
			"id": 100
		}
	}`
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeAddContactByPhone: respondOK(resp),
	})
	defer srv.Close()
	c := New(withWSURL(wsURL))
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()

	user, err := c.AddContactByPhone(ctx, "+79991234567", "Custom")
	if err != nil {
		t.Fatalf("AddContactByPhone: %v", err)
	}
	if user.FirstName != "Custom" {
		t.Errorf("FirstName = %q, want Custom (fallback to first entry)", user.FirstName)
	}
}

func TestAddContactByPhoneServerError(t *testing.T) {
	srv, wsURL := mockWSServer(map[int]mockHandler{
		OpcodeAddContactByPhone: respondOK(`{"error":"user not found"}`),
	})
	defer srv.Close()
	c := New(withWSURL(wsURL))
	ctx := context.Background()
	c.Connect(ctx)
	defer c.Close()

	user, err := c.AddContactByPhone(ctx, "+70000000000", "X")
	// Server returns error payload without "contact" object — parsed user has zero ID.
	if err != nil {
		return // transport error is also acceptable
	}
	if user.ID != 0 {
		t.Errorf("ID = %d, want 0 for error response", user.ID)
	}
}
