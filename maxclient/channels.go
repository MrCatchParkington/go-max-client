package maxclient

import "context"

// ResolveChannel resolves a channel by its ID.
func (c *Client) ResolveChannel(ctx context.Context, channelID int64) (*Packet, error) {
	return c.InvokeMethod(ctx, OpcodeResolveChannel, map[string]any{"chatIds": []int64{channelID}})
}

// SubscribeChat subscribes to events for the given chat ID.
// Without subscribing, the WebSocket does not receive message events for a chat.
func (c *Client) SubscribeChat(ctx context.Context, chatID int64) (*Packet, error) {
	return c.InvokeMethod(ctx, OpcodeSubscribeChat, map[string]any{"chatId": chatID})
}

// ResolveByLink resolves a channel or group by its invite link.
func (c *Client) ResolveByLink(ctx context.Context, link string) (*Packet, error) {
	return c.InvokeMethod(ctx, OpcodeResolveByLink, map[string]any{"link": link})
}
