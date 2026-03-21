package maxclient

import (
	"context"
	"fmt"
)

// ResolveChannel resolves a channel by its ID.
func (c *Client) ResolveChannel(ctx context.Context, channelID int64) (*Packet, error) {
	resp, err := c.InvokeMethod(ctx, OpcodeResolveChannel, map[string]any{"chatIds": []int64{channelID}})
	if err != nil {
		return nil, err
	}
	if err := checkResponseError(resp); err != nil {
		return nil, fmt.Errorf("resolve channel: %w", err)
	}
	return resp, nil
}

// SubscribeChat subscribes to events for the given chat ID.
// Without subscribing, the WebSocket does not receive message events for a chat.
func (c *Client) SubscribeChat(ctx context.Context, chatID int64) (*Packet, error) {
	resp, err := c.InvokeMethod(ctx, OpcodeSubscribeChat, map[string]any{"chatId": chatID})
	if err != nil {
		return nil, err
	}
	if err := checkResponseError(resp); err != nil {
		return nil, fmt.Errorf("subscribe chat: %w", err)
	}
	return resp, nil
}

// ResolveByLink resolves a channel or group by its invite link.
func (c *Client) ResolveByLink(ctx context.Context, link string) (*Packet, error) {
	resp, err := c.InvokeMethod(ctx, OpcodeResolveByLink, map[string]any{"link": link})
	if err != nil {
		return nil, err
	}
	if err := checkResponseError(resp); err != nil {
		return nil, fmt.Errorf("resolve by link: %w", err)
	}
	return resp, nil
}
