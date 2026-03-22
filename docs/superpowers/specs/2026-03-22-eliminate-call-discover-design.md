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
- Corrected comment: `contact.id` is the user's externalId in OneMe (not a "contact record ID")

### 3. AddContactByPhone — make private

Remove the public `AddContactByPhone()` wrapper (line 14 of `contacts.go`). The private `addContactByPhone()` already exists and is unchanged. `FindUserByPhone` is the single public method for resolving users by phone.

### 4. FastStart — opcode 78 (`calls_oneme.go`, `calls_types.go`)

New opcode constant:

```go
OpcodeFastStartCall = 78
```

Request types:

```go
type fastStartRequest struct {
    ConversationID string  `json:"conversationId"` // generated UUID (via newUUID() from auth.go)
    CalleeIDs      []int64 `json:"calleeIds"`
    InternalParams string  `json:"internalParams"` // JSON-encoded fastStartInternalParams
    IsVideo        bool    `json:"isVideo"`
}

type fastStartInternalParams struct {
    DeviceID        string `json:"deviceId"`        // generated UUID (via newUUID())
    SDKVersion      string `json:"sdkVersion"`      // CallsClientVersion ("1.1")
    ClientAppKey    string `json:"clientAppKey"`     // CallsAppKey ("CNHIJPLGDIHBABABA")
    Platform        string `json:"platform"`         // CallsPlatform ("WEB")
    ProtocolVersion int    `json:"protocolVersion"`  // 5 (number, not string)
    DomainID        string `json:"domainId"`         // "" (empty string)
    Capabilities    string `json:"capabilities"`     // CallsCapabilities ("603F")
}
```

Response parsing (double unmarshal — JSON-in-JSON):

```go
// First level: opcode 78 response
type fastStartResponse struct {
    InternalCallerParams string `json:"internalCallerParams"` // JSON string
}

// Second level: unmarshal InternalCallerParams string into startConversationResponse
// (reuses existing type — has Endpoint, TurnServer, StunServer with matching JSON tags)
```

Error handling: `fastStartCall()` must validate that `InternalCallerParams` is non-empty after the first unmarshal, and that `Endpoint` is non-empty after the second unmarshal.

### 5. Call() — full flow after FastStart (`calls.go`)

```go
// Before:
func (c *Client) Call(ctx context.Context, calleeExternalID string, forceRelay bool) (*CallSession, error)

// After:
func (c *Client) Call(ctx context.Context, calleeExternalID int64, forceRelay bool) (*CallSession, error)
```

**`getCallToken` (opcode 158) is NOT needed for FastStart.** The OneMe WebSocket session is already authenticated; opcode 78 does not require a separate call token.

Full `Call()` flow after the change:

1. `fastStartCall(ctx, calleeExternalID)` → returns `*startConversationResponse` (endpoint + TURN/STUN)
2. Parse `Endpoint` URL, append query params (platform, version, capabilities, etc.) — same as current code
3. Connect signaling WebSocket at the endpoint — unchanged
4. Receive server hello, find peer participant — unchanged
5. ICE connect via `iceConnector.connect()` — unchanged

Steps 2–5 are identical to the current implementation. Only step 1 changes (FastStart replaces getCallToken + HTTP login + HTTP startConversation).

**Peer discovery after FastStart:** The current `Call()` finds the peer by comparing `participant.ExternalID.ID` (string) against `loginResp.ExternalUserID` (string from HTTP login). After FastStart, we no longer have `loginResp`. Instead, find the peer by matching `calleeExternalID` against participants: `strconv.FormatInt(calleeExternalID, 10) == participant.ExternalID.ID` (note: `ExternalID.ID` is a `string` in the signaling protocol, so `int64` → `string` conversion is needed). The single `calleeIds` value is wrapped as `[]int64{calleeExternalID}` in the request.

### 6. Remove GetCallsExternalUserID()

Fully removed (not deprecated). No longer needed since `FindUserByPhone` returns `ExternalID`.

### 7. Preserved code

- `calls_api.go` — `login()` still needed for `WaitForCall()` (incoming calls). `startConversation()` becomes dead code after this change — remove it and its test (`TestCallsAPI_StartConversation`).
- `calls_signaling.go` — unchanged
- `calls_ice.go` — unchanged
- `iceConnector.connect(*startConversationResponse, ...)` — contract preserved
- `getCallToken()` in `calls_oneme.go` — still needed for `WaitForCall()`, only removed from the outgoing `Call()` path
- `newUUID()` in `auth.go` — used by `fastStartCall()` for `ConversationID` and `DeviceID`

## File Change Summary

| File | Changes |
|---|---|
| `types.go` | `User` += `ExternalID int64` |
| `contacts.go` | Remove public `AddContactByPhone()`, `FindUserByPhone` saves both IDs, fix comment |
| `protocol.go` | `OpcodeFastStartCall = 78` |
| `calls_types.go` | Add `fastStartRequest`, `fastStartInternalParams`, `fastStartResponse` |
| `calls_oneme.go` | Add private `fastStartCall()` method (opcode 78, double unmarshal) |
| `calls.go` | `Call()`: signature `int64`, body → FastStart, updated peer discovery. Remove `GetCallsExternalUserID()` |
| `contacts_test.go` | `TestAddContactByPhone`, `TestAddContactByPhoneNoONEMEName`, `TestAddContactByPhoneServerError`: convert to use private `addContactByPhone` (package-internal tests). `TestFindUserByPhone`: assert both `user.ID` (chatID) and `user.ExternalID` (externalId) |
| `calls_test.go` | Remove `TestCallsAPI_StartConversation` (dead code) |
| `calls_api.go` | Remove `startConversation()` method (dead code after FastStart; `login()` preserved for `WaitForCall`) |
| `calls_signaling.go` | No changes |
| `calls_ice.go` | No changes |

## Breaking Changes

1. `Call(ctx, externalID string, ...)` → `Call(ctx, externalID int64, ...)` — parameter type
2. `AddContactByPhone()` — removed from public API
3. `GetCallsExternalUserID()` — removed
4. `User.ExternalID` — new field (non-breaking, additive)

All removed/changed APIs are covered by `FindUserByPhone` + `User.ExternalID`.

Caller migration:

```go
// Before:
extID, _ := client.GetCallsExternalUserID(ctx)
session, _ := client.Call(ctx, extID, false)

// After:
user, _ := client.FindUserByPhone(ctx, "+79991234567")
session, _ := client.Call(ctx, user.ExternalID, false)
```

## Security

Real phone numbers and user IDs must NOT appear in source code, tests, or comments. Use fake values in tests (e.g. +70001234567, 12345678, 87654321).
