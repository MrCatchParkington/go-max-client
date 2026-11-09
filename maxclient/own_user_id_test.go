package maxclient

import "testing"

func TestOwnUserID(t *testing.T) {
    client := New()

    if got := client.OwnUserID(); got != 0 {
        t.Fatalf("OwnUserID() before authentication = %d, want 0", got)
    }

    client.ownUserID = 111444795

    if got := client.OwnUserID(); got != 111444795 {
        t.Fatalf("OwnUserID() = %d, want 111444795", got)
    }
}