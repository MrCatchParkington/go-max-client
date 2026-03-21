package maxclient

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	pionIce "github.com/pion/ice/v4"
	"github.com/pion/stun/v3"
)

type iceConnector struct {
	signaling *signalingClient
	peerID    int64
	log       *slog.Logger
}

func newICEConnector(signaling *signalingClient, peerID int64, log *slog.Logger) *iceConnector {
	return &iceConnector{
		signaling: signaling,
		peerID:    peerID,
		log:       log,
	}
}

func (ic *iceConnector) connect(ctx context.Context, turn *startConversationResponse, forceRelay, isControlling bool) (*pionIce.Conn, io.Closer, error) {
	// Parse ICE servers
	var urls []*stun.URI
	for _, u := range turn.StunServer.Urls {
		uri, err := stun.ParseURI(u)
		if err != nil {
			ic.log.Warn("failed to parse STUN URI", "uri", u, "err", err)
			continue
		}
		urls = append(urls, uri)
	}
	for _, u := range turn.TurnServer.Urls {
		uri, err := stun.ParseURI(u)
		if err != nil {
			ic.log.Warn("failed to parse TURN URI", "uri", u, "err", err)
			continue
		}
		uri.Username = turn.TurnServer.Username
		uri.Password = turn.TurnServer.Password
		urls = append(urls, uri)
	}

	// Configure agent
	agentCfg := &pionIce.AgentConfig{
		Urls: urls,
	}
	if forceRelay {
		agentCfg.CandidateTypes = []pionIce.CandidateType{pionIce.CandidateTypeRelay}
	}

	agent, err := pionIce.NewAgent(agentCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("ice: create agent: %w", err)
	}

	// Send local credentials
	localUfrag, localPwd, err := agent.GetLocalUserCredentials()
	if err != nil {
		agent.Close()
		return nil, nil, fmt.Errorf("ice: get local credentials: %w", err)
	}
	if err := ic.signaling.sendData(ic.peerID, iceCredentials{UFrag: localUfrag, Password: localPwd}); err != nil {
		agent.Close()
		return nil, nil, fmt.Errorf("ice: send credentials: %w", err)
	}

	// Receive remote credentials
	var remoteCreds iceCredentials
	if err := ic.signaling.receiveData(&remoteCreds); err != nil {
		agent.Close()
		return nil, nil, fmt.Errorf("ice: receive credentials: %w", err)
	}

	// Register OnCandidate callback
	if err := agent.OnCandidate(func(c pionIce.Candidate) {
		if c == nil {
			ic.signaling.sendData(ic.peerID, iceNewCandidate{}) //nolint:errcheck
			return
		}
		ic.signaling.sendData(ic.peerID, iceNewCandidate{Candidate: c.Marshal()}) //nolint:errcheck
	}); err != nil {
		agent.Close()
		return nil, nil, fmt.Errorf("ice: set OnCandidate: %w", err)
	}

	// Start gathering candidates
	if err := agent.GatherCandidates(); err != nil {
		agent.Close()
		return nil, nil, fmt.Errorf("ice: gather candidates: %w", err)
	}

	// Receive remote candidates in background (terminates on ctx cancel or signaling close)
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			var candidate iceNewCandidate
			if err := ic.signaling.receiveData(&candidate); err != nil {
				return
			}
			if candidate.Candidate == "" {
				return
			}
			c, err := pionIce.UnmarshalCandidate(candidate.Candidate)
			if err != nil {
				ic.log.Warn("failed to unmarshal remote candidate", "err", err)
				continue
			}
			agent.AddRemoteCandidate(c) //nolint:errcheck
		}
	}()

	// Connect — controlling side dials, controlled side accepts
	var conn *pionIce.Conn
	if isControlling {
		conn, err = agent.Dial(ctx, remoteCreds.UFrag, remoteCreds.Password)
	} else {
		conn, err = agent.Accept(ctx, remoteCreds.UFrag, remoteCreds.Password)
	}
	if err != nil {
		agent.Close()
		return nil, nil, fmt.Errorf("ice: connect: %w", err)
	}

	return conn, agent, nil
}
