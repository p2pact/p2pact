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
  away, another takes over its duties.
- **Small and fast.** Binary CBOR messages with numbers instead of names, zstd with
  dictionaries tuned for agent chats, data in the very first packet.

The chat format follows ACP (Agent Client Protocol), so existing ACP clients and agents fit;
P2PACT is what carries it between devices.

## This repository is the specification

- [`spec/core.md`](spec/core.md) — devices, pairing, sessions, roads, delivery, messages,
  compression.
- [`spec/family.md`](spec/family.md) — the family of devices: parent, deputies, the signed
  book, the family dictionary.

## Implementations

| Language | Repository | Status |
|---|---|---|
| Go (reference) | [p2pact/p2pact-go](https://github.com/p2pact/p2pact-go) | pairing, sessions, UDP/TCP/relay, delivery, compression |
| TypeScript (Node.js) | [p2pact/p2pact-ts](https://github.com/p2pact/p2pact-ts) | pairing, sessions, UDP/TCP, delivery, compression; interoperates with Go |
| Python | p2pact/p2pact-py | in progress |

Write your own from the spec — and check it with the interop tests below: every pair of
implementations pairs and exchanges messages both ways, over UDP and TCP.

## Tests and benchmarks

[p2pact/p2pact-tests](https://github.com/p2pact/p2pact-tests) compares P2PACT with WebSocket,
HTTPS, HTTP/2, gRPC and HTTP/3 on bytes and time, for very short to huge chat messages, over
emulated local network, Bluetooth, internet and mobile links, on open datasets of real
conversations. Every number can be reproduced and every scenario changed.
[Latest results](https://github.com/p2pact/p2pact-tests/blob/main/results/RESULTS.md).

## How it compares

- **ACP** defines how a client talks to an agent, inside one machine. P2PACT takes that chat
  across devices.
- **MCP** connects an agent to tools, **A2A** delegates tasks between agents of different
  services over HTTP. Neither is a chat between your own devices.
- **ANP (Agent Network Protocol)** connects agents of different owners across the open web
  with DIDs and domains. P2PACT connects the devices of one owner, without domains or servers.
- **WireGuard, iroh, libp2p** are excellent pipes and P2PACT borrows from them (WireGuard's
  crypto choices in particular), but they know nothing about agents or chats.

## Contributing

Issues and pull requests are welcome: the spec, implementations in other languages, tests.

## License

[Apache License 2.0](LICENSE).
