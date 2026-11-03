# Tool 1 `p2pact.chat` — ACP over P2PACT, v1 (draft)

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

**Long calls answer later** (core §6). The agent device starts `session/prompt` in turn and
answers it later, when the turn ends: `session/cancel` and the client's other messages on the
same stream reach the agent while it works. The client answers `session/request_permission`
later too, when the person decides: the agent device's `session/update` notifications on that
stream keep coming meanwhile.

## 3. Connection

ACP's `initialize` is per connection; here the "connection" is the pair of devices. The client
calls `initialize` once per session of the core (after the hello) and again after the agent
device restarts (its boot id changed). Capabilities are those of ACP; P2PACT adds under
`_meta.p2pact`:

```
"_meta": {"p2pact": {"agents": [{"id": "claude", "name": "Claude Code", "version": "…"}, …]}}
```

— the agents the device can start (in the `initialize` response). The same object carries
`"ext": 1` when the device speaks the extensions of §9. A client that wants the device's
extension notifications (§9.2) sends `"_meta": {"p2pact": {"ext": 1}}` in `initialize`.

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

**Read-only clients** (core §8.1, tool `p2pact.chat` at level 1). One P2PACT method carries all
of ACP, so the level is checked on the JSON-RPC method inside:

- *read* — `initialize`, `session/list`, `session/load`, the extensions marked *read* in §10,
  and the events (`session/update`, `_p2pact/session_changed`, `_p2pact/request_answered`,
  `_p2pact/report`);
- *write* — everything else: `session/new`, `session/prompt`, `session/cancel`,
  `session/set_mode`, `session/set_model`, the extensions marked *write* in §10, and any method
  not listed here.

A refused method is the P2PACT error `4`, not a JSON-RPC error. The agent device does not send
`session/request_permission` to a read-only client: it cannot answer.

## 7. Files and terminals

`fs/*` and `terminal/*` are ACP's way for an agent to use the client's files and terminals. Here
the agent usually runs where the files are, so an agent device does not offer the client's
files: clients answer these with JSON-RPC error `-32601` unless they deliberately share a
folder. Large attachments a person sends with a prompt go inside the prompt (ACP content
blocks), split into parts by the core.

## 8. Metadata under `_meta.p2pact`

ACP leaves `_meta` free for extensions; this tool uses the `p2pact` key. All fields are
optional for a reader; unknown ones are ignored.

- `initialize` request: `ext` (number) — the client wants §9.2 notifications. Response:
  `agents`, `ext` (§3).
- `session/new` request: `agent` (§4), `mid` (§5), and the first-chat fields `model` (string,
  one of the agent's models; empty — the agent's default), `text` (string — the first task,
  sent at once; the client need not send a separate prompt), `perms` (`"skip"` — the agent
  asks nothing | `"ask"` — safe mode; empty — the device's setting).
- `session/prompt` request: `mid` (§5). Response: `duplicate: true` — the `mid` was already
  executed, nothing was sent again.
- `session/load` response: `older: true` — only the tail of the conversation was replayed;
  `_p2pact/older` (§10) fetches earlier messages.
- `session/update` `update`: `seq` (number, increasing within the session) and `time` (RFC 3339)
  on every replayed or live message. A client that loads a session again skips the `seq` it has
  already shown. Streamed text of an unfinished agent message comes as `agent_message_chunk`
  pieces: the first one carries `seq` and the whole text so far, the following ones carry no
  `seq` and extend that message.
- `session/request_permission` `options[]`: `description` (string) — explanation of an answer
  option of an agent's question (§9.1).
- `session/list` entry and `_p2pact/session_changed`: `agent`, `title`, `state`, `updatedAt` (§4).

## 9. Extensions

ACP has no way to close a chat, page the history, or ask several questions with options at
once. The agent device adds methods named `_p2pact/<name>`, written like ACP methods (JSON-RPC,
`sessionId` in params, so they ride the session's stream). A device may implement any subset;
a method it does not know answers JSON-RPC `-32601`. A client must work without them (plain
ACP). Methods with another prefix (the reference implementation has `_nomad/*` for its own
apps) are not part of this tool; a client ignores them.

**9.1 Questions.** An agent may ask the person for a choice. One question with one answer goes
as ACP `session/request_permission`: `toolCall.title` is the question, `options[]` are the
answers (`optionId` `o0`, `o1`…, `name` = label, `kind` `allow_once`, `_meta.p2pact.description`).
The client answers `{"outcome": {"outcome": "selected", "optionId": "o1"}}`.

Several questions, or an answer that may take several options, go as a request from the agent
device `_p2pact/question`:

```
params: {"sessionId", "toolCallId",
         "questions": [{"question": "…", "header": "…"?, "multi": false?,
                        "options": [{"label": "…", "description": "…"?}, …]}, …]}
result: {"answers": {"<question text>": "<label>"}}
```

For a `multi` question the answer is the chosen labels joined by `", "`. Like
`session/request_permission`, it goes to every connected client with write access, the first
answer wins, the others get `_p2pact/request_answered` (§6). A client that cancels or never
answers leaves the question waiting.

**9.2 Notification names.** The device sends its extension notifications and requests
(`_p2pact/question`, `_p2pact/report`) to a client that announced `ext: 1` in `initialize` or
has called any `_p2pact/<name>`; the announcement is per connection and is read again at every
`initialize`. `_p2pact/session_changed` and `_p2pact/request_answered` go to every client as in
§4 and §6.

**9.3 Commands.** Every command method of §10 takes params as listed and answers
`{"lines": [<line>, …]}`: JSON objects, each with a `cmd` field naming what it is, produced
while the command ran. A line produced later (a long command) arrives as the notification
`_p2pact/report` with `{"line": <line>}` on stream 0. Lines with an unknown `cmd` are skipped.
A failure the person should see arrives as the line `{"cmd": "alert", "id": "<sessionId>",
"text": "…"}`. The result is `{"lines": []}` when there is nothing to report.

## 10. Extension methods

Level in brackets: *read* works for a read-only client (§6), *write* needs write access. Params
are those of the method; `sessionId` is a session of `session/list`.

- `_p2pact/older` *(read)* — earlier messages of a long conversation. Params `sessionId`,
  `before` (the `seq` of the oldest message the client has). Line `{"cmd": "older", "id",
  "messages": [{"seq", "role": "user" | "assistant" | "tool", "text", "time", "images"?}],
  "more": bool}` — the page just before `before`, oldest first; `more` — there are still
  earlier ones.
- `_p2pact/commands` *(read)* — the agent's slash commands of this chat, for completion in the
  input field. Params `sessionId`. Late line `{"cmd": "commands", "id", "list": [{"name",
  "desc"?, "hint"?, "args"?: [{"value", "desc"?}]}]}` (the device asks the agent, it takes seconds).
- `_p2pact/files` *(read)* — files of the chat's folder matching a query (for `@file`
  completion). Params `sessionId`, `query`. Line `{"cmd": "files", "id", "query", "list":
  ["<relative path>", …]}` (up to 50).
- `_p2pact/history` *(read)* — closed conversations of the agents on the device, newest first,
  for a "History" screen. Params `query` (text search; empty — all), `offset` (number, default
  0 = start; a page is up to 60 items). Line `{"cmd": "history", "query", "from", "total", "list":
  [{"id", "agent", "dir", "project"?, "title", "updated" (ms), "count"?, "open"?}], "partial"?}`;
  `count` may arrive in a second line with the same `from` that replaces the first. `open`: the
  conversation is in `session/list` now.
- `_p2pact/history_open` *(write)* — bring a closed conversation back into `session/list` as
  `closed` so it can be loaded and continued. Params `sessionId` (the `id` of a history item).
  No lines; an error comes as the alert line.
- `_p2pact/projects` *(read)* — folders to start a chat in: recent ones and folders grouped by
  project. Params `cwd` (an extra folder to describe, optional). Line `{"cmd": "projects",
  "dirs": ["<folder>", …], "project_dirs": {"<project>": ["<folder>", …]}, "dir_repos":
  {"<folder>": "<git remote>"}}`.
- `_p2pact/agents` *(read)* — agents with the models they offer for `session/new` (`model`
  of §8). No params. Lines `{"cmd": "presets", "list": [{"id", "name", …}]}` — ready-made
  choices (agent + model), the first is the main one — and `{"cmd": "agents", "list": [{"id",
  "name", "type", "enabled", "models": ["…"], "default"?}]}` (finding models can take seconds).
- `_p2pact/image` *(read)* — bytes of an image that a message points to with a `file://`
  `resource_link` (the file is on the agent device). Params `sessionId`, `path` (the path from
  the link; only images of that chat are served). Line `{"cmd": "image", "id", "path",
  "data": "<base64>"}` — big images come downscaled — or the same with `"error"`:
  `not_found` | `no_file` | `too_big` | `bad_image`.
- `_p2pact/diff` *(read)* — changes in the chat's folder (git). Params `sessionId`. Line
  `{"cmd": "diff", "id", "files": [{"path", "added", "removed", "status"}], "patch": "<unified
  diff>", "truncated": bool}`, or with `"error"`: `not_found` | `not_git` | text.
- `_p2pact/close` *(write)* — close the chat: it leaves `session/list` (state `closed` in
  `_p2pact/session_changed`), and the agent process of the chat is stopped when the device
  runs it. Params `sessionId`.
- `_p2pact/restart` *(write)* — start the agent of a chat again, same session (a hung or
  updated agent). Params `sessionId`. Only for chats the device runs itself; others get the
  alert line.
- `_p2pact/hide` *(write)* — hide a chat from the list by the person's choice, or show it again;
  the chat itself goes on. Params `sessionId`, `on` (bool).

Examples of every message: `contract/tools/p2pact.chat/` in the reference implementation
(github.com/aidarbn/nomad-code), one JSON file per message.

## 11. Versions

This is tool 1 version 1 (hello `{1: 1}`). ACP's own version is negotiated in `initialize` as
ACP says. Extensions only grow: a method, a line or a field is never removed or changed in type;
a new incompatible shape gets a new name.
