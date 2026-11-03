# Tool 2 `p2pact.fleet` — computers and their sessions, v1 (draft)

Runs on top of the core (`core.md`). The view and the controls of a device that hosts agents:
what the computer is, which agent sessions run on it, in what state, and its settings. A client
on another device (phone, another laptop, a browser page) shows the fleet of its owner and
manages it, under grants (core §8.1).

**Relation to chat.** Tool 1 (`chat.md`) is the conversation with one agent. Tool 2 is
everything around it: *where* sessions run, which exist, their phase and owner, starting and
stopping them, the device's settings. A session has the same id in both.

## 1. Messages

The model is JSON-RPC 2.0, carried exactly as in chat (`chat.md` §2): one JSON-RPC message =
one P2PACT message, its JSON object turned into CBOR one to one.

- `2.1 rpc [request]` → `[response]` — a JSON-RPC request (`method`, `params`; no `id`: the
  P2PACT call id plays its part). A JSON-RPC error comes back as the response with `error`
  (`code`, `message`); P2PACT errors (`1` no such method, `4` forbidden, `6` not delivered)
  mean the call never reached the fleet.
- event `2.1 notify [notification, n]` — a JSON-RPC notification from the device to a client
  that subscribed (§4). `n` — number of the event on its stream, from 1 for each pair of
  devices and stream, so that a receiver runs events in order (as in chat).

Codes: `-32602` params, `-32601` no such method, `-32004` no such resource (session),
`-32000` failed (the text is in `message`). Resources are `{kind, id, spec, status}`: `Node`
(the device: name, `os`, `status` — online, lid, battery, temperature, sessions working —
`capabilities` — agents, tools present, logins present) and `Session` (`spec` — agent, model,
dir, keep, perms; `status` — phase, title, owner, ask).

**Streams.** As in chat: calls and events about one session (`params.id`) go on the stream
numbered from its id — the first 8 bytes of BLAKE2s(id) as an unsigned integer, 0 replaced by
1 — so `session/send` and `session/interrupt` of one session stay in order. Others go on
stream 0.

**Long calls answer later** (core §6): the methods that change a session may take minutes (the
device runs them one after another); the device answers them when they end, without holding
the stream.

## 2. Methods

Level per method (core §8.1): *read* needs level 1, *write* level 2. A refused method is the
P2PACT error `4`, not a JSON-RPC error. An unmarked method is write, so a method this version
does not list is refused to a read-only device and answered `-32601` to the others.

*Read:*

- `fleet/hello` — `{protocol, node, …}`: the version of the fleet methods and the `Node`; the
  device may add the version of its engine.
- `node/get` — `Node`.
- `session/list` — `{sessions: [Session]}`: sessions not finished.
- `session/get` `{id}` — `Session`; no such session — `-32004`.
- `session/messages` `{id, after?, limit?}` — `{messages, last, older}`: the replies of the
  session after number `after` (without it, the last `limit`).
- `fleet/watch` `{partial?}` — subscribe the caller (§4); the response is `{sessions}`, the
  list now. `fleet/unwatch` — end the subscription.

*Write:*

- `engine/status` — the engine's state: version, mode, link, clients, peers. Write, not read:
  it names the device's data directory and its other paired devices.
- `settings/get` — the engine's settings (write for the same reason).
- `session/create` `{spec, text?}` — start a session; returns `Session`.
- `session/send` `{id, text}` — a message into the session.
- `session/answer` `{id, waitId?, answers | allow}` — answer an agent's question or permission.
- `session/interrupt` `{id}` — stop the current turn.
- `session/close` `{id}` — stop the agent and remove the session; may answer
  `{ok: true, warning}` (done, but not smoothly).
- `session/setKeep` `{id, on}` — keep the agent running.
- `settings/set` `{patch}` — change settings; returns them after the change. Invalid `patch`
  — `-32602`, nothing applied.

The parameters and results are those of the fleet protocol 0.2 of Nomad Code, the same on
every transport; a result is `{ok: true}` where nothing else is to say.

## 3. What stays on the device

Not offered over P2PACT (`-32601`; to a read-only device `4`): they either are about the
device itself or would let a call widen its own power.

- `device/list`, `device/grant`, `device/revoke` — who the device trusts and what each may do.
  A device with write on the fleet could grant itself everything. Grants change on the device
  (its terminal or app), and later through the family (`family.md`) by the parent.
- `pair/start` · `pair/confirm` · `pair/reject` · `pair/stop` — pairing a new device (core §2)
  is a person at that device.
- `phone/forget` — removes a trusted device; same reason as grants.
- `engine/yield` — hands the work between two engine programs on the same computer; it has
  no meaning from another device.

## 4. Watching

`fleet/watch` subscribes the calling device; the subscription lasts for the session of the
core and is repeated after a new hello (the device forgot it with its restart). The device then
sends event `2.1 notify` with:

- `session/changed {sessions}` — the whole list, at every change;
- `session/messages {id, messages}` — new replies of a session;
- `session/resync {id}` — a device that could not keep up lost replies of `id`: read
  `session/messages {id, after}` again;
- `session/partial {id, message}` — the agent's text while it is written; only to those who
  asked with `partial: true`;
- `notify {text}` — what the device would show as a notification.

Events are *read*: a device with read on the tool gets them, one without it gets nothing; when
a grant is taken away the subscription ends. A device that is out of reach is dropped from the
subscribers. Answering questions is done through chat (`session/request_permission`) or
`session/answer`; watching does not make the caller the one who answers.

## 5. Versions

This is tool 2 version 1 (hello `{2: 1}`). New methods and fields are only added; unknown
ones a participant skips. Nomad Code's own protocol of the local `nomad` command (fleet 0.2,
HTTP) stays as it is: its methods are those above plus the local-only ones of §3.
