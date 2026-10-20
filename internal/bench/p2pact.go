package bench

import (
	"context"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/p2pact/p2pact"
)

// RunP2PACT measures a chat message as a P2PACT call [0, id, 1 chat, 2 send, chat, text] → [1, id]
// over UDP with the profile emulated on both sides.
func RunP2PACT(p Profile, n int, compress bool) (*Result, error) {
	name := "P2PACT"
	if !compress {
		name = "P2PACT (no zstd)"
	}
	res := newResult(name, p.Name)
	dirA, _ := os.MkdirTemp("", "p2pact-bench-a")
	dirB, _ := os.MkdirTemp("", "p2pact-bench-b")
	defer os.RemoveAll(dirA)
	defer os.RemoveAll(dirB)

	start := func(dir, name, addr string) (*p2pact.Node, error) {
		nd, err := p2pact.NewNode(dir, name)
		if err != nil {
			return nil, err
		}
		if err := nd.ListenUDP(addr); err != nil {
			return nil, err
		}
		nd.NoCompression = !compress
		nd.Handle(1, 2, func(string, []cbor.RawMessage) ([]any, error) { return []any{}, nil })
		nd.Start()
		return nd, nil
	}
	var rngMu sync.Mutex
	rng := rand.New(rand.NewSource(7))
	emulate := func(nd *p2pact.Node) {
		u := nd.UDP()
		u.Delay, u.Rate = p.Delay, p.Rate
		if p.Loss > 0 {
			// Drop runs on both send and receive, so each side gets half the loss
			u.Drop = func() bool { rngMu.Lock(); defer rngMu.Unlock(); return rng.Float64() < p.Loss/2 }
		}
	}

	srv, err := start(dirA, "Laptop", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	srvAddr := srv.UDP().LocalAddr()
	cli, err := start(dirB, "Phone", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	inv, _ := p2pact.ParseInvite(srv.Invite(time.Minute).String())
	peer, err := cli.Join(ctx, inv)
	if err != nil {
		return nil, err
	}
	for i := 0; i < 200 && srv.Trust.Get(cli.ID.Addr()) == nil; i++ {
		time.Sleep(10 * time.Millisecond)
	}
	// cold: both restart with the same keys — paired, but no session
	srv.Close()
	cli.Close()
	if srv, err = start(dirA, "Laptop", srvAddr); err != nil {
		return nil, err
	}
	defer srv.Close()
	if cli, err = start(dirB, "Phone", "127.0.0.1:0"); err != nil {
		return nil, err
	}
	defer cli.Close()
	emulate(srv)
	emulate(cli)
	sent := func() (int64, int64) {
		a, b := srv.UDP().Stats(), cli.UDP().Stats()
		return a.SentBytes + b.SentBytes, a.SentPackets + b.SentPackets
	}
	send := func(text string) error {
		_, err := cli.Call(ctx, peer.Addr, 1, 2, 1, text)
		return err
	}
	b0, p0 := sent()
	t := time.Now()
	if err := send(Text(Sizes[0].Bytes, 1)); err != nil {
		return nil, err
	}
	res.ColdTime = time.Since(t)
	time.Sleep(3*p.Delay + 100*time.Millisecond) // let the last ack land
	b1, p1 := sent()
	res.ColdBytes = b1 - b0 + 28*(p1-p0)

	for _, s := range Sizes {
		var rtts []time.Duration
		cnt := count(n, s)
		b0, p0 := sent()
		for i := 0; i < cnt; i++ {
			t := time.Now()
			if err := send(Text(s.Bytes, int64(i))); err != nil {
				return nil, err
			}
			rtts = append(rtts, time.Since(t))
		}
		time.Sleep(3*p.Delay + 100*time.Millisecond)
		b1, p1 := sent()
		res.Payload[s.Name] = (b1 - b0) / int64(cnt)
		res.Wire[s.Name] = (b1 - b0 + 28*(p1-p0)) / int64(cnt)
		res.RTT[s.Name] = median(rtts)
	}
	return res, nil
}

// Run measures every protocol over every profile.
func Run(profiles []Profile, n int, progress func(string)) ([]*Result, error) {
	var out []*Result
	for _, p := range profiles {
		for _, f := range []func() (*Result, error){
			func() (*Result, error) { return RunP2PACT(p, n, true) },
			func() (*Result, error) { return RunWebSocket(p, n) },
			func() (*Result, error) { return RunHTTPS(p, n) },
		} {
			r, err := f()
			if err != nil {
				return nil, err
			}
			if progress != nil {
				progress(r.Protocol + " · " + r.Profile)
			}
			out = append(out, r)
		}
	}
	return out, nil
}
