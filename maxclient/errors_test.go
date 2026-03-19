package maxclient

import (
	"errors"
	"testing"
)

func TestSentinelErrors(t *testing.T) {
	// Verify errors are distinct and matchable with errors.Is
	errs := []error{
		ErrDisconnected, ErrReconnecting, ErrReconnected,
		ErrAuthRequired, ErrQRExpired, ErrNotConnected,
	}
	for i, e1 := range errs {
		for j, e2 := range errs {
			if i != j && errors.Is(e1, e2) {
				t.Errorf("errors should be distinct: %v == %v", e1, e2)
			}
		}
	}
}
