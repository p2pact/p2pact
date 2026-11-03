# Write a plugin

A plugin is a program on the computer that adds **tools** to the P2PACT engine. Apps and other
devices call those tools through the engine, under the plugin's own tool name, and receive the
plugin's events. A plugin can be written in any language that has a P2PACT library (Go,
TypeScript, Python, Kotlin).

The engine is the one in Nomad Code (`nomadcode`, command line `nomad`). The wire details are
in [`spec/plugins.md`](../spec/plugins.md) and [core §8.2](../spec/core.md). To build an app
that uses plugin tools, see [apps.md](apps.md).

## How it fits

```
app ──P2PACT──▶ engine ──local socket──▶ plugin
        call to tool com.example.jira       (your program)
        ◀── result / error / events ────────┘
```

The plugin pairs with the engine as a device of kind `3` over a local socket, with no QR and
no question to the person (installing it was the consent). The engine announces the plugin's
tools to every other device in its own hello, checks the caller's grants, forwards the call
to the plugin and returns its answer. A caller cannot tell the plugin from the engine.

## Manifest `plugin.json`

A plugin is a folder with a `plugin.json` at its root. The folder name equals `name`.

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

- `name` — required. Reverse domain, lowercase, dot-separated (`com.example.jira`); the names
  `p2pact.*` are taken by the spec.
- `version`, `title` — shown to the person.
- `run` — required. The command, started in the plugin folder. A first element containing a
  slash is relative to the folder and must stay inside it (no absolute path, no `..`); other
  first elements are looked up in `PATH` (`["python3", "plugin.py"]`, `["node", "plugin.mjs"]`).
- `tools` — the **only** tool names the engine accepts from the plugin. Any other name it
  announces is ignored. Each must be a valid tool name.
- `read` — per tool, the numbers of the methods that need only *read* from the caller. Every
  other method of the tool needs *write*. Leave it out and every method needs write — the safe
  default. Every tool in `read` must be in `tools`. The tool's events go to devices with read
  on it.
- `needs` — what the plugin may use of the engine's own tools, `tool[:read|:write]`; no level
  means read (`p2pact.chat:read`). It gets exactly this: empty means nothing, not everything.
  The person sees it before the install and confirms.
- `platforms` — any of `darwin`, `linux`, `windows`; empty means any. On another system the
  plugin is listed as `unsupported`.

## Lifecycle

The engine starts every installed, enabled plugin when it starts. For each run it:

- sets the working directory to the plugin folder and adds to the engine's environment:
  - `P2PACT_SOCKET` — path of the engine's local socket;
  - `P2PACT_INVITE` — a single-use invite, valid two minutes, with the grants of `needs`;
  - `P2PACT_PLUGIN_NAME` — the plugin's name;
  - `P2PACT_PLUGIN_DIR` — the plugin folder;
  - `P2PACT_PLUGIN_DATA` — `<plugin folder>/data`, created for you: keep the plugin's keys
    and state here; it survives upgrades;
- writes the plugin's stdout and stderr to `plugin.log` in the folder (capped at 1 MB, then
  rotated to `plugin.log.1`).

Your program pairs at once with the library's `newPlugin`, announces its tools and runs until
it gets SIGTERM (on Windows it is killed). A new run means a new invite and fresh grants; the
engine forgets the plugin's device when the process ends.

**Restarts.** A plugin that exits is started again after 1 s, then 5 s, then 30 s. The fourth
exit in a row disables it (`failed`) and notifies the person. A plugin that ran for a minute
starts the count over. `nomad plugin enable <name>` starts a disabled one again.

**Stopping.** `nomad plugin disable <name>` (or the `plugins/disable` method, write on
`p2pact.plugins`) sends SIGTERM and after 3 s kills the process; the choice survives an engine
restart. While the plugin is gone but not disabled (restarting), calls to its tools fail with
P2PACT error `6`; when it is disabled or has failed, its tools leave the engine's hello.

States in `nomad plugin list`: `running`, `starting`, `restarting`, `stopped`, `failed`,
`unsupported`, `broken` (unreadable manifest, or a name different from the folder).

```sh
nomad plugin list
nomad plugin log com.example.jira -n 100
```

## Write the program

The steps are the same in every language:

1. `newPlugin(dir, name)` with `dir = $P2PACT_PLUGIN_DATA` — reads `P2PACT_SOCKET` and
   `P2PACT_INVITE`, pairs, returns your node and the engine as a device (`host`).
2. Register a handler per method of your tool (a method is a number you choose; document it).
   A handler gets the caller's address and the CBOR arguments and returns result values, or an
   error with a P2PACT code.
3. Register the tool name with its version: the library picks a number from 64 up and announces
   it in the hello. The name must be listed in `tools` of the manifest.
4. Send events to `host` with the tool number and your event number.

### Go ([p2pact-go](https://github.com/p2pact/p2pact-go))

```go
import (
	"context"
	"os"

	"github.com/fxamacker/cbor/v2"
	"github.com/p2pact/p2pact-go"
)

const tool = "com.example.hello"

func main() {
	n, host, err := p2pact.NewPlugin(context.Background(), os.Getenv("P2PACT_PLUGIN_DATA"), "Hello plugin")
	if err != nil { /* not started by an engine */ }
	defer n.Close()

	n.HandleTool(tool, 1, func(peer string, args []cbor.RawMessage) ([]any, error) {
		var name string
		if len(args) > 0 {
			_ = cbor.Unmarshal(args[0], &name)
		}
		return []any{"Hello, " + name}, nil // to fail: &p2pact.Error{Code: p2pact.ErrBadArgs, Text: "…"}
	})
	num, _ := n.RegisterTool(tool, 1)

	n.Event(host.Addr, num, 1, 42) // event 1 with one value
	select {}
}
```

### Python ([p2pact-py](https://github.com/p2pact/p2pact-py))

```python
import asyncio, os
from p2pact.cbor import decode
from p2pact.plugin import new_plugin

TOOL = "com.example.hello"

async def main():
    node, host = await new_plugin(os.environ["P2PACT_PLUGIN_DATA"], "Hello plugin")

    def greet(peer, args):               # to fail: raise p2pact.node.P2PACTError(code, text)
        return ["Hello, " + (decode(args[0]) if args else "world")]

    node.handle_tool(TOOL, 1, greet)
    num = node.register_tool(TOOL, 1)
    node.event(host.addr, num, 1, 42)    # event 1
    await asyncio.Event().wait()

asyncio.run(main())
```

Unix only today; Python 3.10+, and the `python3` in `run` must be able to `import p2pact`.

### TypeScript / Node.js ([p2pact-ts](https://github.com/p2pact/p2pact-ts))

```js
import { decode, newPlugin } from "p2pact";

const tool = "com.example.hello";
const { node, host } = await newPlugin(process.env.P2PACT_PLUGIN_DATA, "Hello plugin");

node.handleTool(tool, 1, (peer, args) => {        // to fail: throw new P2PACTError(code, text)
  return ["Hello, " + (args.length ? decode(args[0]) : "world")];
});
const num = node.registerTool(tool, 1);
node.event(host.addr, num, 1, 42);                // event 1
process.on("SIGTERM", () => { node.close(); process.exit(0); });
```

Node.js 22.15+. Kotlin has the same shape: `newPlugin(File(dir), name)` returns a
`PluginSession(node, host)`.

### Examples

Each is a complete plugin with a `plugin.json`, one read method `greet`, one write method
`bump` and a periodic `tick` event:

- Go — `agent/examples/plugin-hello` in the Nomad Code repository (`dev.p2pact.example.hello`)
- Python — [`p2pact-py/examples/plugin_hello`](https://github.com/p2pact/p2pact-py/tree/main/examples/plugin_hello)
- TypeScript — [`p2pact-ts/examples/plugin-hello`](https://github.com/p2pact/p2pact-ts/tree/main/examples/plugin-hello)

### Calls, errors, events

- Calls and events of one caller on one stream run one at a time, in order; a slow handler
  holds up its own stream only. For a call that takes long, answer later with the library's
  `Later` helper (Go `p2pact.Later`, TypeScript `later`, Python `Later`; core §6).
- A handler's error reaches the caller with the same code (`1`–`5`). The engine adds `6` when
  your plugin is gone.
- Do not check the caller's rights in your handler: the engine has already checked them
  against `read` and the caller's grants. The `peer` argument is the engine, not the caller.
- Events go to every device with at least read on the tool. Send changes, not whole state on a
  timer.
- The plugin may call the engine's tools within `needs` (it is a device holding those grants):
  in Go `n.CallTool(ctx, host.Addr, "p2pact.chat", method, args…)`; the other libraries have
  the same call by tool name.
- Keep values to CBOR types (numbers, text, bytes, lists, maps, booleans, null).

## How devices call it

An app that holds a grant on `com.example.jira` sees the tool in the engine's hello under a
number the engine chose, and calls a method by name:

```kotlin
val out = computer.callTool("com.example.jira", 1, "PROJ-1").await()
client.toolEvents.subscribe { e -> println("${e.tool}: event ${e.event} ${e.args}") }
```

A call needs *read* if the method number is in the manifest's `read`, otherwise *write*; a
call without it is refused with error `4` before it reaches the plugin.

## Install

```sh
nomad plugin install ./jira                                          # a folder
nomad plugin install https://github.com/example/jira-plugin.git      # a git address
nomad plugin install ./jira -y                                       # no question
```

The engine prepares a copy (for git, `git clone --depth 1`; no `.git`), `nomad` shows the
manifest — name, version, the command it runs, the tools it announces, `needs` — and asks
`y/N`. Installing again replaces the plugin and keeps its `data/` and log. The folder is
copied as it is, so build the program first; there is no catalogue yet. `nomad plugin remove
<name>` uninstalls. `nomad plugin list | enable | disable | log` manage the rest.

Install and remove only run at the computer: over P2PACT they are refused even with write,
because they put code on the computer. An app can list plugins, read their logs, enable and
disable them. Store builds of Nomad Code contain no plugin host.

## Security model

- A plugin is a program of your OS user. It can read your files and use your network like any
  program you run. **Install only plugins you trust.**
- Grants limit only what **the engine** gives: `needs` decides which of the engine's tools
  (chats, fleet) the plugin may call, and the caller's grants decide who may call the plugin's
  tools. They are not a sandbox.
- The plugin reaches the engine only over the local socket (mode 0600) and is never part of
  the family of devices. Its invite is single use and expires in two minutes.
- Mark as `read` only the methods that really only read; everything else needs write.
- Ask for the least in `needs`: the person sees it at install.
