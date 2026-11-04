# Tool 5 `p2pact.term` — terminals, v1 (draft)

Runs on top of the core (`core.md`). Shells on a computer, seen and typed into from any of its
owner's devices: a phone, another computer, a browser page. A terminal outlives the device that
opened it: others attach to it, see what it printed and type into it, under grants (core §8.1).

**What write gives.** A terminal is a shell of the computer's OS user: write on `p2pact.term`
is everything that user may do on that computer. A device grants it only to devices of the same
person, and shows it as such when asked (core §2, `wants`).

## 1. Messages

The model is JSON-RPC 2.0, carried as in fleet (`fleet.md` §1): one JSON-RPC message = one
P2PACT message, its JSON object turned into CBOR one to one — except the fields named `data`
(§3), which are CBOR byte strings, not base64 text.

- `5.1 rpc [request]` → `[response]` — a JSON-RPC request (`method`, `params`; no `id`: the
  P2PACT call id plays its part). A JSON-RPC error comes back as the response with `error`;
  P2PACT errors (`1` no such method, `4` forbidden, `6` not delivered) mean the call never
  reached the terminals.
- event `5.1 notify [notification, n]` — a JSON-RPC notification from the device (§3). `n` —
  number of the event on its stream, from 1 for each pair of devices and stream, so that a
  receiver runs events in order (as in chat).

Codes: `-32602` params, `-32601` no such method, `-32000` failed (the text is in `message`).

**Streams.** Calls and events about one terminal that carry its bytes — `term/input`,
`term/out`, `term/exit` — go on the stream numbered from its `id`: the first 8 bytes of
BLAKE2s(id) as an unsigned integer, 0 replaced by 1 (as in chat). So a terminal's output stays
in order and one busy terminal does not hold up another. Everything else goes on stream 0.

**Answers come as events.** The methods answer `{}` at once; what they produce — a new terminal,
the list, output — comes as events, because the same events also reach the other devices that
watch. A call's `at` (optional, every method) is when the person acted, in milliseconds since
1970; a device may drop a command that waited too long on the way (it drops `term/input` older
than 2 minutes: keys typed before a laptop slept are not typed after it wakes).

`Term` — a terminal: `{id, cwd, title, created, cols, rows, seq, chat?, cli?}`. `id` — the
device's own, stable while the terminal lives; `cwd` — its folder; `title` — what to show
(the folder's name by default); `created` — milliseconds since 1970; `cols`, `rows` — its size
now; `seq` — how many bytes it has printed so far. `chat` — the chat (`chat.md`) it was opened
from; `cli: true` — it runs that chat's agent instead of a shell. A participant skips fields it
does not know.

## 2. Methods

Level per method (core §8.1): *read* needs level 1, *write* level 2. A refused method is the
P2PACT error `4`, not a JSON-RPC error.

*Read:*

- `term/list` — `{}`; the device sends event `term/list` to the caller.
- `term/attach` `{id, since, cols?, rows?}` — start watching: the device sends the caller what
  the terminal printed from byte `since` that it still keeps (`term/out` with `tail: true`, in
  pieces of at most 32 KB), then its output as it comes. `since` is the `seq` after the last byte
  the caller has (0 — everything kept). `cols`, `rows` — the caller's window, as `term/resize`.
  No such terminal — event `term/exit {id, code: -1}`.
- `term/detach` `{id}` — stop watching.

*Write:*

- `term/open` `{dir, cols, rows, mid?}` — open a shell in folder `dir` with that window size.
  The device sends `term/opened` to the caller, and the caller watches it. `mid` — the caller's
  own mark, returned in `term/opened`, to tell its opens apart. A device that cannot open it
  answers `-32000` or sends nothing; a caller does not wait forever.
- `term/input` `{id, data}` — `data`: bytes to type, as they are (keys, pasted text, control
  bytes). No such terminal — event `term/exit {id, code: -1}`.
- `term/resize` `{id, cols, rows}` — the caller's window. The device keeps a size per watching
  device and gives the terminal the size of the device that typed last (or of the caller while
  nobody has typed); when another device types, the terminal takes its size.
- `term/close` `{id}` — end the terminal; every device watching it gets `term/exit`. No such
  terminal — event `term/exit {id, code: -1}` to the caller.

## 3. Events

All are *read*: a device with read on the tool gets them, one without it gets nothing.

- `term/opened {id, mid?, cwd, chat?, cli?}` — stream 0: a terminal was opened for the caller
  of `term/open` (with its `mid`). A device may also open a terminal on its own — say, to run a
  chat's agent in it (`cli: true`) — and tell the devices that show that chat.
- `term/list {list: [Term]}` — stream 0: the terminals now; the answer to `term/list`, and sent
  again when a terminal opens or ends, to the devices that show its terminals (at least the
  one whose call opened it).
- `term/out {id, seq, data, tail?}` — the terminal's stream: bytes it printed, from byte `seq`
  of its output. `tail: true` — part of the catch-up after `term/attach`: output that came
  before it is already in it. Only to devices watching the terminal (they opened or attached
  and did not detach).
- `term/exit {id, code}` — the terminal's stream: it ended with exit `code` (`-1` — no such
  terminal, or it failed); the watchers stop. To the devices watching it.

A device out of reach is dropped from the watchers; after its next hello it attaches again with
the `seq` it has, and loses nothing the terminal still keeps.

## 4. Former names

A device that served terminals before this tool, under a name of its own (a named tool from 64
up, say `com.example.term`), may keep announcing that name next to tool 5 for devices that
know only it (core §8.1, former names). The methods and events are the same; a device answers
and sends events under the number it was called by or watched under, and grants on either name
count for both.

## 5. Versions

This is tool 5 version 1 (hello `{5: 1}`). New methods and fields are only added; unknown ones
a participant skips.
