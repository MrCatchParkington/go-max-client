# Eliminate CALL_DISCOVER Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the CALL_DISCOVER hack by making `FindUserByPhone` return both chatID and externalId, and replacing the 3-step HTTP call flow with a single WebSocket FastStart (opcode 78).

**Architecture:** Two independent changes: (1) `FindUserByPhone` preserves externalId from AddContactByPhone instead of discarding it, (2) `Call()` uses opcode 78 (FastStart) instead of HTTP calls to calls.okcdn.ru. The signaling and ICE layers are untouched.

**Tech Stack:** Go, WebSocket (coder/websocket), ICE (pion/ice)

**Spec:** `docs/superpowers/specs/2026-03-22-eliminate-call-discover-design.md`

---

## File Map

| File | Action | Responsibility |
|---|---|---|
| `maxclient/types.go` | Modify | Add `ExternalID` field to `User` struct |
| `maxclient/contacts.go` | Modify | Remove public `AddContactByPhone()`, update `FindUserByPhone` to save both IDs |
| `maxclient/contacts_test.go` | Modify | Fix tests for private `addContactByPhone`, assert `ExternalID` in `FindUserByPhone` |
| `maxclient/protocol.go` | Modify | Add `OpcodeFastStartCall = 78` |
| `maxclient/calls_types.go` | Modify | Add FastStart request/response types |
| `maxclient/calls_oneme.go` | Modify | Add `fastStartCall()` method |
| `maxclient/calls.go` | Modify | Rewrite `Call()` to use FastStart, remove `GetCallsExternalUserID()` |
| `maxclient/calls_api.go` | Modify | Remove dead `startConversation()` method |
| `maxclient/calls_test.go` | Modify | Remove `TestCallsAPI_StartConversation` |

---

### Task 1: Add ExternalID to User and update FindUserByPhone

**Files:**
- Modify: `maxclient/types.go:68-73`
- Modify: `maxclient/contacts.go:10-54`
- Modify: `maxclient/contacts_test.go`

- [ ] **Step 1: Update TestFindUserByPhone to assert ExternalID**

In `maxclient/contacts_test.go`, the existing `TestFindUserByPhone` (line 68) asserts only `user.ID`. Add assertion for `user.ExternalID`:

```go
// In TestFindUserByPhone, after the existing user.ID assertion (line 97):
// user.ID should be the real userId (55501), not the contact record ID (90001)
if user.ID != 55501 {
    t.Errorf("ID = %d, want 55501 (chatID from participants)", user.ID)
}
// ExternalID should be the contact.id from AddContactByPhone (90001)
if user.ExternalID != 90001 {
    t.Errorf("ExternalID = %d, want 90001 (externalId from contact)", user.ExternalID)
}
```

Also update the comment on line 96 from "real userId" to "chatID" (the correct term per spec).

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/parkington/git/go-max-client && go test ./maxclient/ -run TestFindUserByPhone -v`
Expected: compilation error — `user.ExternalID undefined`

- [ ] **Step 3: Add ExternalID to User struct**

In `maxclient/types.go`, change the `User` struct (line 68-73):

```go
// User represents a MAX user.
type User struct {
	ID         int64  `json:"id"`
	ExternalID int64  `json:"externalId,omitempty"`
	FirstName  string `json:"firstName,omitempty"`
	LastName   string `json:"lastName,omitempty"`
	Phone      string `json:"phone,omitempty"`
}
```

- [ ] **Step 4: Update FindUserByPhone to preserve ExternalID**

In `maxclient/contacts.go`, replace `FindUserByPhone` (lines 21-54):

```go
// FindUserByPhone looks up a user by phone number.
// Calls addContactByPhone to get the user's externalId (contact.id),
// then resolves the chatID from chat participants via ResolveChannel.
func (c *Client) FindUserByPhone(ctx context.Context, phone string) (*User, error) {
	user, err := c.addContactByPhone(ctx, phone, "_")
	if err != nil {
		return nil, err
	}
	// contact.id from AddContactByPhone is the user's externalId in OneMe.
	user.ExternalID = user.ID

	// Resolve the chatID from chat participants.
	resp, err := c.ResolveChannel(ctx, user.ExternalID)
	if err != nil {
		return nil, fmt.Errorf("resolve contact chat: %w", err)
	}

	var resolved struct {
		Chats []struct {
			Participants map[string]any `json:"participants"`
		} `json:"chats"`
	}
	if err := json.Unmarshal(resp.Payload, &resolved); err == nil {
		ownID := strconv.FormatInt(c.ownUserID, 10)
		for _, chat := range resolved.Chats {
			for pidStr := range chat.Participants {
				if pidStr != ownID {
					if chatID, err := strconv.ParseInt(pidStr, 10, 64); err == nil {
						user.ID = chatID
						return user, nil
					}
				}
			}
		}
	}

	return user, nil
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /home/parkington/git/go-max-client && go test ./maxclient/ -run TestFindUserByPhone -v`
Expected: PASS

- [ ] **Step 6: Remove public AddContactByPhone and update its tests**

In `maxclient/contacts.go`, delete the public `AddContactByPhone` method (lines 10-16):

```go
// DELETE THIS ENTIRE BLOCK:
// AddContactByPhone adds a user to contacts by phone number and returns their info.
// The phone should be in E.164 format (e.g. "+79991234567").
// The firstName is stored as the contact's custom display name.
// The returned User.ID can be used directly as a chatID for SendMessage (DM chatID == userID).
func (c *Client) AddContactByPhone(ctx context.Context, phone, firstName string) (*User, error) {
	return c.addContactByPhone(ctx, phone, firstName)
}
```

In `maxclient/contacts_test.go`, update the three tests that call the public method to use the private one instead:

- `TestAddContactByPhone` (line 40): `c.AddContactByPhone(...)` → `c.addContactByPhone(...)`
- `TestAddContactByPhoneNoONEMEName` (line 120): `c.AddContactByPhone(...)` → `c.addContactByPhone(...)`
- `TestAddContactByPhoneServerError` (line 139): `c.AddContactByPhone(...)` → `c.addContactByPhone(...)`

This works because the test file is in `package maxclient` (same package, has access to unexported methods).

- [ ] **Step 7: Run all tests to verify nothing is broken**

Run: `cd /home/parkington/git/go-max-client && go test ./maxclient/ -v`
Expected: all tests PASS

- [ ] **Step 8: Commit**

```bash
git add maxclient/types.go maxclient/contacts.go maxclient/contacts_test.go
git commit -m "feat: FindUserByPhone returns ExternalID, make AddContactByPhone private"
```

---

### Task 2: Add FastStart types and opcode constant

**Files:**
- Modify: `maxclient/protocol.go:19-56`
- Modify: `maxclient/calls_types.go`

- [ ] **Step 1: Add OpcodeFastStartCall to protocol.go**

In `maxclient/protocol.go`, add `OpcodeFastStartCall` in the const block, between `OpcodeSubscribeChat` (line 39) and `OpcodeGroupOps` (line 40):

```go
OpcodeSubscribeChat    = 75
OpcodeGroupOps         = 77
OpcodeFastStartCall    = 78
OpcodePhotoUploadURL   = 80
```

- [ ] **Step 2: Add FastStart types to calls_types.go**

In `maxclient/calls_types.go`, add the following types at the end of the `// --- OneMe types ---` section (after `vcpDecoded`, before `// --- Calls HTTP API types ---`):

```go
type fastStartRequest struct {
	ConversationID string  `json:"conversationId"`
	CalleeIDs      []int64 `json:"calleeIds"`
	InternalParams string  `json:"internalParams"`
	IsVideo        bool    `json:"isVideo"`
}

type fastStartInternalParams struct {
	DeviceID        string `json:"deviceId"`
	SDKVersion      string `json:"sdkVersion"`
	ClientAppKey    string `json:"clientAppKey"`
	Platform        string `json:"platform"`
	ProtocolVersion int    `json:"protocolVersion"`
	DomainID        string `json:"domainId"`
	Capabilities    string `json:"capabilities"`
}

type fastStartResponse struct {
	InternalCallerParams string `json:"internalCallerParams"`
}
```

- [ ] **Step 3: Verify compilation**

Run: `cd /home/parkington/git/go-max-client && go build ./maxclient/`
Expected: success (no errors)

- [ ] **Step 4: Commit**

```bash
git add maxclient/protocol.go maxclient/calls_types.go
git commit -m "feat: add FastStart (opcode 78) types and constant"
```

---

### Task 3: Implement fastStartCall()

**Files:**
- Modify: `maxclient/calls_oneme.go`

- [ ] **Step 1: Add fastStartCall() to calls_oneme.go**

Add the method after the existing `getCallToken()` function (after line 23):

```go
// fastStartCall initiates a call via opcode 78 (FastStart).
// Returns TURN/STUN servers and signaling endpoint — same shape as startConversation HTTP response.
func (c *Client) fastStartCall(ctx context.Context, calleeExternalID int64) (*startConversationResponse, error) {
	convID, err := newUUID()
	if err != nil {
		return nil, fmt.Errorf("calls: generate conversation ID: %w", err)
	}
	deviceID, err := newUUID()
	if err != nil {
		return nil, fmt.Errorf("calls: generate device ID: %w", err)
	}

	internalParams, err := json.Marshal(fastStartInternalParams{
		DeviceID:        deviceID,
		SDKVersion:      CallsClientVersion,
		ClientAppKey:    CallsAppKey,
		Platform:        CallsPlatform,
		ProtocolVersion: 5,
		DomainID:        "",
		Capabilities:    CallsCapabilities,
	})
	if err != nil {
		return nil, fmt.Errorf("calls: marshal internal params: %w", err)
	}

	resp, err := c.InvokeMethod(ctx, OpcodeFastStartCall, fastStartRequest{
		ConversationID: convID,
		CalleeIDs:      []int64{calleeExternalID},
		InternalParams: string(internalParams),
		IsVideo:        false,
	})
	if err != nil {
		return nil, fmt.Errorf("calls: fast start: %w", err)
	}
	if err := checkResponseError(resp); err != nil {
		return nil, fmt.Errorf("calls: fast start: %w", err)
	}

	var fsResp fastStartResponse
	if err := json.Unmarshal(resp.Payload, &fsResp); err != nil {
		return nil, fmt.Errorf("calls: parse fast start response: %w", err)
	}
	if fsResp.InternalCallerParams == "" {
		return nil, fmt.Errorf("calls: fast start: empty internalCallerParams")
	}

	var startResp startConversationResponse
	if err := json.Unmarshal([]byte(fsResp.InternalCallerParams), &startResp); err != nil {
		return nil, fmt.Errorf("calls: parse internal caller params: %w", err)
	}
	if startResp.Endpoint == "" {
		return nil, fmt.Errorf("calls: fast start: empty endpoint in internal caller params")
	}

	return &startResp, nil
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /home/parkington/git/go-max-client && go build ./maxclient/`
Expected: success

- [ ] **Step 3: Commit**

```bash
git add maxclient/calls_oneme.go
git commit -m "feat: add fastStartCall() for opcode 78"
```

---

### Task 4: Rewrite Call() to use FastStart and clean up dead code

**Files:**
- Modify: `maxclient/calls.go:10-90`
- Modify: `maxclient/calls_api.go:84-106`
- Modify: `maxclient/calls_test.go:76-99`

- [ ] **Step 1: Rewrite Call() to use FastStart**

In `maxclient/calls.go`, replace the entire `Call()` method (lines 10-90):

```go
// Call initiates a call to the given user and returns a CallSession
// with a bidirectional data stream over ICE.
// calleeExternalID is the callee's externalId in OneMe
// (obtain via FindUserByPhone — it's the User.ExternalID field).
// forceRelay=true restricts to TURN-only candidates (no P2P).
func (c *Client) Call(ctx context.Context, calleeExternalID int64, forceRelay bool) (*CallSession, error) {
	startResp, err := c.fastStartCall(ctx, calleeExternalID)
	if err != nil {
		return nil, err
	}
	c.log.Info("fast start call", "endpoint", startResp.Endpoint)

	sigParsed, err := url.Parse(startResp.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("calls: parse signaling endpoint: %w", err)
	}
	q := sigParsed.Query()
	q.Set("platform", CallsPlatform)
	q.Set("appVersion", CallsClientVersion)
	q.Set("version", CallsProtocolVersion)
	q.Set("device", CallsDeviceType)
	q.Set("capabilities", CallsCapabilities)
	q.Set("clientType", CallsClientTypeSignaling)
	q.Set("tgt", "start")
	sigParsed.RawQuery = q.Encode()
	sigURL := sigParsed.String()

	sig, err := newSignalingClient(ctx, sigURL, c.log)
	if err != nil {
		return nil, err
	}

	hello, err := sig.receiveServerHello()
	if err != nil {
		sig.close()
		return nil, err
	}

	calleeIDStr := strconv.FormatInt(calleeExternalID, 10)
	peerID := int64(0)
	for _, p := range hello.Conversation.Participants {
		if p.ExternalID.ID == calleeIDStr {
			peerID = p.ID
			break
		}
	}
	if peerID == 0 {
		sig.close()
		return nil, fmt.Errorf("calls: peer not found in participants")
	}

	ic := newICEConnector(sig, peerID, c.log)
	iceConn, agent, err := ic.connect(ctx, startResp, forceRelay, true)
	if err != nil {
		sig.close()
		return nil, err
	}
	c.log.Info("ICE connection established")

	return &CallSession{
		conn:    iceConn,
		agent:   agent,
		closeFn: func() error { return sig.close() },
	}, nil
}
```

Also add `"strconv"` to the imports if not already present. The current imports in `calls.go` (lines 3-8) are:

```go
import (
	"context"
	"fmt"
	"net/url"
	"strings"
)
```

Change to:

```go
import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)
```

- [ ] **Step 2: Remove GetCallsExternalUserID()**

In `maxclient/calls.go`, delete the `GetCallsExternalUserID` method (lines 178-191 in the original file — will be at the end after rewriting Call()):

```go
// DELETE THIS ENTIRE BLOCK:
// GetCallsExternalUserID returns this account's external user ID in the Calls system.
// This is needed to tell the caller which externalIds to pass to startConversation.
func (c *Client) GetCallsExternalUserID(ctx context.Context) (string, error) {
	callToken, err := c.getCallToken(ctx)
	if err != nil {
		return "", err
	}
	api := newCallsAPI(c.httpClient)
	loginResp, err := api.login(ctx, callToken)
	if err != nil {
		return "", err
	}
	return loginResp.ExternalUserID, nil
}
```

- [ ] **Step 3: Remove startConversation() from calls_api.go**

In `maxclient/calls_api.go`, delete the `startConversation` method (lines 84-106):

```go
// DELETE THIS ENTIRE BLOCK:
func (a *callsAPI) startConversation(ctx context.Context, sessionKey, conversationID, externalUserID string) (*startConversationResponse, error) {
	// ... entire method body ...
}
```

Also delete `startConversationPayload` from `maxclient/calls_types.go` (lines 66-68):

```go
// DELETE THIS:
type startConversationPayload struct {
	IsVideo bool `json:"is_video"`
}
```

- [ ] **Step 4: Remove TestCallsAPI_StartConversation from calls_test.go**

In `maxclient/calls_test.go`, delete the `TestCallsAPI_StartConversation` test (lines 76-99):

```go
// DELETE THIS ENTIRE BLOCK:
func TestCallsAPI_StartConversation(t *testing.T) {
	// ... entire test body ...
}
```

- [ ] **Step 5: Run all tests**

Run: `cd /home/parkington/git/go-max-client && go test ./maxclient/ -v`
Expected: all tests PASS

- [ ] **Step 6: Run go vet**

Run: `cd /home/parkington/git/go-max-client && go vet ./maxclient/`
Expected: no issues

- [ ] **Step 7: Commit**

```bash
git add maxclient/calls.go maxclient/calls_api.go maxclient/calls_types.go maxclient/calls_test.go
git commit -m "feat: Call() uses FastStart (opcode 78), remove HTTP call flow

Replace 3-step HTTP chain (getCallToken → anonymLogin → startConversation)
with single WebSocket request via opcode 78.

BREAKING: Call() parameter type changed from string to int64.
BREAKING: GetCallsExternalUserID() removed — use FindUserByPhone().ExternalID."
```

---

## Post-Implementation Verification

After all tasks are complete:

- [ ] **Full test suite**: `cd /home/parkington/git/go-max-client && go test ./... -v`
- [ ] **Build check**: `cd /home/parkington/git/go-max-client && go build ./...`
- [ ] **Vet check**: `cd /home/parkington/git/go-max-client && go vet ./...`
- [ ] **Verify no real data leaked**: `grep -rn '79913086046\|79166105979\|23731790\|233947328\|211541646\|145320822\|1125899947283405' maxclient/`
  Expected: no matches
