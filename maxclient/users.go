package maxclient

import (
	"context"
	"fmt"
)

// ResolveUsers resolves users by their IDs.
func (c *Client) ResolveUsers(ctx context.Context, userIDs []int64) (*Packet, error) {
	resp, err := c.InvokeMethod(ctx, OpcodeResolveUsers, map[string]any{"contactIds": userIDs})
	if err != nil {
		return nil, err
	}
	if err := checkResponseError(resp); err != nil {
		return nil, fmt.Errorf("resolve users: %w", err)
	}
	return resp, nil
}
