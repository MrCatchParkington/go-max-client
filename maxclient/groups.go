package maxclient

import (
	"context"
	"fmt"
)

// CreateGroup creates a new group chat with the given name and member IDs.
func (c *Client) CreateGroup(ctx context.Context, name string, memberIDs []int64) (*Packet, error) {
	return c.InvokeMethod(ctx, OpcodeSendMessage, map[string]any{
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
}

// GetGroupMembers retrieves the member list of a group.
func (c *Client) GetGroupMembers(ctx context.Context, groupID int64) (*Packet, error) {
	return c.InvokeMethod(ctx, OpcodeGetMembers, map[string]any{
		"type": "MEMBER", "marker": 0, "chatId": groupID, "count": 500,
	})
}

// JoinGroup joins a group chat.
func (c *Client) JoinGroup(ctx context.Context, groupID int64) error {
	_, err := c.InvokeMethod(ctx, OpcodeJoinChannel, map[string]any{"chatId": groupID})
	if err != nil {
		return fmt.Errorf("join group: %w", err)
	}
	return nil
}

// LeaveGroup leaves a group chat.
func (c *Client) LeaveGroup(ctx context.Context, groupID int64) error {
	_, err := c.InvokeMethod(ctx, OpcodeGroupOps, map[string]any{"chatId": groupID, "operation": "leave"})
	if err != nil {
		return fmt.Errorf("leave group: %w", err)
	}
	return nil
}
