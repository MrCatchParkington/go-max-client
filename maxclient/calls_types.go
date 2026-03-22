package maxclient

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/pierrec/lz4/v4"
)

// Call API constants
const (
	CallsAPIURL              = "https://calls.okcdn.ru/fb.do"
	CallsAppKey              = "CNHIJPLGDIHBABABA"
	CallsClientType          = "SDK_JS"
	CallsClientVersion       = "1.1"
	CallsProtocolVersion     = "5"
	CallsCapabilities        = "603F"
	CallsDeviceType          = "browser"
	CallsPlatform            = "WEB"
	CallsClientTypeSignaling = "ONE_ME"
)

// --- OneMe types ---

type callTokenResponse struct {
	Token string `json:"token"`
}

type incomingCallPayload struct {
	Vcp            string `json:"vcp"`
	CallerID       int64  `json:"callerId"`
	ConversationID string `json:"conversationId"`
}

type vcpDecoded struct {
	SignalingToken  string `json:"tkn"`
	SignalingServer string `json:"wse"`
	StunServer      string `json:"stne"`
	TurnServers     string `json:"trne"`
	TurnUser        string `json:"trnu"`
	TurnPassword    string `json:"trnp"`
}

type fastStartRequest struct {
	ConversationID string  `json:"conversationId"`
	CalleeIDs      []int64 `json:"calleeIds"`
	InternalParams string  `json:"internalParams"`
	IsVideo        bool    `json:"isVideo"`
}

type fastStartInternalParams struct {
	DeviceID        string `json:"deviceId"`
	SDKVersion      string `json:"sdkVersion"`
	ClientAppKey    string `json:"clientAppKey"`
	Platform        string `json:"platform"`
	ProtocolVersion int    `json:"protocolVersion"`
	DomainID        string `json:"domainId"`
	Capabilities    string `json:"capabilities"`
}

type fastStartResponse struct {
	InternalCallerParams string `json:"internalCallerParams"`
}

// --- Calls HTTP API types ---

type callsSessionData struct {
	Token         string `json:"auth_token"`
	ClientType    string `json:"client_type"`
	ClientVersion string `json:"client_version"`
	DeviceID      string `json:"device_id"`
	Version       int    `json:"version"`
}

type callsLoginResponse struct {
	UID              string `json:"uid"`
	SessionKey       string `json:"session_key"`
	SessionSecretKey string `json:"session_secret_key"`
	APIServer        string `json:"api_server"`
	ExternalUserID   string `json:"external_user_id"`
}

type startConversationResponse struct {
	TurnServer struct {
		Urls     []string `json:"urls"`
		Username string   `json:"username"`
		Password string   `json:"credential"`
	} `json:"turn_server"`
	StunServer struct {
		Urls []string `json:"urls"`
	} `json:"stun_server"`
	Endpoint string `json:"endpoint"`
}

// --- Signaling types ---

type signalingServerHello struct {
	Conversation struct {
		Participants []signalingParticipant `json:"participants"`
	} `json:"conversation"`
}

type signalingParticipant struct {
	ExternalID struct {
		ID string `json:"id"`
	} `json:"externalId"`
	ID int64 `json:"id"`
}

type signalingTransmitData struct {
	Command         string `json:"command"`
	Sequence        int    `json:"sequence"`
	ParticipantID   int64  `json:"participantId"`
	Data            any    `json:"data"`
	ParticipantType string `json:"participantType"`
}

type signalingNotification struct {
	Type         string          `json:"type"`
	Notification string          `json:"notification"`
	Data         json.RawMessage `json:"data"`
}

type signalingAcceptCall struct {
	Command       string                 `json:"command"`
	Sequence      int                    `json:"sequence"`
	MediaSettings signalingMediaSettings `json:"mediaSettings"`
}

type signalingMediaSettings struct {
	IsAudioEnabled             bool `json:"isAudioEnabled"`
	IsVideoEnabled             bool `json:"isVideoEnabled"`
	IsScreenSharingEnabled     bool `json:"isScreenSharingEnabled"`
	IsFastScreenSharingEnabled bool `json:"isFastScreenSharingEnabled"`
	IsAudioSharingEnabled      bool `json:"isAudioSharingEnabled"`
	IsAnimojiEnabled           bool `json:"isAnimojiEnabled"`
}

// --- ICE types ---

type iceCredentials struct {
	UFrag    string `json:"ufrag"`
	Password string `json:"password"`
}

type iceNewCandidate struct {
	Candidate string `json:"candidate"`
}

// --- Public types ---

// CallSession represents an established call with a bidirectional data stream.
type CallSession struct {
	conn    interface{ Read([]byte) (int, error); Write([]byte) (int, error); Close() error }
	closeFn func() error // cleanup signaling WS
	agent   io.Closer    // ICE agent
}

func (s *CallSession) Read(b []byte) (int, error)  { return s.conn.Read(b) }
func (s *CallSession) Write(b []byte) (int, error) { return s.conn.Write(b) }
func (s *CallSession) Close() error {
	connErr := s.conn.Close()
	if s.agent != nil {
		if err := s.agent.Close(); err != nil && connErr == nil {
			connErr = err
		}
	}
	if s.closeFn != nil {
		if err := s.closeFn(); err != nil && connErr == nil {
			connErr = err
		}
	}
	return connErr
}

func decodeVCP(vcp string) (*vcpDecoded, error) {
	parts := strings.SplitN(vcp, ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("calls: invalid VCP format")
	}
	uncompressedSize, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, fmt.Errorf("calls: invalid VCP size: %w", err)
	}
	const maxVCPSize = 1 << 20 // 1 MB
	if uncompressedSize <= 0 || uncompressedSize > maxVCPSize {
		return nil, fmt.Errorf("calls: VCP size out of range: %d", uncompressedSize)
	}
	compressed, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("calls: VCP base64 decode: %w", err)
	}
	decompressed := make([]byte, uncompressedSize)
	n, err := lz4.UncompressBlock(compressed, decompressed)
	if err != nil {
		return nil, fmt.Errorf("calls: VCP lz4 decompress: %w", err)
	}
	var decoded vcpDecoded
	if err := json.Unmarshal(decompressed[:n], &decoded); err != nil {
		return nil, fmt.Errorf("calls: VCP JSON unmarshal: %w", err)
	}
	return &decoded, nil
}
