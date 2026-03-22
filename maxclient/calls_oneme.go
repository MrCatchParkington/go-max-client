package maxclient

import (
	"context"
	"encoding/json"
	"fmt"
)

// getCallToken requests a call token via opcode 158.
func (c *Client) getCallToken(ctx context.Context) (string, error) {
	resp, err := c.InvokeMethod(ctx, OpcodeCallToken, struct{}{})
	if err != nil {
		return "", fmt.Errorf("calls: get call token: %w", err)
	}
	var result callTokenResponse
	if err := json.Unmarshal(resp.Payload, &result); err != nil {
		return "", fmt.Errorf("calls: parse call token: %w", err)
	}
	if result.Token == "" {
		return "", fmt.Errorf("calls: empty call token")
	}
	return result.Token, nil
}

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

	// FastStart uses "turn"/"stun" field names, not "turn_server"/"stun_server".
	var fsParams fastStartCallerParams
	if err := json.Unmarshal([]byte(fsResp.InternalCallerParams), &fsParams); err != nil {
		return nil, fmt.Errorf("calls: parse internal caller params: %w", err)
	}
	if fsParams.Endpoint == "" {
		return nil, fmt.Errorf("calls: fast start: empty endpoint in internal caller params")
	}

	// Convert to startConversationResponse (used by ICE connector).
	startResp := &startConversationResponse{Endpoint: fsParams.Endpoint}
	startResp.TurnServer.Urls = fsParams.Turn.Urls
	startResp.TurnServer.Username = fsParams.Turn.Username
	startResp.TurnServer.Password = fsParams.Turn.Credential
	startResp.StunServer.Urls = fsParams.Stun.Urls

	return startResp, nil
}

// waitIncomingCall blocks until an incoming call notification (opcode 137) arrives.
func (c *Client) waitIncomingCall(ctx context.Context) (*incomingCallPayload, *vcpDecoded, error) {
	for {
		select {
		case <-ctx.Done():
			return nil, nil, ctx.Err()
		case <-c.lifecycleCtx.Done():
			return nil, nil, ErrDisconnected
		case pkt, ok := <-c.callIncoming:
			if !ok {
				return nil, nil, fmt.Errorf("calls: call incoming channel closed")
			}
			var payload incomingCallPayload
			if err := json.Unmarshal(pkt.Payload, &payload); err != nil {
				c.log.Warn("calls: failed to parse incoming call", "err", err)
				continue
			}
			vcp, err := decodeVCP(payload.Vcp)
			if err != nil {
				c.log.Warn("calls: failed to decode VCP", "err", err)
				continue
			}
			return &payload, vcp, nil
		}
	}
}
