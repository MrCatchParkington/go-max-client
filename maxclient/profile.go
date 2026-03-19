package maxclient

import (
	"context"
	"fmt"
)

// SetProfile updates the current user's profile fields. Only non-nil fields are sent.
func (c *Client) SetProfile(ctx context.Context, opts ProfileOpts) error {
	payload := map[string]any{}
	if opts.FirstName != nil {
		payload["firstName"] = *opts.FirstName
	}
	if opts.LastName != nil {
		payload["lastName"] = *opts.LastName
	}
	if opts.Bio != nil {
		payload["description"] = *opts.Bio
	}
	_, err := c.InvokeMethod(ctx, OpcodeChangeProfile, payload)
	if err != nil {
		return fmt.Errorf("set profile: %w", err)
	}
	return nil
}

// SetSettings updates the current user's settings.
func (c *Client) SetSettings(ctx context.Context, opts SettingsOpts) error {
	_, err := c.InvokeMethod(ctx, OpcodeSetSettings, map[string]any{"settings": opts.Settings})
	if err != nil {
		return fmt.Errorf("set settings: %w", err)
	}
	return nil
}
