package maxclient

import (
	"encoding/json"
	"sync"
	"testing"
)

func TestSeqCounterIncrement(t *testing.T) {
	sc := &seqCounter{}
	if got := sc.Next(); got != 1 {
		t.Errorf("first = %d, want 1", got)
	}
	if got := sc.Next(); got != 2 {
		t.Errorf("second = %d, want 2", got)
	}
}

func TestSeqCounterConcurrency(t *testing.T) {
	sc := &seqCounter{}
	var wg sync.WaitGroup
	n := 1000
	seen := make(map[int]bool)
	var mu sync.Mutex
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v := sc.Next()
			mu.Lock()
			seen[v] = true
			mu.Unlock()
		}()
	}
	wg.Wait()
	if len(seen) != n {
		t.Errorf("got %d unique seqs, want %d", len(seen), n)
	}
}

func TestBuildRequest(t *testing.T) {
	sc := &seqCounter{}
	payload := map[string]any{"interactive": false}
	data, seq, err := buildRequest(sc, OpcodeKeepalive, payload)
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}

	if seq != 1 {
		t.Errorf("seq = %d, want 1", seq)
	}

	var pkt Packet
	if err := json.Unmarshal(data, &pkt); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if pkt.Ver != RPCVersion {
		t.Errorf("ver = %d, want %d", pkt.Ver, RPCVersion)
	}
	if pkt.Cmd != 0 {
		t.Errorf("cmd = %d, want 0", pkt.Cmd)
	}
	if pkt.Opcode != OpcodeKeepalive {
		t.Errorf("opcode = %d, want %d", pkt.Opcode, OpcodeKeepalive)
	}
}

func TestOpcodeConstants(t *testing.T) {
	if OpcodeMessageEvent != 128 {
		t.Errorf("OpcodeMessageEvent = %d, want 128", OpcodeMessageEvent)
	}
	if OpcodeSendMessage != 64 {
		t.Errorf("OpcodeSendMessage = %d, want 64", OpcodeSendMessage)
	}
	if OpcodeDeleteMessage != 66 {
		t.Errorf("OpcodeDeleteMessage = %d, want 66", OpcodeDeleteMessage)
	}
}
