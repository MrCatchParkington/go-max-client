package maxclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
)

// AddContactByPhone adds a user to contacts by phone number and returns their info.
// The phone should be in E.164 format (e.g. "+79991234567").
// The firstName is stored as the contact's custom display name.
// The returned User.ID can be used directly as a chatID for SendMessage (DM chatID == userID).
func (c *Client) AddContactByPhone(ctx context.Context, phone, firstName string) (*User, error) {
	return c.addContactByPhone(ctx, phone, firstName)
}

// FindUserByPhone looks up a user by phone number.
// Calls AddContactByPhone, then resolves the real userId from chat participants,
// because contact.id is a contact record ID, not the user's real ID.
func (c *Client) FindUserByPhone(ctx context.Context, phone string) (*User, error) {
	user, err := c.addContactByPhone(ctx, phone, "_")
	if err != nil {
		return nil, err
	}
	contactID := user.ID

	// Resolve the real userId from chat participants.
	resp, err := c.ResolveChannel(ctx, contactID)
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
					if realID, err := strconv.ParseInt(pidStr, 10, 64); err == nil {
						user.ID = realID
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
