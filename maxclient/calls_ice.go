package maxclient

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	pionIce "github.com/pion/ice/v4"
	"github.com/pion/logging"
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
	stunCount, turnCount := 0, 0
	for i, u := range turn.StunServer.Urls {
		uri, err := stun.ParseURI(u)
		if err != nil {
			ic.log.Warn("failed to parse STUN URI", "uri", u, "err", err)
			continue
		}
		urls = append(urls, uri)
		stunCount++
		ic.log.Info("ice: STUN url", "n", i, "url", u)
	}
	for i, u := range turn.TurnServer.Urls {
		uri, err := stun.ParseURI(u)
		if err != nil {
			ic.log.Warn("failed to parse TURN URI", "uri", u, "err", err)
			continue
		}
		uri.Username = turn.TurnServer.Username
		uri.Password = turn.TurnServer.Password
		urls = append(urls, uri)
		turnCount++
		ic.log.Info("ice: TURN url", "n", i, "url", u)
	}
	ic.log.Info("ice: parsed servers", "stun", stunCount, "turn", turnCount, "is_controlling", isControlling)

	// Configure agent
	agentCfg := &pionIce.AgentConfig{
		Urls:          urls,
		LoggerFactory: pionSlogLoggerFactory{logger: ic.log},
	}
	if forceRelay {
		agentCfg.CandidateTypes = []pionIce.CandidateType{pionIce.CandidateTypeRelay}
	}

	agent, err := pionIce.NewAgent(agentCfg)
	if err != nil {
		return nil, nil, fmt.Errorf("ice: create agent: %w", err)
	}
	ic.log.Info("ice: agent created")

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
	ic.log.Info("ice: local credentials sent, awaiting remote")

	// Receive remote credentials
	var remoteCreds iceCredentials
	if err := ic.signaling.receiveDataCtx(ctx, &remoteCreds); err != nil {
		agent.Close()
		return nil, nil, fmt.Errorf("ice: receive credentials: %w", err)
	}
	ic.log.Info("ice: remote credentials received")

	// Register OnCandidate callback
	var localCandCount int
	if err := agent.OnCandidate(func(c pionIce.Candidate) {
		if c == nil {
			ic.log.Info("ice: local gathering complete", "local_candidates", localCandCount)
			ic.signaling.sendData(ic.peerID, iceNewCandidate{}) //nolint:errcheck
			return
		}
		localCandCount++
		ic.log.Info("ice: local candidate", "type", c.Type().String(), "n", localCandCount)
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
	ic.log.Info("ice: gathering started")

	// Receive remote candidates in background (terminates on ctx cancel or signaling close)
	go func() {
		var remoteCandCount int
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			var candidate iceNewCandidate
			if err := ic.signaling.receiveData(&candidate); err != nil {
				ic.log.Info("ice: remote candidate stream ended", "err", err, "remote_candidates", remoteCandCount)
				return
			}
			if candidate.Candidate == "" {
				ic.log.Info("ice: remote gathering complete", "remote_candidates", remoteCandCount)
				return
			}
			c, err := pionIce.UnmarshalCandidate(candidate.Candidate)
			if err != nil {
				ic.log.Warn("failed to unmarshal remote candidate", "err", err)
				continue
			}
			remoteCandCount++
			ic.log.Info("ice: remote candidate", "type", c.Type().String(), "n", remoteCandCount)
			agent.AddRemoteCandidate(c) //nolint:errcheck
		}
	}()

	// Connect — controlling side dials, controlled side accepts
	ic.log.Info("ice: starting connectivity check", "controlling", isControlling)
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

type pionSlogLoggerFactory struct {
	logger *slog.Logger
}

func (f pionSlogLoggerFactory) NewLogger(scope string) logging.LeveledLogger {
	logger := f.logger
	if logger == nil {
		logger = slog.Default()
	}
	return pionSlogLogger{logger: logger.With("pion_scope", scope)}
}

type pionSlogLogger struct {
	logger *slog.Logger
}

func (l pionSlogLogger) Trace(string) {}

func (l pionSlogLogger) Tracef(string, ...any) {}

func (l pionSlogLogger) Debug(string) {}

func (l pionSlogLogger) Debugf(string, ...any) {}

func (l pionSlogLogger) Info(msg string) {
	l.logger.Info("pion: " + msg)
}

func (l pionSlogLogger) Infof(format string, args ...any) {
	l.logger.Info("pion: " + fmt.Sprintf(format, args...))
}

func (l pionSlogLogger) Warn(msg string) {
	l.logger.Warn("pion: " + msg)
}

func (l pionSlogLogger) Warnf(format string, args ...any) {
	l.logger.Warn("pion: " + fmt.Sprintf(format, args...))
}

func (l pionSlogLogger) Error(msg string) {
	l.logger.Error("pion: " + msg)
}

func (l pionSlogLogger) Errorf(format string, args ...any) {
	l.logger.Error("pion: " + fmt.Sprintf(format, args...))
}
