# Tool 6 `p2pact.screen` — screens, v1 (draft)

Runs on top of the core (`core.md`). A computer's screens seen and, if allowed, driven from
another device of its owner: the picture goes as H.264 video, the keys and the mouse come back.
The device that shows its screen is the **source**, the one that watches is the **viewer**.

**Two levels.** Looking and driving are granted apart (core §8.1): level `1` (read, *view*) lets
a device see the screens and open a stream of them; level `2` (write, *control*) also lets it
type and click. A device may be given view alone.

A source asks its person before it shows a screen to a device it has no standing permission
for (see §5): the person's answer is not part of the wire, only its result is (`-32001`).

## 1. Messages

The model is JSON-RPC 2.0, carried as in term (`term.md` §1): one JSON-RPC message = one
P2PACT message, its JSON object turned into CBOR one to one — except the fields named `data`,
which are CBOR byte strings.

- `6.1 rpc [request]` → `[response]` — a JSON-RPC request (`method`, `params`; no `id`).
- event `6.1 notify [notification, n]` — a JSON-RPC notification from the device. `n` — number
  of the event on its stream, from 1 for each pair of devices and stream.

Codes: `-32602` params, `-32601` no such method, `-32000` failed, and those of §2.

**Streams.** `screen/frame` and `screen/keyframe` of a viewing go on the stream numbered from
BLAKE2s(`id` + `"/v"`), `screen/input` on the stream numbered from BLAKE2s(`id`): the first 8
bytes as an unsigned integer, 0 replaced by 1. Everything else goes on stream 0. So a late
picture never holds up the keys, nor the keys the picture.

**Media is not learnt from.** A `screen/frame` is sent without compression and is not added to
the history a device trains its dictionary on (core §5): video does not shrink and would only
spoil the dictionary. This changes nothing on the wire: the message is a normal one, with the
"compressed" flag `8` not set; a receiver needs no new rule.

## 2. Methods

*View* (level 1):

- `screen/list` — `{}` → `{screens: [{id, name, w, h, scale, main}]}`. `id` — the source's own,
  stable while the screen is connected; `w`, `h` — pixels; `scale` — pixels per point;
  `main` — the main display.
- `screen/open` `{screen, max_size, fps, control, codecs, device?}` → `{id, w, h, codec, fps}`.
  Starts a stream of `screen` (its `id` from the list) no longer than `max_size` pixels on the
  longer side at most `fps` frames a second; `codecs` — the viewer's, in order of wish
  (`["h264"]`). `control: true` also asks to type and click and needs level 2. `id` — the
  stream's. `device` — only from a program on the source's own local socket: the device the
  stream is for (a proxy through the engine of its own computer). Errors:
  - `-32001` the person refused (or did not answer in 60 s);
  - `-32002` the source's OS does not let it capture the screen;
  - `-32003` no such screen;
  - `-32004` `control: true` without level 2.
- `screen/keyframe` `{id}` — the next frame is a key frame. `{}` answer.
- `screen/close` `{id}` — end the stream. `{}` answer.

*Local* — only from a program on the source's own local socket (a command of the product, not a device of the
fleet); a device of the fleet gets `-32601`. They manage the source and are for its person:

- `screen/devices` `{}` → `{devices: [{addr, name, os, phone}]}` — the devices the source knows.
- `screen/consents` `{}` → the standing permissions, the questions waiting and the running streams.
- `screen/consent` `{device, action, level?}` — `action` `allow|deny|revoke`: allow (a standing permission
  of `level` `view|control`, answers the waiting question), refuse the waiting question, take the permission back
  (running streams of the device end).
- `screen/stop` `{id?}` — end the stream `id`, or all of them.

*Control* (level 2):

- `screen/input` `{id, kind, x, y, button, dx, dy, code, mods, text, at}` — one input, `kind`
  `move|down|up|scroll|key|text`. `x`, `y` — position in the screen, 0..1 of its width and height;
  `button` — `left|right|middle`; `dx`, `dy` — scroll, in points, with the sign of a W3C `wheel` event's
  `deltaX`, `deltaY` (`dy > 0` — the content moves up, as the page scrolls down); a source whose system counts
  the other way flips them; `code` — a W3C `KeyboardEvent.code`
  (`KeyA`, `Enter`); `mods` — a list of `shift|ctrl|alt|meta`; `text` — text to type; `at` — when
  the person acted, milliseconds since 1970. `down` and `up` press and release a mouse
  `button` at `x`, `y`, or, with a `code` and no `button`, a key; `key` is a press and a release
  of `code`; `text` types `text`. A source drops an input older than 2 s and may join `move`s (at most 60 a
  second). A stream opened without `control` takes none (error `-32004`).

## 3. Events

All are *view*.

- `screen/frame` `{id, n, ts, key, data}` — the viewing's stream: one access unit of H.264 in
  Annex B form; `n` — its number from 1, `ts` — microseconds since the stream began, `key` — a
  key frame; a key frame carries SPS and PPS. Frames are not repeated and not skipped by the
  protocol; a source that cannot keep up drops frames before it encodes them (§4).
- `screen/stopped` `{id, reason}` — stream 0: the stream ended on the source's side: `stop` (its
  person pressed Stop), `permission` (the OS took the capture right back), `display` (the
  screen went away), `error`.
- `screen/road` `{id, road}` — stream 0: the road under the stream changed: `lan`, `direct`,
  `relay`. The viewer shows it, and the next frame is a key frame.

## 4. Pace

There is no unreliable channel: delivery repeats what is lost (core §5), so a slow road must
not be given more than it carries. The source keeps its sending queue short: while what waits
to go is more than about 150 ms of the road's rate, it does not feed the encoder a new frame
(a frame dropped before encoding costs nothing, one dropped after breaks the picture). Its
bit rate follows 0.7 of the road's measured rate. A key frame goes at the start, on
`screen/keyframe`, and when the road changes. A source that has no `lan` road yet waits for it
up to 1 s before the first frame if the devices may be on one network.

## 5. Permission

A source shows a screen to a device only with its person's approval: asked on the computer and,
if the person has other devices, on them; no answer in 60 s is a refusal. A source with no
person at the screen (a service) asks by notification. An approval the person remembers for the
pair is a **standing permission** and is not asked again; it can be revoked at any time. While a
stream runs, the source shows that it does and a way to stop it.

## 6. Versions

This is tool 6 version 1 (hello `{6: 1}`). New methods and fields are only added; unknown ones a
participant skips.
