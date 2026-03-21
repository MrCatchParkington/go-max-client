package maxclient

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	gorillaWs "github.com/gorilla/websocket"
)

type signalingClient struct {
	conn      *gorillaWs.Conn
	seq       atomic.Int32
	log       *slog.Logger
	writeMu   sync.Mutex
	closeOnce sync.Once
	closeErr  error
	done      chan struct{}
}

func newSignalingClient(ctx context.Context, wsURL string, log *slog.Logger) (*signalingClient, error) {
	dialer := gorillaWs.Dialer{}
	header := make(map[string][]string)
	header["Origin"] = []string{DefaultOrigin}

	conn, _, err := dialer.DialContext(ctx, wsURL, header)
	if err != nil {
		return nil, fmt.Errorf("signaling: connect: %w", err)
	}
	sc := &signalingClient{conn: conn, log: log, done: make(chan struct{})}
	// No ctx-based cleanup goroutine: signaling lifetime is managed by
	// CallSession.Close() -> closeFn -> sig.close(), not by the setup context.
	// This prevents a timeout/cancel on the setup ctx from killing an active call.
	return sc, nil
}

func (sc *signalingClient) nextSeq() int {
	return int(sc.seq.Add(1))
}

func (sc *signalingClient) receiveServerHello() (*signalingServerHello, error) {
	sc.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	defer sc.conn.SetReadDeadline(time.Time{})
	for {
		_, msg, err := sc.conn.ReadMessage()
		if err != nil {
			return nil, fmt.Errorf("signaling: read hello: %w", err)
		}
		if string(msg) == "ping" {
			if err := sc.writePong(); err != nil {
				sc.log.Warn("signaling: failed to write pong", "err", err)
			}
			continue
		}
		var hello signalingServerHello
		if err := json.Unmarshal(msg, &hello); err != nil {
			sc.log.Debug("signaling: ignoring non-hello message", "err", err)
			continue
		}
		if len(hello.Conversation.Participants) > 0 {
			return &hello, nil
		}
	}
}

func (sc *signalingClient) sendAcceptCall() error {
	msg := signalingAcceptCall{
		Command:  "accept-call",
		Sequence: sc.nextSeq(),
		MediaSettings: signalingMediaSettings{
			IsAudioEnabled: true,
		},
	}
	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()
	return sc.conn.WriteJSON(msg)
}

func (sc *signalingClient) sendData(participantID int64, data any) error {
	msg := signalingTransmitData{
		Command:         "transmit-data",
		Sequence:        sc.nextSeq(),
		ParticipantID:   participantID,
		Data:            data,
		ParticipantType: "USER",
	}
	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()
	return sc.conn.WriteJSON(msg)
}

func (sc *signalingClient) receiveData(dst any) error {
	const readTimeout = 60 * time.Second
	for {
		sc.conn.SetReadDeadline(time.Now().Add(readTimeout))
		_, raw, err := sc.conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("signaling: read: %w", err)
		}
		if string(raw) == "ping" {
			if err := sc.writePong(); err != nil {
				sc.log.Warn("signaling: failed to write pong", "err", err)
			}
			continue
		}
		var notif signalingNotification
		if err := json.Unmarshal(raw, &notif); err != nil {
			continue
		}
		if notif.Type == "notification" && notif.Notification == "transmitted-data" {
			return json.Unmarshal(notif.Data, dst)
		}
	}
}

func (sc *signalingClient) writePong() error {
	sc.writeMu.Lock()
	defer sc.writeMu.Unlock()
	return sc.conn.WriteMessage(gorillaWs.TextMessage, []byte("pong"))
}

func (sc *signalingClient) close() error {
	sc.closeOnce.Do(func() {
		close(sc.done)
		sc.closeErr = sc.conn.Close()
	})
	return sc.closeErr
}
