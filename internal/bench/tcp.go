package bench

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// TCP header estimate: IPv4 20 + TCP 20 + timestamps 12.
const (
	tcpHeader = 52
	tcpMSS    = 1448
)

// tcpWire estimates bytes on the wire for one exchange: data segments in both directions
// with headers, plus pure ACKs (one per two segments, delayed ACK).
func tcpWire(up, down int64) int64 {
	segs := func(n int64) int64 { return (n + tcpMSS - 1) / tcpMSS }
	su, sd := segs(up), segs(down)
	acks := (su+1)/2 + (sd+1)/2
	return up + down + tcpHeader*(su+sd+acks)
}

// tcpHandshake is SYN, SYN-ACK, ACK on the wire.
const tcpHandshake = 64 + 64 + 52

// emulated link between a client and a server: a TCP proxy that delays and paces every chunk
// per direction without serialising them (pipelined like a real link).
func linkProxy(target string, p Profile) (addr string, stop func(), err error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	pipe := func(dst, src net.Conn) {
		type chunk struct {
			b   []byte
			due time.Time
		}
		q := make(chan chunk, 4096)
		go func() {
			var free time.Time
			buf := make([]byte, 32<<10)
			for {
				n, err := src.Read(buf)
				if n > 0 {
					now := time.Now()
					due := now
					if p.Rate > 0 {
						if free.Before(now) {
							free = now
						}
						free = free.Add(time.Duration(n) * time.Second / time.Duration(p.Rate))
						due = free
					}
					q <- chunk{append([]byte(nil), buf[:n]...), due.Add(p.Delay)}
				}
				if err != nil {
					close(q)
					return
				}
			}
		}()
		for c := range q {
			time.Sleep(time.Until(c.due))
			if _, err := dst.Write(c.b); err != nil {
				break
			}
		}
		dst.Close()
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			s, err := net.Dial("tcp", target)
			if err != nil {
				c.Close()
				continue
			}
			go pipe(s, c)
			go pipe(c, s)
		}
	}()
	return ln.Addr().String(), func() { ln.Close() }, nil
}

// counting wraps a connection and counts bytes both ways (TLS payload above TCP).
type counting struct {
	net.Conn
	in, out *atomic.Int64
}

func (c counting) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	c.in.Add(int64(n))
	return n, err
}

func (c counting) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	c.out.Add(int64(n))
	return n, err
}

func selfSigned() tls.Certificate {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	der, _ := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

const chatID = "62b7b92a-252b-43a2-971a-ec011b3c299c"

var bearer = strings.Repeat("3f9a1c7e", 4)

// --- HTTPS + JSON (HTTP/1.1 keep-alive, as MCP/A2A-style APIs) ---

// RunHTTPS measures POST /chats/<id>/messages {"text": …} → {"ok":true} over TLS 1.3.
func RunHTTPS(p Profile, n int) (*Result, error) {
	res := newResult("HTTPS+JSON", p.Name)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{selfSigned()}, NextProtos: []string{"http/1.1"}})
	if err != nil {
		return nil, err
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})}
	go srv.Serve(ln)
	defer srv.Close()
	addr, stop, err := linkProxy(ln.Addr().String(), p)
	if err != nil {
		return nil, err
	}
	defer stop()

	var in, out atomic.Int64
	client := func() *http.Client {
		return &http.Client{Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, a string) (net.Conn, error) {
				c, err := (&net.Dialer{}).DialContext(ctx, network, a)
				if err != nil {
					return nil, err
				}
				return counting{c, &in, &out}, nil
			},
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: true, ClientSessionCache: nil},
			ForceAttemptHTTP2: false,
			TLSNextProto:      map[string]func(string, *tls.Conn) http.RoundTripper{},
		}}
	}
	send := func(c *http.Client, text string) error {
		body, _ := json.Marshal(map[string]string{"text": text})
		req, _ := http.NewRequest("POST", "https://"+addr+"/chats/"+chatID+"/messages", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+bearer)
		resp, err := c.Do(req)
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("status %d", resp.StatusCode)
		}
		return nil
	}

	// cold: a fresh connection with a full TLS handshake
	cold := client()
	start := time.Now()
	if err := send(cold, Message(Sizes[0], 1)); err != nil {
		return nil, err
	}
	res.ColdTime = time.Since(start)
	res.ColdBytes = tcpHandshake + tcpWire(out.Load(), in.Load()) + 3*tcpHeader // + TLS flights' ACKs
	cold.CloseIdleConnections()

	warm := client()
	if err := send(warm, "warm up"); err != nil {
		return nil, err
	}
	for _, s := range Sizes {
		var rtts []time.Duration
		in0, out0 := in.Load(), out.Load()
		cnt := count(n, s)
		for i := 0; i < cnt; i++ {
			t := time.Now()
			if err := send(warm, Message(s, i)); err != nil {
				return nil, err
			}
			rtts = append(rtts, time.Since(t))
		}
		up, down := (out.Load()-out0)/int64(cnt), (in.Load()-in0)/int64(cnt)
		res.Payload[s.Name] = up + down
		res.Wire[s.Name] = tcpWire(up, down)
		res.RTT[s.Name] = median(rtts)
	}
	if p.Loss > 0 {
		res.Note = "no packet loss emulated for TCP (it would need root); real numbers would be worse"
	}
	return res, nil
}

// count — fewer repetitions for big messages.
func count(n int, s Size) int {
	if s.Bytes >= 50<<10 {
		return max(1, n/10)
	}
	return n
}

// --- WebSocket: JSON frames over TLS (as chat apps do) ---

// RunWebSocket measures {"type":"send","chat":…,"text":…} → {"type":"ok"} as WebSocket frames
// over TLS 1.3, after a real HTTP upgrade.
func RunWebSocket(p Profile, n int) (*Result, error) {
	res := newResult("WebSocket", p.Name)
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{Certificates: []tls.Certificate{selfSigned()}})
	if err != nil {
		return nil, err
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				r := bufio.NewReader(c)
				for { // upgrade request headers
					line, err := r.ReadString('\n')
					if err != nil {
						return
					}
					if line == "\r\n" {
						break
					}
				}
				_, _ = io.WriteString(c, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"+
					"Sec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=\r\n\r\n")
				for {
					if _, err := readFrame(r); err != nil {
						return
					}
					if _, err := c.Write(frame([]byte(`{"type":"ok"}`), false)); err != nil {
						return
					}
				}
			}()
		}
	}()
	addr, stop, err := linkProxy(ln.Addr().String(), p)
	if err != nil {
		return nil, err
	}
	defer stop()

	var in, out atomic.Int64
	dial := func() (net.Conn, *bufio.Reader, error) {
		raw, err := net.Dial("tcp", addr)
		if err != nil {
			return nil, nil, err
		}
		c := tls.Client(counting{raw, &in, &out}, &tls.Config{InsecureSkipVerify: true})
		_, err = io.WriteString(c, "GET /chat HTTP/1.1\r\nHost: "+addr+"\r\nUpgrade: websocket\r\nConnection: Upgrade\r\n"+
			"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\nSec-WebSocket-Version: 13\r\nAuthorization: Bearer "+bearer+"\r\n\r\n")
		if err != nil {
			return nil, nil, err
		}
		r := bufio.NewReader(c)
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return nil, nil, err
			}
			if line == "\r\n" {
				return c, r, nil
			}
		}
	}
	send := func(c net.Conn, r *bufio.Reader, text string) error {
		msg, _ := json.Marshal(map[string]string{"type": "send", "chat": chatID, "text": text})
		if _, err := c.Write(frame(msg, true)); err != nil {
			return err
		}
		_, err := readFrame(r)
		return err
	}

	start := time.Now()
	c, r, err := dial()
	if err != nil {
		return nil, err
	}
	if err := send(c, r, Message(Sizes[0], 1)); err != nil {
		return nil, err
	}
	res.ColdTime = time.Since(start)
	res.ColdBytes = tcpHandshake + tcpWire(out.Load(), in.Load()) + 3*tcpHeader

	for _, s := range Sizes {
		var rtts []time.Duration
		in0, out0 := in.Load(), out.Load()
		cnt := count(n, s)
		for i := 0; i < cnt; i++ {
			t := time.Now()
			if err := send(c, r, Message(s, i)); err != nil {
				return nil, err
			}
			rtts = append(rtts, time.Since(t))
		}
		up, down := (out.Load()-out0)/int64(cnt), (in.Load()-in0)/int64(cnt)
		res.Payload[s.Name] = up + down
		res.Wire[s.Name] = tcpWire(up, down)
		res.RTT[s.Name] = median(rtts)
	}
	c.Close()
	if p.Loss > 0 {
		res.Note = "no packet loss emulated for TCP (it would need root); real numbers would be worse"
	}
	return res, nil
}

// frame is a WebSocket text frame (RFC 6455); clients mask.
func frame(payload []byte, masked bool) []byte {
	b := []byte{0x81}
	m := byte(0)
	if masked {
		m = 0x80
	}
	switch n := len(payload); {
	case n < 126:
		b = append(b, m|byte(n))
	case n < 1<<16:
		b = append(b, m|126)
		b = binary.BigEndian.AppendUint16(b, uint16(n))
	default:
		b = append(b, m|127)
		b = binary.BigEndian.AppendUint64(b, uint64(n))
	}
	if !masked {
		return append(b, payload...)
	}
	key := []byte{0x12, 0x34, 0x56, 0x78}
	b = append(b, key...)
	for i, x := range payload {
		b = append(b, x^key[i%4])
	}
	return b
}

func readFrame(r *bufio.Reader) ([]byte, error) {
	var h [2]byte
	if _, err := io.ReadFull(r, h[:]); err != nil {
		return nil, err
	}
	n := uint64(h[1] & 0x7f)
	switch n {
	case 126:
		var x [2]byte
		if _, err := io.ReadFull(r, x[:]); err != nil {
			return nil, err
		}
		n = uint64(binary.BigEndian.Uint16(x[:]))
	case 127:
		var x [8]byte
		if _, err := io.ReadFull(r, x[:]); err != nil {
			return nil, err
		}
		n = binary.BigEndian.Uint64(x[:])
	}
	if n > 64<<20 {
		return nil, errors.New("frame too big")
	}
	var key [4]byte
	if h[1]&0x80 != 0 {
		if _, err := io.ReadFull(r, key[:]); err != nil {
			return nil, err
		}
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return nil, err
	}
	if h[1]&0x80 != 0 {
		for i := range b {
			b[i] ^= key[i%4]
		}
	}
	return b, nil
}
