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
