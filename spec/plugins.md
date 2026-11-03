# Tool 4 `p2pact.plugins` — a device's plugins, v1 (draft)

Runs on top of the core (`core.md`). The view and the controls of the plugins a device hosts:
which are installed, in what state, what they may do, their log; switching them on and off. A
client on another device shows them and manages them under grants (core §8.1).

**Relation to hosted tools.** A plugin is a program the device starts (`core.md` §2, §3). The
tools it announces are served by the device to everyone else under the plugin's names (core
§8.2): they are *not* this tool. Tool 4 is about the plugins themselves.

## 1. Plugins on the device

A plugin is a folder `<data>/plugins/<name>/` with a manifest `plugin.json`:

```json
{
  "name": "com.example.jira",
  "version": "1.2.0",
  "title": "Jira",
  "run": ["./jira-plugin"],
  "tools": ["com.example.jira"],
  "read": {"com.example.jira": [1, 2]},
  "needs": ["p2pact.chat:read", "p2pact.fleet:read"],
  "platforms": ["darwin", "linux", "windows"]
}
```

- `name` — reverse domain, the folder's name; a tool name's rules (core §8). `run` — the
  command, run in the folder (a first element with a slash is relative to it). `tools` — the
  only tool names the device accepts from the plugin; any other it announces is ignored.
- `needs` — `tool[:read|:write]` (read when no level): the grants the plugin gets, and nothing
  else. The person is shown them before the install and confirms. `platforms` — empty for any.
- `read` — per tool, the numbers of the methods that need only read (core §8.1); every other
  method of the tool needs write, so a plugin that says nothing is safe. The tool's events go to
  devices with read on it.

The device starts the plugin with `P2PACT_SOCKET` (the local socket) and `P2PACT_INVITE` (a
single-use invite carrying the grants of `needs`), and `P2PACT_PLUGIN_DATA` (a folder for its
keys). The plugin pairs as `kind` plugin; the person is never asked. The device keeps the
plugin's output in `plugin.log` (capped) and restarts a plugin that exits after 1 s, 5 s, 30 s;
a fourth exit in a row disables it and tells the person. A plugin that ran a minute starts the
count over.

## 2. Messages

Carried as in fleet (`fleet.md` §1): `4.1 rpc [request]` → `[response]`, JSON-RPC 2.0 with the
object turned into CBOR one to one; errors `-32602` params, `-32601` no such method, `-32004`
no such plugin, `-32000` failed. A call refused for lack of a grant (a write to a reader) is
P2PACT error 4, not a JSON-RPC error.

Event `4.1 notify [notification, n]` — a JSON-RPC notification from the device to a client
that subscribed with `plugins/watch` (§3); `n` counts per device, as in fleet. Sent only while
the client holds read on `p2pact.plugins`; the subscription ends when read is taken away, the
road to the client is gone (it subscribes again after its next hello) or it unwatches. Events
of different plugins and kinds are independent: no order between them is promised.

- `plugins/changed` — params: the plugin's `Plugin` entry as in `plugins/list`, now. Sent when
  a plugin is installed, removed, switched on or off, starts, stops, announces or drops a tool,
  exits (`restarting`), or is disabled after repeated exits (`failed`, `enabled: false`). Removed:
  the last entry plus `removed: true`. The same state is never sent twice in a row. A slow client
  may lose events: `plugins/list` is the truth.

## 3. Methods

*Read* (level 1):

- `plugins/list` — `{plugins: [Plugin]}`; `Plugin` is `{name, version, title, enabled, status,
  error?, restarts, pid?, tools, announced, needs}`. `status` is `running`, `starting`,
  `restarting`, `stopped` (switched off), `failed` (disabled after repeated exits),
  `unsupported` (not for this system), `broken` (unreadable manifest). `announced` — the tools
  the plugin has announced now, a subset of `tools`.
- `plugins/log` `{name, lines?}` — `{name, log}`: the last `lines` (100) lines of `plugin.log`.
- `plugins/grants` `{name}` — `{name, grants: [{tool, level}], read}`: what the plugin may use
  of the device (`level` is the string `read` or `write`) and which of its methods are read.
- `plugins/watch` — subscribe to `plugins/changed` (§2); the answer is the same as
  `plugins/list`, taken after the subscription starts, so no change falls between the two.
  `plugins/unwatch` — `{ok: true}`. A device without the event source answers `-32601`.

*Write* (level 2):

- `plugins/enable` `{name}` · `plugins/disable` `{name}` — start or stop the plugin; the choice
  survives the device's restart. Enabling a plugin that failed starts the count over.

## 4. What stays on the device

`plugins/install` `{source | stage, dry?}` and `plugins/remove` `{name}` are not offered over
P2PACT (`-32601`, even with write): they put code on the computer or take it away, which is a
person at that computer. Install takes a folder or a git address and shows the manifest (`dry`)
before the person confirms; the plugin's `data/` and log survive an upgrade.

A plugin is a program of the device's OS user: grants limit what the *device* gives it, not the
files of the computer. The person is told this at install.

## 5. Versions

This is tool 4 version 1 (hello `{4: 1}`). New methods and fields are only added; unknown ones a
participant skips.
