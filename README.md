# P2PACT — P2P Agent Chat Protocol

**Chat with your AI agents — and let them chat with each other — across your own devices.
No servers, no accounts, over any connection.**

Your coding agents run on a laptop, a desktop, a home server; you follow and answer them from
a phone. Today that means a cloud relay, an account, a tunnel, or a port opened to the
internet. P2PACT connects your devices directly and keeps the chat going:

- **No servers, no accounts.** Devices pair once with a QR code and recognise each other by
  their keys. There is nothing to sign up for and nobody in the middle.
- **Any road.** Direct on the local network, across the internet, through a relay that cannot
  read anything, over Bluetooth *(planned)*. One session uses all roads at once, so switching
  from Wi-Fi to a hotspot does not drop the chat.
- **Both sides speak.** Not request–response: an agent's message, question or "done" reaches
  you the moment it happens.
- **The swarm survives.** A family of devices with a parent and deputies: if a computer goes
  away, another takes over its duties (`spec/family.md`).
- **Small and fast.** Binary CBOR messages with numbers instead of names, zstd with
  dictionaries tuned for agent chats, and data in the very first packet.

The chat format follows ACP (Agent Client Protocol), so existing ACP clients and agents fit;
P2PACT is what carries it between devices.

## How small

Measured by `go test -v -run Compare` (packet sizes on the wire, without IP/UDP/TCP headers):

| Message | P2PACT | JSON over TLS | JSON-RPC over HTTPS |
|---|---|---|---|
| keepalive | 26 B | 56 B | 296 B |
| new chat message "ok" | 40 B | 137 B | 382 B |
| human reply to a chat | 45 B | 147 B | 389 B |
| history search | 41 B | 84 B | 331 B |

- A chat list of 12 chats polled every 1.5 s costs 171 KB a minute; with P2PACT's change
  events it is about 244 bytes a minute.
- First answer on a new connection with 50 ms one-way latency: 208 ms for TLS over TCP,
  139 ms for P2PACT (the request rides in the first handshake packet).
- Network change mid-chat: zero new handshakes; the session continues.
- Dictionary compression on real agent chats: 2–2.5× for human messages, about 3.5× for
  agent replies (zstd alone: 1.4× and 2.1×).

## Layers

1. **Roads** — UDP, TCP, a relay; Bluetooth, via a third device *(planned)*.
2. **Session** — Noise (`IK` for known devices, `XXpsk2` for pairing), X25519,
   ChaCha20-Poly1305, BLAKE2s. 25 bytes of overhead per packet.
3. **Delivery** — piggybacked acks, retransmission over the next live road, long messages in
   parts, compression with dictionaries.
4. **Messages** — `call`, `result`, `error`, `event` as CBOR arrays.
5. **Tools** — `1 chat`, `3 family`, and your own (64+).

The full spec: [`spec/core.md`](spec/core.md) and [`spec/family.md`](spec/family.md).

## How it compares

- **ACP** defines how a client talks to an agent, inside one machine. P2PACT takes that chat
  across devices.
- **MCP** connects an agent to tools, **A2A** delegates tasks between agents of different
  services over HTTP. Neither is a chat between your own devices.
- **ANP (Agent Network Protocol)** connects agents of different owners across the open web
  with DIDs and domains. P2PACT connects the devices of one owner, without domains or servers.
- **WireGuard, iroh, libp2p** are excellent pipes and P2PACT borrows from them (WireGuard's
  crypto choices in particular), but they know nothing about agents or chats.

## Status

Draft, v1. The reference implementation in Go covers pairing, sessions, UDP, TCP and relay roads,
delivery, messages and dictionary compression. Next: the chat tool spec, the family tool,
Bluetooth and NAT hole punching.

```go
a, _ := p2pact.NewNode(dirA, "Laptop")
_ = a.ListenUDP(":48730")
a.Handle(64, 1, func(peer string, args []cbor.RawMessage) ([]any, error) {
	return []any{"pong"}, nil
})
a.Start()

invite := a.Invite(10 * time.Minute).String() // p2pact1.… — show it as a QR code

b, _ := p2pact.NewNode(dirB, "Phone")
_ = b.ListenUDP(":0")
b.Start()
inv, _ := p2pact.ParseInvite(invite)
peer, _ := b.Join(ctx, inv)
res, _ := b.Call(ctx, peer.Addr, 64, 1)
```

Relay: `go run ./cmd/p2pact-relay -addr :8443 -data ./relay-data` prints its certificate
fingerprint; give devices the address and the fingerprint.

## Contributing

Issues and pull requests are welcome: the spec, the Go implementation, implementations in
other languages, interop tests.

## License

[Apache License 2.0](LICENSE).
