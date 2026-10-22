# Tool 1 `chat` — ACP over P2PACT, v1 (draft)

Runs on top of the core (`core.md`). A chat between a person and an agent, or between agents,
on different devices of one owner.

**Why ACP.** ACP (Agent Client Protocol, agentclientprotocol.com) already defines what a client
and a coding agent say to each other: sessions, prompts, streamed replies, tool calls, plans,
permission requests, file and terminal access. P2PACT does not invent another chat format: it
carries ACP between devices. An ACP client on a phone talks to agents on a laptop as if they
ran next to it; an existing ACP agent needs no change.

## 1. Roles

- **Agent device** — runs agents (Claude Code, Codex, OpenCode… through their ACP adapters, or
  anything that speaks ACP) and offers them as tool 1. One device may host many agents and many
  sessions.
- **Client device** — shows chats and sends prompts: a phone, another laptop, a browser page.
- A device can be both. Two agent devices may chat with each other (an agent asks another one).

## 2. Messages

ACP is JSON-RPC 2.0. Each JSON-RPC message travels as one P2PACT message, its JSON object turned
into CBOR one to one (map, array, text, number, bool, null; RFC 8949 §6.2): smaller, and the
dictionary compresses it well.

- `1.1 rpc [request]` → `[response]` — a JSON-RPC request (`method`, `params`; no `id`: the
  P2PACT call id plays its part, and the response keeps the call's id). Either side calls it:
  the client for `initialize`, `session/new`, `session/prompt`…; the agent device for
  `session/request_permission`, `fs/read_text_file`, `terminal/create`…
- event `1.1 notify [notification]` — a JSON-RPC notification (`session/update`,
  `session/cancel`…).
- A JSON-RPC error comes back as the response with `error` (code, message, data); P2PACT errors
  (`1` no such method, `6` not delivered…) mean the message never reached ACP.

**Streams.** Everything about one ACP session goes on its own stream (core §5): stream number =
the first 8 bytes of BLAKE2s(sessionId) as an unsigned integer, 0 replaced by 1. Messages of
one session stay in order; a loss on one chat holds up no other. Calls without a session
(`initialize`, `session/new`, listing) go on stream 0.

## 3. Connection

ACP's `initialize` is per connection; here the "connection" is the pair of devices. The client
calls `initialize` once per session of the core (after the hello) and again after the agent
device restarts (its boot id changed). Capabilities are those of ACP; P2PACT adds under
`_meta.p2pact`:

```
"_meta": {"p2pact": {"agents": [{"id": "claude", "name": "Claude Code", "version": "…"}, …]}}
```

— the agents the device can start (in the `initialize` response).

## 4. Many agents, many sessions

ACP talks to one agent; a device offers several. The agent is chosen in `session/new`:
`_meta.p2pact.agent` = one of the announced ids (default — the first). Every later message names
the session (`sessionId`), so the device knows which agent it is for.

Sessions outlive the connection: a client that reconnects resumes with `session/load` (if the
agent announced `loadSession`) and gets the history again as `session/update` notifications.

**Listing.** A client needs the device's chats, also those started elsewhere (in a terminal,
from another device). The agent device answers ACP's `session/list` (cursor, `cwd`) with every
session it can load, and adds under `_meta.p2pact` of each: `agent`, `title`, `state`
(`working` | `waiting` — waits for the person | `idle` | `closed`), `updatedAt`. When a session
appears, changes state or closes, the device sends event `1.1 notify` with
`{"method": "_p2pact/session_changed", "params": {<the same entry>}}` to every client that has
called `initialize` in this session of the core.

## 5. Delivery and duplicates

The core delivers each message once and in order within a stream, across road switches. A
prompt sent while the agent device is unreachable waits in the client's queue (core §5) and goes
when a road is back; it is not lost and not doubled. A client that restarts with prompts not yet
confirmed sends them again with `_meta.p2pact.mid` (a random id); the agent device ignores a
`mid` it has seen in the last 24 h and answers as before.

## 6. Permissions and questions

`session/request_permission` is a call from the agent device to the client device. More than
one client may be connected: the agent device asks all of them, the first answer wins, and the
others get `{"method": "_p2pact/request_answered", "params": {"sessionId", "toolCallId"}}` so
they stop showing it. With no client connected, the request waits (the agent waits) until one
connects and calls `session/load` or `session/list`.

## 7. Files and terminals

`fs/*` and `terminal/*` are ACP's way for an agent to use the client's files and terminals. Here
the agent usually runs where the files are, so an agent device does not offer the client's
files: clients answer these with JSON-RPC error `-32601` unless they deliberately share a
folder. Large attachments a person sends with a prompt go inside the prompt (ACP content
blocks), split into parts by the core.

## 8. Versions

This is tool 1 version 1 (hello `{1: 1}`). ACP's own version is negotiated in `initialize` as
ACP says.
