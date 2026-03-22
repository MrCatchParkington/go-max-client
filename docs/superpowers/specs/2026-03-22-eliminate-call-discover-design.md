# Eliminate CALL_DISCOVER Dependency

**Date:** 2026-03-22
**Approach:** B (Full — FindUserByPhone + FastStart)

## Problem

The `go-max-client` library requires callers to use a CALL_DISCOVER hack — sending a text message to a server so it responds with its `externalId` — before initiating calls. This is because:

1. `FindUserByPhone` loses the `externalId` (contact.id from AddContactByPhone) by overwriting it with `chatID` from ResolveChannel
2. `Call()` uses a 3-step HTTP chain (getCallToken → anonymLogin → startConversation) against `calls.okcdn.ru`

## Background: MAX ID Systems

- **chatID** (e.g. DM channel ID) — used for chat operations: GetHistory, SendMessage, SubscribeChat
- **externalId** (userId in OneMe) — used for calls, appears as `sender` in messages, `participants` in chats
- **OK uid** — internal ID in Calls API (calls.okcdn.ru), not needed directly

Key finding: `AddContactByPhone` (opcode 41) returns `contact.id` which IS the `externalId`, not a "contact record ID" as previously documented.

Key finding: Web client (web.max.ru) uses opcode 78 (FastStart) — a single WebSocket request that replaces the 3-step HTTP chain for outgoing calls.

## Design

### 1. User struct (`types.go`)

Add `ExternalID` field:

```go
type User struct {
    ID         int64  `json:"id"`                  // chatID (DM channel ID)
    ExternalID int64  `json:"externalId,omitempty"` // userId in OneMe (for calls, sender in messages)
    FirstName  string `json:"firstName,omitempty"`
    LastName   string `json:"lastName,omitempty"`
    Phone      string `json:"phone,omitempty"`
}
```

### 2. FindUserByPhone (`contacts.go`)

- Preserves `contact.id` from AddContactByPhone as `ExternalID`
- chatID from ResolveChannel as `ID` (unchanged semantics)
- Fix misleading comment about "contact record ID"

### 3. AddContactByPhone — make private

Remove the public `AddContactByPhone()` wrapper. The private `addContactByPhone()` already exists. `FindUserByPhone` is the single public method for resolving users by phone.

### 4. FastStart — opcode 78 (`calls_oneme.go`, `calls_types.go`)

New opcode constant:

```go
OpcodeFastStartCall = 78
```

Request types:

```go
type fastStartRequest struct {
    ConversationID string  `json:"conversationId"`
    CalleeIDs      []int64 `json:"calleeIds"`
    InternalParams string  `json:"internalParams"` // JSON-encoded fastStartInternalParams
    IsVideo        bool    `json:"isVideo"`
}

type fastStartInternalParams struct {
    DeviceID        string `json:"deviceId"`
    SDKVersion      string `json:"sdkVersion"`
    ClientAppKey    string `json:"clientAppKey"`
    Platform        string `json:"platform"`
    ProtocolVersion int    `json:"protocolVersion"` // number, not string (value: 5)
    DomainID        string `json:"domainId"`
    Capabilities    string `json:"capabilities"`
}
```

Response parsing (double unmarshal — JSON-in-JSON):

```go
// First level: opcode 78 response
type fastStartResponse struct {
    InternalCallerParams string `json:"internalCallerParams"` // JSON string
}

// Second level: unmarshal InternalCallerParams string into startConversationResponse
// (reuses existing type — has Endpoint, TurnServer, StunServer)
```

### 5. Call() signature change (`calls.go`)

```go
// Before:
func (c *Client) Call(ctx context.Context, calleeExternalID string, forceRelay bool) (*CallSession, error)

// After:
func (c *Client) Call(ctx context.Context, calleeExternalID int64, forceRelay bool) (*CallSession, error)
```

Body: single `InvokeMethod(ctx, OpcodeFastStartCall, ...)` replaces 3 HTTP requests.

### 6. Remove GetCallsExternalUserID()

Fully removed (not deprecated). No longer needed since `FindUserByPhone` returns `ExternalID`.

### 7. Preserved code

- `calls_api.go` — unchanged, still needed for `WaitForCall()`
- `calls_signaling.go` — unchanged
- `calls_ice.go` — unchanged
- `iceConnector.connect(*startConversationResponse, ...)` — contract preserved

## File Change Summary

| File | Changes |
|---|---|
| `types.go` | `User` += `ExternalID int64` |
| `contacts.go` | Remove public `AddContactByPhone()`, `FindUserByPhone` saves both IDs |
| `protocol.go` | `OpcodeFastStartCall = 78` |
| `calls_types.go` | Add `fastStartRequest`, `fastStartInternalParams`, `fastStartResponse` |
| `calls_oneme.go` | Add `fastStartCall()` method (opcode 78, double unmarshal) |
| `calls.go` | `Call()`: signature `int64`, body → FastStart. Remove `GetCallsExternalUserID()` |
| `contacts_test.go` | Update: AddContactByPhone now private, FindUserByPhone checks both ID and ExternalID |
| `calls_test.go` | Update: Call() parameter int64 |
| `calls_api.go` | No changes |
| `calls_signaling.go` | No changes |
| `calls_ice.go` | No changes |

## Breaking Changes

1. `Call(ctx, externalID string, ...)` → `Call(ctx, externalID int64, ...)` — parameter type
2. `AddContactByPhone()` — removed from public API
3. `GetCallsExternalUserID()` — removed
4. `User.ExternalID` — new field (non-breaking, additive)

All removed/changed APIs are covered by `FindUserByPhone` + `User.ExternalID`.

## Security

Real phone numbers and user IDs must NOT appear in source code, tests, or comments. Use fake values in tests (e.g. +70001234567, 12345678, 87654321).
