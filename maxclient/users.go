package maxclient

import "context"

// ResolveUsers resolves users by their IDs.
func (c *Client) ResolveUsers(ctx context.Context, userIDs []int64) (*Packet, error) {
	return c.InvokeMethod(ctx, OpcodeResolveUsers, map[string]any{"contactIds": userIDs})
}
