package bench

import (
	"context"
	"errors"
	"math/rand"
	"os"
	"sync"
	"time"

	"github.com/fxamacker/cbor/v2"
	"github.com/p2pact/p2pact"
)

// RunP2PACT measures a chat message as a P2PACT call [0, id, 1 chat, 2 send, chat, text] → [1, id]
// over one road ("udp" or "tcp") with the profile emulated on both sides.
func RunP2PACT(p Profile, n int, road string) (*Result, error) {
	res := newResult("P2PACT "+map[string]string{"udp": "UDP", "tcp": "TCP"}[road], p.Name)
	dirA, _ := os.MkdirTemp("", "p2pact-bench-a")
	dirB, _ := os.MkdirTemp("", "p2pact-bench-b")
	defer os.RemoveAll(dirA)
	defer os.RemoveAll(dirB)

	start := func(dir, name, addr string, listen bool) (*p2pact.Node, error) {
		nd, err := p2pact.NewNode(dir, name)
		if err != nil {
			return nil, err
		}
		if road == "udp" {
			err = nd.ListenUDP(addr)
		} else {
			if !listen {
				addr = ""
			}
			err = nd.ListenTCP(addr)
		}
		if err != nil {
			return nil, err
		}
		nd.Handle(1, 2, func(string, []cbor.RawMessage) ([]any, error) { return []any{}, nil })
		nd.Start()
		return nd, nil
	}
	local := func(nd *p2pact.Node) string {
		if road == "udp" {
			return nd.UDP().LocalAddr()
		}
		return nd.TCP().LocalAddr()
	}
	stats := func(nd *p2pact.Node) p2pact.RoadStats {
		if road == "udp" {
			return nd.UDP().Stats()
		}
		return nd.TCP().Stats()
	}
	var rngMu sync.Mutex
	rng := rand.New(rand.NewSource(7))
	emulate := func(nd *p2pact.Node) {
		if road == "tcp" {
			t := nd.TCP()
			t.Delay, t.Rate = p.Delay, p.Rate
			return
		}
		u := nd.UDP()
		u.Delay, u.Rate = p.Delay, p.Rate
		if p.Loss > 0 {
			// Drop runs on both send and receive, so each side gets half the loss
			u.Drop = func() bool { rngMu.Lock(); defer rngMu.Unlock(); return rng.Float64() < p.Loss/2 }
		}
	}

	srv, err := start(dirA, "Laptop", "127.0.0.1:0", true)
	if err != nil {
		return nil, err
	}
	srvAddr := local(srv)
	cli, err := start(dirB, "Phone", "127.0.0.1:0", false)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
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
	if srv, err = start(dirA, "Laptop", srvAddr, true); err != nil {
		return nil, err
	}
	defer srv.Close()
	if cli, err = start(dirB, "Phone", "127.0.0.1:0", false); err != nil {
		return nil, err
	}
	defer cli.Close()
	emulate(srv)
	emulate(cli)
	// up — client → server, down — server → client
	snap := func() (up, down, pkts int64) {
		c, s := stats(cli), stats(srv)
		return c.SentBytes, s.SentBytes, c.SentPackets + s.SentPackets
	}
	wire := func(up, down, pkts int64) int64 {
		if road == "udp" {
			return up + down + 28*pkts
		}
		return tcpWire(up, down)
	}
	send := func(text string) error {
		_, err := cli.Call(ctx, peer.Addr, 1, 2, 1, text)
		return err
	}
	settle := func() { time.Sleep(3*p.Delay + 100*time.Millisecond) } // let the last ack land

	u0, d0, p0 := snap()
	t := time.Now()
	if err := send(Message(Sizes[0], 1)); err != nil {
		return nil, err
	}
	res.ColdTime = time.Since(t)
	settle()
	u1, d1, p1 := snap()
	res.ColdBytes = wire(u1-u0, d1-d0, p1-p0)
	if road == "tcp" {
		res.ColdBytes += tcpHandshake
	}

	for _, s := range Sizes {
		var rtts []time.Duration
		cnt := count(n, s)
		u0, d0, p0 := snap()
		for i := 0; i < cnt; i++ {
			t := time.Now()
			if err := send(Message(s, i)); err != nil {
				return nil, err
			}
			rtts = append(rtts, time.Since(t))
		}
		settle()
		u1, d1, p1 := snap()
		c := int64(cnt)
		res.Payload[s.Name] = (u1 - u0 + d1 - d0) / c
		res.Wire[s.Name] = wire((u1-u0)/c, (d1-d0)/c, (p1-p0)/c)
		res.RTT[s.Name] = median(rtts)
	}
	if road == "tcp" && p.Loss > 0 {
		res.Note = "no packet loss emulated for TCP (it would need root); real numbers would be worse"
	}
	return res, nil
}

// Run measures every protocol over every profile.
func Run(profiles []Profile, n int, progress func(string)) ([]*Result, error) {
	var out []*Result
	for _, p := range profiles {
		for _, f := range []func() (*Result, error){
			func() (*Result, error) { return RunP2PACT(p, n, "udp") },
			func() (*Result, error) { return RunP2PACT(p, n, "tcp") },
			func() (*Result, error) { return RunWebSocket(p, n) },
			func() (*Result, error) { return RunHTTPS(p, n) },
		} {
			r, err := f()
			if err != nil {
				return nil, errors.Join(errors.New(p.Name), err)
			}
			if progress != nil {
				progress(r.Protocol + " · " + r.Profile)
			}
			out = append(out, r)
		}
	}
	return out, nil
}
