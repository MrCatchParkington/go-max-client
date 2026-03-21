package maxclient

import (
	"context"
	"fmt"
	"math/rand/v2"
)

// generateCID returns a random client-side message ID.
// Range ~1.75e12 to ~2e12 matches the MAX web client behavior (from Python vkmax reference).
func generateCID() int64 {
	return rand.Int64N(250000000000) + 1750000000000
}

// SendMessage sends a text message to a chat.
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string, opts ...SendMessageOpts) (*Packet, error) {
	msg := map[string]any{
		"text":     text,
		"cid":      generateCID(),
		"elements": []any{},
		"attaches": []any{},
	}

	var opt SendMessageOpts
	if len(opts) > 0 {
		opt = opts[0]
	}

	if opt.ReplyTo != "" {
		msg["link"] = map[string]any{
			"type":      "REPLY",
			"messageId": opt.ReplyTo,
		}
	}

	if len(opt.Attaches) > 0 {
		msg["attaches"] = opt.Attaches
	}

	payload := map[string]any{
		"chatId":  chatID,
		"message": msg,
		"notify":  true,
	}

	resp, err := c.InvokeMethod(ctx, OpcodeSendMessage, payload)
	if err != nil {
		return nil, err
	}
	if err := checkResponseError(resp); err != nil {
		return nil, fmt.Errorf("send message: %w", err)
	}
	return resp, nil
}

// DeleteMessage deletes messages from a chat.
func (c *Client) DeleteMessage(ctx context.Context, chatID int64, messageIDs []string, forMe bool) error {
	resp, err := c.InvokeMethod(ctx, OpcodeDeleteMessage, map[string]any{
		"chatId":     chatID,
		"messageIds": messageIDs,
		"forMe":      forMe,
	})
	if err != nil {
		return fmt.Errorf("delete message: %w", err)
	}
	if err := checkResponseError(resp); err != nil {
		return fmt.Errorf("delete message: %w", err)
	}
	return nil
}

// EditMessage edits a message in a chat. Optional attaches can be provided.
func (c *Client) EditMessage(ctx context.Context, chatID int64, messageID string, text string, attaches ...Attachment) error {
	payload := map[string]any{
		"chatId":    chatID,
		"messageId": messageID,
		"text":      text,
		"elements":  []any{},
	}
	if len(attaches) > 0 {
		// TODO: verify if the server expects "attachments" or "attaches" for edit
		// (SendMessage uses "attaches", but EditMessage has not been tested with attachments)
		payload["attachments"] = attaches
	}
	resp, err := c.InvokeMethod(ctx, OpcodeEditMessage, payload)
	if err != nil {
		return fmt.Errorf("edit message: %w", err)
	}
	if err := checkResponseError(resp); err != nil {
		return fmt.Errorf("edit message: %w", err)
	}
	return nil
}

// PinMessage pins a message in a chat.
func (c *Client) PinMessage(ctx context.Context, chatID int64, messageID string, notify bool) error {
	resp, err := c.InvokeMethod(ctx, OpcodePinMessage, map[string]any{
		"chatId":    chatID,
		"notifyPin": notify,
		"messageId": messageID,
	})
	if err != nil {
		return fmt.Errorf("pin message: %w", err)
	}
	if err := checkResponseError(resp); err != nil {
		return fmt.Errorf("pin message: %w", err)
	}
	return nil
}

// GetHistory retrieves chat message history.
func (c *Client) GetHistory(ctx context.Context, chatID int64, count int) (*Packet, error) {
	resp, err := c.InvokeMethod(ctx, OpcodeGetHistory, map[string]any{
		"chatId":      chatID,
		"from":        0,
		"forward":     count,
		"backward":    0,
		"getMessages": true,
	})
	if err != nil {
		return nil, err
	}
	if err := checkResponseError(resp); err != nil {
		return nil, fmt.Errorf("get history: %w", err)
	}
	return resp, nil
}
