package maxclient

import (
	"context"
	"fmt"
)

// CreateGroup creates a new group chat with the given name and member IDs.
func (c *Client) CreateGroup(ctx context.Context, name string, memberIDs []int64) (*Packet, error) {
	resp, err := c.InvokeMethod(ctx, OpcodeSendMessage, map[string]any{
		"message": map[string]any{
			"cid": generateCID(),
			"attaches": []map[string]any{
				{
					"_type":    "CONTROL",
					"event":    "new",
					"chatType": "CHAT",
					"title":    name,
					"userIds":  memberIDs,
				},
			},
		},
		"notify": true,
	})
	if err != nil {
		return nil, err
	}
	if err := checkResponseError(resp); err != nil {
		return nil, fmt.Errorf("create group: %w", err)
	}
	return resp, nil
}

// GetGroupMembers retrieves the member list of a group.
func (c *Client) GetGroupMembers(ctx context.Context, groupID int64) (*Packet, error) {
	resp, err := c.InvokeMethod(ctx, OpcodeGetMembers, map[string]any{
		"type": "MEMBER", "marker": 0, "chatId": groupID, "count": 500,
	})
	if err != nil {
		return nil, err
	}
	if err := checkResponseError(resp); err != nil {
		return nil, fmt.Errorf("get group members: %w", err)
	}
	return resp, nil
}

// JoinGroup joins a group chat.
func (c *Client) JoinGroup(ctx context.Context, groupID int64) error {
	resp, err := c.InvokeMethod(ctx, OpcodeJoinChannel, map[string]any{"chatId": groupID})
	if err != nil {
		return fmt.Errorf("join group: %w", err)
	}
	if err := checkResponseError(resp); err != nil {
		return fmt.Errorf("join group: %w", err)
	}
	return nil
}

// LeaveGroup leaves a group chat.
func (c *Client) LeaveGroup(ctx context.Context, groupID int64) error {
	resp, err := c.InvokeMethod(ctx, OpcodeGroupOps, map[string]any{"chatId": groupID, "operation": "leave"})
	if err != nil {
		return fmt.Errorf("leave group: %w", err)
	}
	if err := checkResponseError(resp); err != nil {
		return fmt.Errorf("leave group: %w", err)
	}
	return nil
}
