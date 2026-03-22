package maxclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

// FindUserByPhone looks up a user by phone number.
// Calls addContactByPhone to get the user's externalId (contact.id),
// then resolves the chatID from chat participants via ResolveChannel.
func (c *Client) FindUserByPhone(ctx context.Context, phone string) (*User, error) {
	user, err := c.addContactByPhone(ctx, phone, "_")
	if err != nil {
		return nil, err
	}
	// contact.id from AddContactByPhone is the user's externalId in OneMe.
	user.ExternalID = user.ID

	// Resolve the chatID from chat participants.
	resp, err := c.ResolveChannel(ctx, user.ExternalID)
	if err != nil {
		return nil, fmt.Errorf("resolve contact chat: %w", err)
	}

	var resolved struct {
		Chats []struct {
			Participants map[string]any `json:"participants"`
		} `json:"chats"`
	}
	if err := json.Unmarshal(resp.Payload, &resolved); err == nil {
		ownID := strconv.FormatInt(c.ownUserID, 10)
		for _, chat := range resolved.Chats {
			for pidStr := range chat.Participants {
				if pidStr != ownID {
					if chatID, err := strconv.ParseInt(pidStr, 10, 64); err == nil {
						user.ID = chatID
						return user, nil
					}
				}
			}
		}
	}

	return user, nil
}

func (c *Client) addContactByPhone(ctx context.Context, phone, firstName string) (*User, error) {
	resp, err := c.InvokeMethod(ctx, OpcodeAddContactByPhone, map[string]any{
		"phone":     phone,
		"firstName": firstName,
	})
	if err != nil {
		return nil, fmt.Errorf("add contact by phone: %w", err)
	}
	if err := checkResponseError(resp); err != nil {
		return nil, fmt.Errorf("add contact by phone: %w", err)
	}

	var result struct {
		Contact struct {
			ID    int64 `json:"id"`
			Phone int64 `json:"phone"`
			Names []struct {
				FirstName string `json:"firstName"`
				LastName  string `json:"lastName"`
				Type      string `json:"type"`
			} `json:"names"`
		} `json:"contact"`
	}
	if err := json.Unmarshal(resp.Payload, &result); err != nil {
		return nil, fmt.Errorf("parse add contact response: %w", err)
	}

	user := &User{
		ID:    result.Contact.ID,
		Phone: "+" + strconv.FormatInt(result.Contact.Phone, 10),
	}

	// Prefer ONEME name (user's actual profile name), fallback to first entry.
	for _, n := range result.Contact.Names {
		if n.Type == "ONEME" {
			user.FirstName = n.FirstName
			user.LastName = n.LastName
			break
		}
	}
	if user.FirstName == "" && len(result.Contact.Names) > 0 {
		user.FirstName = result.Contact.Names[0].FirstName
		user.LastName = result.Contact.Names[0].LastName
	}

	return user, nil
}
