package maxclient

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Call initiates a call to the given user and returns a CallSession
// with a bidirectional data stream over ICE.
// calleeExternalID is the callee's externalId in OneMe
// (obtain via FindUserByPhone — it's the User.ExternalID field).
// forceRelay=true restricts to TURN-only candidates (no P2P).
func (c *Client) Call(ctx context.Context, calleeExternalID int64, forceRelay bool) (*CallSession, error) {
	startResp, err := c.fastStartCall(ctx, calleeExternalID)
	if err != nil {
		return nil, err
	}
	c.log.Info("fast start call", "endpoint", startResp.Endpoint)

	sigParsed, err := url.Parse(startResp.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("calls: parse signaling endpoint: %w", err)
	}
	q := sigParsed.Query()
	q.Set("platform", CallsPlatform)
	q.Set("appVersion", CallsClientVersion)
	q.Set("version", CallsProtocolVersion)
	q.Set("device", CallsDeviceType)
	q.Set("capabilities", CallsCapabilities)
	q.Set("clientType", CallsClientTypeSignaling)
	q.Set("tgt", "start")
	sigParsed.RawQuery = q.Encode()
	sigURL := sigParsed.String()

	sig, err := newSignalingClient(ctx, sigURL, c.log)
	if err != nil {
		return nil, err
	}

	hello, err := sig.receiveServerHello()
	if err != nil {
		sig.close()
		return nil, err
	}

	calleeIDStr := strconv.FormatInt(calleeExternalID, 10)
	peerID := int64(0)
	for _, p := range hello.Conversation.Participants {
		if p.ExternalID.ID == calleeIDStr {
			peerID = p.ID
			break
		}
	}
	if peerID == 0 {
		sig.close()
		return nil, fmt.Errorf("calls: peer not found in participants")
	}

	ic := newICEConnector(sig, peerID, c.log)
	iceConn, agent, err := ic.connect(ctx, startResp, forceRelay, true)
	if err != nil {
		sig.close()
		return nil, err
	}
	c.log.Info("ICE connection established")

	return &CallSession{
		conn:    iceConn,
		agent:   agent,
		closeFn: func() error { return sig.close() },
	}, nil
}

// WaitForCall blocks until an incoming call arrives and establishes
// the ICE connection. Returns a CallSession with a bidirectional data stream.
func (c *Client) WaitForCall(ctx context.Context, forceRelay bool) (*CallSession, error) {
	// Eagerly cache calls API login while waiting for incoming call.
	// This avoids ~500ms of HTTP round-trips after the call arrives,
	// which is critical because the signaling server has a short join timeout.
	if _, err := c.getOrCacheCallsLogin(ctx); err != nil {
		c.log.Warn("pre-cache calls login failed (will retry on call)", "err", err)
	}

	incoming, vcp, err := c.waitIncomingCall(ctx)
	if err != nil {
		return nil, err
	}
	c.log.Info("incoming call", "callerID", incoming.CallerID, "conversationID", incoming.ConversationID)

	loginResp, err := c.getOrCacheCallsLogin(ctx)
	if err != nil {
		return nil, err
	}

	sigQ := url.Values{
		"userId":         {loginResp.UID},
		"entityType":     {"USER"},
		"conversationId": {incoming.ConversationID},
		"token":          {vcp.SignalingToken},
		"platform":       {CallsPlatform},
		"appVersion":     {CallsClientVersion},
		"version":        {CallsProtocolVersion},
		"device":         {CallsDeviceType},
		"capabilities":   {CallsCapabilities},
		"clientType":     {CallsClientTypeSignaling},
		"tgt":            {"start"},
	}
	sigURL := vcp.SignalingServer + "?" + sigQ.Encode()

	sig, err := newSignalingClient(ctx, sigURL, c.log)
	if err != nil {
		return nil, err
	}

	hello, err := sig.receiveServerHello()
	if err != nil {
		sig.close()
		return nil, err
	}

	peerID := int64(0)
	for _, p := range hello.Conversation.Participants {
		if p.ExternalID.ID != loginResp.ExternalUserID {
			peerID = p.ID
			break
		}
	}
	if peerID == 0 {
		sig.close()
		return nil, fmt.Errorf("calls: caller not found in participants")
	}

	if err := sig.sendAcceptCall(); err != nil {
		sig.close()
		return nil, err
	}

	turnURLs := strings.Split(vcp.TurnServers, ",")
	turnResp := &startConversationResponse{}
	turnResp.TurnServer.Urls = turnURLs
	turnResp.TurnServer.Username = vcp.TurnUser
	turnResp.TurnServer.Password = vcp.TurnPassword
	if vcp.StunServer != "" {
		turnResp.StunServer.Urls = []string{vcp.StunServer}
	}

	ic := newICEConnector(sig, peerID, c.log)
	iceConn, agent, err := ic.connect(ctx, turnResp, forceRelay, false)
	if err != nil {
		sig.close()
		return nil, err
	}
	c.log.Info("ICE connection established (incoming call)")

	return &CallSession{
		conn:    iceConn,
		agent:   agent,
		closeFn: func() error { return sig.close() },
	}, nil
}
