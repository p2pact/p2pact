package bench

import (
	"testing"
)

// Sizes on a local network: P2PACT must be smaller than WebSocket and HTTPS for every message
// size, and for a new connection. `go run ./cmd/p2pact-bench` prints the full report.
func TestSizesLocal(t *testing.T) {
	if testing.Short() {
		t.Skip("benchmark")
	}
	res, err := Run(Profiles[:1], 3, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("\n" + Markdown(res))
	ours := res[0]
	for _, other := range res[1:] {
		for _, s := range Sizes {
			if ours.Wire[s.Name] >= other.Wire[s.Name] {
				t.Errorf("%s: P2PACT %d B is not smaller than %s %d B", s.Name, ours.Wire[s.Name], other.Protocol, other.Wire[s.Name])
			}
		}
		if ours.ColdBytes >= other.ColdBytes {
			t.Errorf("new connection: P2PACT %d B is not smaller than %s %d B", ours.ColdBytes, other.Protocol, other.ColdBytes)
		}
	}
}

func TestTextDeterministic(t *testing.T) {
	for _, s := range Sizes {
		a, b := Text(s.Bytes, 3), Text(s.Bytes, 3)
		if a != b || len(a) != s.Bytes {
			t.Fatalf("%s: %d bytes, deterministic %v", s.Name, len(a), a == b)
		}
	}
}
