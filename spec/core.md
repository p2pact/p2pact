# P2PACT core — v1 (draft)

Identifier: `p2pact/1`.

**What it is.** P2PACT — the P2P Agent Chat Transport Protocol — is how people and AI agents chat across
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
- **Application roads** — a link the application opens and the node cannot: Bluetooth LE,
  a serial line, a pipe to another program. The application hands the node a way to send and
  gives it what arrives; the node treats it like any other road. One is either
  - a **datagram road** — one packet per datagram, which may be lost or reordered; it must
    carry any packet whole, up to 1264 bytes (the most the TCP road's frame takes: a
    handshake start with first data may pass 1200), or
  - a **stream road** — a link that keeps order and loses nothing while it is up: packets are
    framed as on TCP (a 2-byte length, then the packet), and the frames are cut into writes
    of at most the link's own size (its MTU), however small. A broken link is attached again
    as a new road, so framing starts afresh; a length over 1264 means the stream is damaged.

  The application declares the road's MTU and, if it knows it, its rate (bytes a second); the
  node plans with that rate until it has measured the link (§5).

  **Bluetooth LE GATT** is a stream road: characteristic writes and notifications of 20–512
  bytes (the negotiated ATT MTU minus 3), acknowledged by the link layer, in order; tens of
  kilobytes a second. A packet of 1200 bytes is 3 to 60 writes; the layers above the road do
  not change, and nothing new goes on the wire. A link with small datagrams that may be lost
  or reordered cannot carry P2PACT in v1: that would need packets cut below the session.

  **Which device is at the other end.** The application may tell the node that the road
  reaches device X (it saw X's advertisement): the node then sends to X on it as on X's hints,
  and starts a session over it at once if it has none. Otherwise the road is bound by what
  comes on it, as any road is: a handshake names the device (§4.2), a data packet finds its
  session; answers go back on the road they came on. An unbound road is also offered for
  pairing (§2): an invite needs no hints when the devices share such a link.
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
- **First data after a restart.** Part numbers count from the receiver's boot (§5): a sender
  that knew an earlier boot numbered the part in its first packet for that boot, and
  renumbers everything once the reply shows the new boot id. So a device takes first data
  only from a device that has heard its current boot id — that sent it data in a session, or
  answered its handshake, since it started. Otherwise it drops the first data unacknowledged
  (the handshake itself goes on), and the part comes again in the session, renumbered.
  Accepting it would deliver that message twice and drop the part that later gets its number.
- Replaying the first packet does not work: it carries a timestamp (8 bytes); the responder
  remembers the last one from that device and drops anything not newer.
- The sender's key must be in the trusted list, otherwise silence: a device does not answer
  strangers at all (it is invisible to scanners).
- The boot id tells the other side that this device restarted: unacknowledged messages are
  renumbered and resent — except results and errors: the calls they answer ended with the
  restart, and its call ids start from 1 again, so a stale result would answer a new call.
  A result whose call came before the restart is not sent at all. Round-trip and bandwidth
  estimates start afresh (the device may be on another path now).

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
  needs for the parts queued ahead. It doubles on a retry only while a queue builds, or after
  three tries in a row: a random loss is no reason to wait longer. A part is also taken as
  lost when one sent a quarter of a round trip after it has arrived (RACK, RFC 8985), or when
  three parts past it have arrived; it goes again at once, on the next live road if this one
  is silent. An ack sooner than the minimum round trip after a copy resent by the timeout is
  for an earlier copy — the timeout fired on a queue, not a loss: for RACK the part counts
  as sent at that earlier copy (else every part sent between the copies would be taken as
  lost), and the ack is a round-trip sample, so the timeout catches up with the queue. A
  reliable road (TCP, a relay) loses nothing, so there only the timeout applies:
  a late ack there means the road's own queue, not a loss.
- **Congestion control** — the window starts at 32 parts and keeps doubling every round trip
  while no queue builds (the round trip stays near its minimum); then it grows by one part
  per round trip. A loss halves it (never below 32) only while a queue builds: a loss
  without one is random and says nothing about congestion. While a queue builds, at most
  twice the bandwidth-delay product (in bytes) is in flight, the bandwidth being measured
  from acks (as in BBR). On a reliable road (TCP, a relay) there is no window of our own.
- **Slow application roads** (§3: Bluetooth) — a window of 32 parts is 35 KB, a second or
  more of such a link, and the round trip of a small packet says nothing about the queue a
  burst builds in it (over a pipe with no latency, a rate measured from it is many times the
  link's). So on an application road:
  - at most one second of its rate is in flight, in bytes: the measured rate, or the declared
    one before any measurement (16 KB/s when none is declared);
  - the rate is measured only from parts sent while others were in flight ahead of them;
  - the retransmission timeout adds twice the time the link needs for the part and those
    queued ahead of it (with their packet and framing bytes), and the 40 ms an ack may wait:
    a timer that fires on the link's queue sends the part again behind that queue, while a
    late one costs little where nothing is lost;
  - on a datagram road a part resent for a gap goes again only a round trip later: its copy
    waits behind the link's queue, and the acks that show the gap meanwhile do not mean it was
    lost again;
  - a stream road loses nothing and keeps order, so an ack for a part sent twice is for its
    first copy: a round-trip sample, and, as on TCP, only the timeout resends.
- **Long messages** — in packet-sized parts, flag `4` on all but the last.
- **Keepalive** — a flags-only packet (26 bytes), every 25 s on every quiet road (keeps NAT
  mappings open).
- **Compression** — zstd **with a dictionary**. Plain compression gains little on short texts:
  there is nothing to repeat inside one. A dictionary of what this conversation keeps repeating
  (words, phrases, code, an agent's output format) does. Messages under 64 bytes are not
  compressed; from
  32 KB on, a faster level is used (the best one costs more time than the bytes it saves).
  - Two kinds of dictionaries:
    - **common** — `dict/common-1.zdict` in this repository: 64 KB, trained on the training
      half of OpenAssistant oasst2 (people and assistants in many languages, Apache-2.0) with
      `zstd --train`; the same for everyone. It is what compresses until a device has its own.
    - **own** — every device trains one **for each device it talks to**, on that conversation
      only, so the dictionary shows the other side nothing it has not already seen. It is never
      listed to, used with or given to any other device. *How* to train is up to the
      implementation (any zstd dictionary in the standard format works: only its owner compresses
      with it); the reference one uses fastCover, the algorithm of `zstd --train`. The standard
      format — magic `0xEC30A437`, a non-zero id, entropy tables, then content — is required: a
      raw-content dictionary has no id to restore in the frame (below) and is refused.
  - **What an own dictionary learns:** the conversation messages the device sent to that device
    (not tool 0), each cut to 4 KB — the start of a message gains most; up to the newest 1 MB.
    While its own messages are fewer than 256 KB, the messages it received from that device
    fill the history up to 256 KB: they help a device that writes little (a phone), and stop
    helping once it has enough of its own. A device that has sent fewer than 8 messages worth
    compressing trains none: what it receives alone does not make a dictionary for what it
    sends. The history stays on the device, like its keys.
  - **When:** the first at 32 KB of history; a new one each time the history has doubled (up to
    1 MB); then monthly, or when compression of what it sends got 15 % worse than over the first
    32 KB after training (checked once a day). Dictionary size: a quarter of the history,
    4–64 KB.
  - **Not on the sending path:** sending and receiving only queue a message for the history; the
    queue is taken in batches in the background, and a due dictionary is trained in a lull of
    that conversation (the reference implementation: no message for 1 s, or after waiting 1 min).
    A new dictionary costs the other side a download, so it comes rarely.
  - **Dictionary id** — the first 16 bytes of BLAKE2s of its bytes: two different
    dictionaries never share an id, and a dictionary received from anyone is checked by it.
  - In the hello (`0.2 info`) a device lists the dictionaries **the receiving device may see**:
    the common ones and the pair's (its own for that device and the one it got from it). When
    that list changes it sends event `0.5 dicts` to that device.
  - **A listed dictionary keeps its place.** A message compressed with it may still be on its
    way on another stream, behind the new list (streams do not wait for each other), so places
    never shift: a new dictionary goes to the end, a dropped one leaves an empty id (a
    zero-length byte string) in its place. Only past 32 places does a new one take the oldest
    empty place. A receiver ignores empty ids.
  - A device that sees an id it lacks in the other's list asks for it (`0.5 dict [id]`),
    checks the id and announces its list again; what it gets belongs to the pair (a device
    that does not ask simply never gets that dictionary used for it). A device
    answers `0.5 dict` only with a dictionary the asker may see; otherwise error `3`.
  - A compressed message (flag `8` on all its parts) is `slot (1) | zstd frame`. The slot is
    the dictionary's position in the **sender's** list for this receiver (1, 2…), `0` — no
    dictionary. The zstd magic and the dictionary id inside the frame are stripped (8 bytes):
    the receiver restores them. The sender uses its own dictionary for the receiver if the
    receiver has it, else the newest one both have; if the result is not shorter than the
    original, it sends the original.
  - **Missing dictionary** (just published and not yet arrived, a stale list) is not an
    error: the receiver acknowledges delivery, holds the message, asks the sender
    `0.5 dict [id]` (the sender certainly has it), checks the id, decompresses and runs the
    message. Everything after it waits in order; only results of its own calls (the
    dictionary among them), hellos and `0.5 dicts` may run out of order. A receiver that does
    not know the sender's list yet (the sender restarted) waits for its hello the same way.
  - A device that asked for `0.5 dict` lacks that dictionary: the sender does not compress
    anything for it with that dictionary (the reply included) until it announces its list
    again.
  - Up to 4 dictionaries of each kind are kept (common, own for a device, a device's own):
    an old one lives long enough for packets in flight to decompress.

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
- **Order.** A device runs the calls and events of one stream one at a time, in the order
  sent (a call's turn ends when its handler returns); streams do not wait for each other, so a
  slow handler holds up its own stream only. Service calls (tool 0) run at once.
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
  `dictionaries` — ids (§5), each in its place; the hello is also sent as event `0.2` at the start
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
  kind (UDP before TCP before relay before an application road) wins only if heard within
  half a second of it, since a device that reconnected leaves old endpoints "alive" for a
  while. Retransmissions go on all roads.
- **New road** — a keepalive on it; answered — it is live. Known devices are tried on all
  hints every 30 s while there is no live direct road, and while the session runs over a relay
  or an application road: a keepalive to every UDP and TCP address in their hints. A packet
  that arrives on a better kind of road than the one in use is answered at once with a
  keepalive on it, so both devices hear the better road and move to it together.
- **Application roads** carry a session only while nothing better is up: a slow local link
  (Bluetooth) keeps two devices talking with no network; once UDP, TCP or a relay answers,
  the session moves there, and it comes back when they fall silent — a part resent on all
  roads arrives over the application road, and the answer comes back on it.
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
  follows ACP (Agent Client Protocol) so any ACP client can be a chat client: `chat.md`;
- `2` — reserved;
- `3 family` — the family of devices: `family.md`.

## 9. Verification

- **Interop tests** — scenarios with reference packets: pairing, handshake, refusing a
  stranger, first-packet replay, loss and reordering, a long message, switching roads
  mid-transfer, compression and a missing dictionary. An implementation in any language runs
  them against the reference one.
- **On one computer** — two devices with different data directories and a relay: in one
  process (automated tests) or in three (by hand), with artificial packet loss; Bluetooth as an
  application stream road over a local connection with 20-byte writes at 40 KB/s.

## 10. Versions

The version is in the prologue (`p2pact/1`) and the first number of the invite. New fields are
optional and unknown ones are skipped. Anything incompatible gets a new prologue; a device may
speak several and picks the newest common one.
