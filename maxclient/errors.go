package maxclient

import "errors"

var (
	ErrDisconnected = errors.New("maxclient: disconnected")
	ErrReconnecting = errors.New("maxclient: reconnecting")
	ErrReconnected  = errors.New("maxclient: reconnected")
	ErrAuthRequired = errors.New("maxclient: authentication required")
	ErrQRExpired    = errors.New("maxclient: QR code expired")
	ErrNotConnected = errors.New("maxclient: not connected")
)
