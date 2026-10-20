# P2PACT core — v1 (draft)

Identifier: `p2pact/1`.

**What it is.** P2PACT — the P2P Agent Chat Protocol — is how people and AI agents chat across
their own devices: how devices meet, find each other, trust each other, encrypt, and exchange
chat messages directly, without servers. The core below carries any message; the chat itself
is tool 1 (§8), and the family of devices (`family.md`, tool 3) decides who may chat with whom.
Other agent protocols (task delegation, tools) are out of scope; they can be built as separate
tools on the same core.

**Goals:**

- **any road** — UDP, TCP, a relay, Bluetooth, via a third device; the same packets on all of
  them, and switching roads mid-conversation does not break it;
- **set up once** — devices pair once and then find each other by themselves;
- **fast** — known devices send data in the very first packet, without waiting for a handshake;
- **compact** — 25 bytes of overhead per packet, binary messages with numbers instead of
  names, dictionary compression, changes instead of full copies;
- **open** — a spec, a reference implementation (Go), interop tests. No home-made crypto:
  Noise, X25519, ChaCha20-Poly1305, BLAKE2s, Ed25519.

Layers, bottom up:

1. **Roads** — anything that carries packets; they may lose and reorder them (§3).
2. **Session** — encryption, authentication, packet numbering (§4).
3. **Delivery** — acknowledgements, retransmission, splitting long messages, compression (§5).
4. **Messages** — calls, results, errors, events in CBOR (§6).
5. **Tools** — numbered sets of methods and events (§8).

## 1. Device

- On first start a device creates a **permanent X25519 key pair** (sessions) and an
  **Ed25519 key pair** (signatures). Private keys never leave the device.
- **Address** — the first 16 bytes of BLAKE2s of the X25519 public key (as text: lowercase
  base32, 26 characters). It cannot be forged without stealing the key.
- The name ("Work laptop") is a label for humans, never used for verification.
- No accounts, no registration, no central server.

## 2. Pairing — once

**Invite.** Device A creates the string `p2pact1.<base64url(CBOR)>`:

```
[1, key_A, "Work laptop", secret, exp, hints]
```

`secret` — 32 random bytes; `exp` — expiry (10 minutes); `hints` — where to find A now (§7).
The invite can travel **any way**: QR code, link, text, NFC. It is single-use.
*(planned)* A short 6-digit code to type by hand, with lookup on the local network.

**Pairing handshake** — `Noise_XXpsk2_25519_ChaChaPoly_BLAKE2s`, pre-shared key `secret`.
B checks that A's key matches the invite; A learns that B had the invite. Both save each other
as trusted: `[address, key, name, pair_secret, hints, added]`, with `pair_secret = secret`.
The handshake payload carries `[name, boot id, hints, signing key, os]`.

**Family.** Devices pair once — with the family: the parent or a deputy invites, the newcomer
receives the signed family book and meets everyone else from it, with no further QR codes
(`family.md`). Devices that know each other from the book derive their pair secret from their
permanent keys: `HMAC-BLAKE2s(X25519(own private, their public), "family-pair")`.

## 3. Roads

A road is anything that can **send a packet** and **receive a packet**. A road may lose,
duplicate and reorder packets — sessions and delivery cope. A road encrypts nothing and trusts
nothing. A packet belongs to a session, not to a road: one session runs over all roads at
once, and any packet may take any road.

- **UDP — main.** One packet is one datagram of at most 1200 bytes (crosses any network
  without fragmentation). Default port 48730.
- **TCP — fallback** where UDP is blocked (offices, hotels): a 2-byte length, then the packet.
  Connections are opened on demand and used both ways; a device behind a carrier NAT may only
  dial out. Hints list TCP addresses separately.
- **Relay** — a server that forwards packets inside a pair's channel and sees nothing else.
  The channel is the first 16 bytes of `HMAC-BLAKE2s(pair_secret, "relay")`: only those two
  know it. Today the relay speaks newline-terminated lines over TLS (a packet is a base64
  line, sides `a`/`b`; see the `relay` package); *(planned)* binary and over UDP.
- **Via a third device** — `0.4 forward [to, packet]` to a device that knows both; it forwards
  without understanding (end-to-end encryption). *(planned)*
- **Bluetooth LE** — a packet per characteristic write. *(planned)*
- **Local socket** — programs on the same computer (a UI, a CLI, an MCP server for agents)
  talk to the local node with the same packets. *(planned)*

## 4. Session

### 4.1 Packet

```
type (1) | receiver's session index (4) | packet number (4) | ciphertext | tag (16)
```

Overhead: **25 bytes**. The receiver chooses the session index, so a packet finds its
session by number, not by the sender's address: a network change (Wi-Fi → hotspot), another
road — same session. The packet number is explicit and grows with every packet; the receiver
keeps a window of the last 2048 numbers and drops replays and too-old packets. The packet
number is also the cipher nonce. The tag covers the cleartext header too.

Types: `1` handshake start, `2` handshake reply, `3`–`5` pairing (§2), `6` data,
`7` "unknown session".

### 4.2 Known-device handshake — data in the first packet

`Noise_IK_25519_ChaChaPoly_BLAKE2s`, prologue `p2pact/1`: the initiator already knows the
responder's key (they are paired).

1. initiator → `e, es, s, ss` + timestamp + boot id + **first data** (already encrypted);
2. responder → `e, ee, se` + its boot id + its data. The session is ready.

- First data does not wait for the reply: a call or event goes at once, and its answer comes
  in the second packet.
- Replaying the first packet does not work: it carries a timestamp (8 bytes); the responder
  remembers the last one from that device and drops anything not newer.
- The sender's key must be in the trusted list, otherwise silence: a device does not answer
  strangers at all (it is invisible to scanners).
- The boot id tells the other side that this device restarted: unacknowledged messages are
  renumbered and resent.

### 4.3 Session life

- A session lives as long as needed; keys are renewed every 2 hours or 2³¹ packets (a new
  handshake; the old session finishes within a minute).
- A packet for an unknown session → type `7`, at most once per 5 s; on `7` the initiator
  handshakes again (and immediately resends everything unacknowledged, §5).
- Both started a handshake at once — the session of the lower address wins.

## 5. Delivery

Inside the ciphertext — the delivery header and the message part:

```
flags (1) | [ack] | [part number | [stream | number in stream] | part]
ack = up to N | count | count × (missing, received)
```

Flags: `1` ack present, `2` message part present, `4` more parts follow, `8` compressed,
`16` do not acknowledge, `32` the first part of a message on a stream (the stream and the
message's number in it follow the part number), `64` a further part of a message on a
stream. Numbers are varints (usually 1–2 bytes).

- **Streams** — stream 0 (parts without `32`/`64`) is delivered strictly in the order sent.
  A message on another stream is delivered as soon as all its parts are here and its
  stream's earlier messages are delivered; streams do not wait for each other, so a loss on
  one does not hold up the rest. One stream per chat is the natural use. Message numbers
  start at 1 per stream; a result goes back on the stream of its call.
- **Ack** — "got everything up to N", then the ranges received above the first gap as pairs
  (parts missing, parts received), as many as fit in 60 bytes; it rides along in any
  packet. As in TCP, a standalone ack goes at once for every second part and for
  anything out of order; a lone part waits up to 40 ms for a reply to ride on.
- **Retransmission** — the timeout follows the measured round trip (RFC 6298: smoothed RTT
  plus four deviations, 50 ms – 4 s, 500 ms before the first sample) plus the time the link
  needs for the parts queued ahead, doubling on every retry; resend on the next live road if
  this one is silent. Three parts past a gap — resend the gap at once (fast retransmit).
- **Congestion control** — the window starts at 32 parts and keeps doubling every round trip
  while no queue builds (the round trip stays near its minimum); then it grows by one part
  per round trip. A loss halves it (never below 32) only while a queue builds: a loss
  without one is random and says nothing about congestion. While a queue builds, at most
  twice the bandwidth-delay product (in bytes) is in flight, the bandwidth being measured
  from acks (as in BBR). On a reliable road (TCP, a relay) there is no window of our own.
- **Long messages** — in packet-sized parts, flag `4` on all but the last.
- **Keepalive** — a flags-only packet (26 bytes), every 25 s on every quiet road (keeps NAT
  mappings open).
- **Compression** — zstd **with a dictionary**. Plain compression gains nothing on short
  texts; a dictionary of frequent words and fragments (natural language, code, agent replies)
  compresses even short messages 2–4×. Messages under 64 bytes are not compressed; from
  32 KB on, a faster level is used (the best one costs more time than the bytes it saves).
  - Two kinds of dictionaries: **common** — published with the spec, trained on open texts,
    the same for everyone; **family** — one per family, published by the parent
    (`family.md` §6). A device never trains its own — dictionaries would diverge.
  - **Dictionary id** — the first 16 bytes of BLAKE2s of its bytes: two different
    dictionaries never share an id, and a dictionary received from anyone is checked by it.
  - In the hello (`0.2 info`) a device lists its dictionary ids, newest first; when the list
    changes it sends event `0.5 dicts`.
  - A compressed message (flag `8` on all its parts) is `slot (1) | zstd frame`. The slot is
    the dictionary's position in the **sender's** list (1, 2…), `0` — no dictionary. The zstd
    magic and the dictionary id inside the frame are stripped (8 bytes): the receiver
    restores them. The sender uses the newest dictionary the receiver also has; if the result
    is not shorter than the original, it sends the original.
  - **Missing dictionary** (just published and not yet arrived, a stale list) is not an
    error: the receiver acknowledges delivery, holds the message, asks the sender
    `0.5 dict [id]` (the sender certainly has it), checks the id, decompresses and runs the
    message. Everything after it waits in order; only results of its own calls (the
    dictionary among them), hellos and `0.5 dicts` may run out of order. A receiver that does
    not know the sender's list yet (the sender restarted) waits for its hello the same way.
  - A device that asked for `0.5 dict` lacks that dictionary: the sender does not compress
    anything for it with that dictionary (the reply included) until it announces its list
    again.
  - The last 4 dictionaries are kept: a new one comes at most monthly, so an old one lives
    for months and packets in flight still decompress.

## 6. Messages

A message is a CBOR array (RFC 8949). The first element is the kind:

```
call    [0, id, tool, method, args…]
result  [1, id, values…]
error   [2, id, code, text]
event   [3, tool, event, data…]
```

- Numbers instead of names, fields in order, optional trailing fields omitted; time —
  Unix seconds; keys — bytes.
- A call `id` grows from 1 within a session; delivery has already removed duplicates, so a
  call runs exactly once.
- **Changes only.** State is not resent wholesale on a timer: subscribe → one snapshot →
  events about changes.
- **Short names.** Long ids (chats, files) are replaced by small numbers within a session:
  on first mention `[3, 0, 4, n, "a5a301b7"]` ("n means a5a301b7"), then just `n`.

Example — a new message "ok" in a chat (the chat already aliased as 1):

```
[3, 1, 1, 1, 58, 0, "ok"]        11 bytes
```

With the delivery header and the packet: about **40 bytes** on the wire. The same as JSON
over HTTPS is about 400.

**Tool 0 — service:**

- `0.1 ping` → `[]`;
- `0.2 info` → hello: `[name, os, implementation, {tool: version}, [dictionaries], hints]`;
  `dictionaries` — ids (§5), newest first; the hello is also sent as event `0.2` at the start
  of every session;
- `0.3 bye` — event: we were forgotten;
- `0.4 forward [to, packet]` — forward *(planned)*;
- `0.5 dict [id]` → dictionary bytes; event `0.5 dicts [dictionaries]` — the list changed;
- event `0.4 alias [n, value]` — a short name (above).

Errors: `1` no such method, `2` bad arguments, `3` not found, `4` forbidden, `5` failed,
`6` not delivered.

For humans: CBOR in diagnostic notation (RFC 8949 §8); *(planned)* a dump tool that prints
packets with names instead of numbers.

## 7. Paths

- **Hints** — where to find a device: UDP/TCP addresses, a relay. From the invite, from the
  hello (every device reports its new addresses over any working road), *(planned)* from
  local discovery (mDNS `_p2pact._udp`).
- **Live road** — something arrived on it within 60 s.
- **Choice** — every packet takes the road the current session was heard on last; a better
  kind (UDP before TCP before relay) wins only if heard within half a second of it, since a
  device that reconnected leaves old endpoints "alive" for a while. Retransmissions go on
  all roads.
- **New road** — a keepalive on it; answered — it is live. Known devices are tried on all
  hints every 30 s while there is no live direct road.
- *(planned)* **Direct across the internet** — both send UDP towards each other at the
  addresses the relay observed (NAT hole punching); once it works the relay is not needed.

## 8. Tools

A tool is a number with numbered methods and events, its own spec and version. A device
announces its tools in the hello; calling an unknown one is error `1`. Numbers 0–63 are
assigned by the spec; 64 and up are free for implementations, no coordination needed.

Permissions in v1: a known device may use everything announced (your own devices).
*(planned)* Per-tool permissions.

- `0` — service (§6);
- `1 chat` — chats with agents and between agents: list and read chats, send messages, answer
  an agent's question or permission request, subscribe to new messages. Its message format
  follows ACP (Agent Client Protocol) so any ACP client can be a chat client *(spec next)*;
- `2` — reserved;
- `3 family` — the family of devices: `family.md`.

## 9. Verification

- **Interop tests** — scenarios with reference packets: pairing, handshake, refusing a
  stranger, first-packet replay, loss and reordering, a long message, switching roads
  mid-transfer, compression and a missing dictionary. An implementation in any language runs
  them against the reference one.
- **On one computer** — two devices with different data directories and a relay: in one
  process (automated tests) or in three (by hand), with artificial packet loss.

## 10. Versions

The version is in the prologue (`p2pact/1`) and the first number of the invite. New fields are
optional and unknown ones are skipped. Anything incompatible gets a new prologue; a device may
speak several and picks the newest common one.
