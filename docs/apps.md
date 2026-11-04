# Build your own app

An app for P2PACT is a program you hold — a phone app, a desktop app, a web page, a bot — that
shows and drives the AI agents running on your computer. This page is for the developer of
such an app. (To add tools to the engine instead, see [plugins.md](plugins.md).)

The computer side is **the P2PACT engine**. Today it ships inside Nomad Code (binary
`nomadcode`, command line `nomad`); the examples below use its `nomad` command. The protocol is
in [`spec/`](../spec); this page is the short path.

## Concepts

- **Device.** Every participant has a key pair and an address derived from it. No accounts, no
  server. Your app is a device; the computer is a device ([core §1](../spec/core.md)).
- **Kind.** An app pairs as kind `2` (client). The computer is kind `1`, a plugin is kind `3`.
- **Pairing.** Once. The computer creates a single-use invite, a string `p2pact1.…` (shown as
  text and QR by `nomad invite`, valid 10 minutes by default). Your app joins with it; both
  save each other. After that the devices find each other by themselves, over any road.
- **Wants and grants.** In the join your app says what it wants to use: a list of
  `(tool name, level)`. The person at the computer sees it, may narrow it, and answers. What is
  allowed are the **grants**, sent to your app as an event and changeable at any time. Levels:
  `1` read, `2` write; the tool name `*` means every tool. A call outside the grants fails
  with error `4`.
- **Tools.** What the computer offers ([core §8](../spec/core.md)):
  - `p2pact.chat` — chats with agents, ACP carried between devices ([chat.md](../spec/chat.md)).
  - `p2pact.fleet` — the computer, its sessions, settings, watching ([fleet.md](../spec/fleet.md)).
  - `p2pact.plugins` — installed plugins: list, log, enable, disable ([plugins.md](../spec/plugins.md)).
  - `p2pact.term` — terminals on the computer: open, watch, type, close ([term.md](../spec/term.md)).
    Write on it is a shell on the computer: ask for it only if you need it.
  - tools added by plugins, under their own names.

## Pair

On the computer:

```sh
nomad invite              # prints p2pact1.… and a QR, then waits for your app
```

Your app joins; the terminal shows its name, OS and what it asks (a request for `*` is
highlighted) and asks `y` (as asked) / `r` (the same tools, read only) / `N` (refuse). No
answer within 5 minutes is a refusal. Only kind `2` can join by such an invite.

`nomad devices` lists what the computer trusts (your app shows as `client (app)`);
`nomad devices grant <device> <tool>[:read|:write]…` and `nomad devices revoke <device>
[tool]` change it later. Revoke without a tool forgets the app entirely.

## Client SDK

The Kotlin client SDK **p2pact-client** (JVM 17, Android API 26+) wraps the three public tools:
[p2pact/p2pact-kotlin](https://github.com/p2pact/p2pact-kotlin), module `client/`, package
`io.github.p2pact.client`. The Go client is the package `client` of
[p2pact/p2pact-go](https://github.com/p2pact/p2pact-go) (see [Go](#go) below); the Swift client is the library product `P2PACTClient` of
[p2pact/p2pact-swift](https://github.com/p2pact/p2pact-swift) (see [Swift](#swift) below); clients for
TypeScript and Python are in progress. For other
languages see [the raw protocol](#raw-protocol) below. The module is built from the repository
(`./gradlew :p2pact-client:build`); depend on it as a project or an included build.

Calls return `CompletableFuture`; `.await()` (from the SDK) waits for one and throws the
failure itself. Events go to `Events<T>`: `subscribe { … }` returns a `Subscription` to close.
Listeners run on library threads, one at a time per stream, in order.

```kotlin
import io.github.p2pact.client.*
import java.io.File

val client = P2pactClient(File(dataDir), "My app")   // keys and paired computers live in dataDir
val computer = client.pair("p2pact1.…").await()       // asks DEFAULT_WANTS; see below
println(computer.name + " " + computer.state)         // ConnectionState.ONLINE / OFFLINE
println(computer.grants)                              // what the person allowed
```

`DEFAULT_WANTS` is `p2pact.chat:write`, `p2pact.fleet:write`, `p2pact.plugins:read`. Ask for
less, or for a plugin's tool, with your own list:

```kotlin
import io.github.p2pact.Grant
import io.github.p2pact.LEVEL_READ

client.pair(invite, wants = listOf(Grant(Tools.CHAT, LEVEL_READ))).await()
```

A pairing finishes before the person decides; until then the grants are empty. Watch
`client.computerEvents` (`Paired`, `Online`, `GrantsChanged`, `Forgotten`) and check
`computer.canRead(tool)` / `canWrite(tool)` to hide what is not allowed. Later runs:
`client.computers()` returns the paired ones; `computer.whenOnline()` completes at its hello.

### Chats (`computer.chat`)

```kotlin
val chat = computer.chat
val sessions = chat.listAllSessions().await()         // initialize goes first by itself
sessions.forEach { println("${it.id} ${it.state.wire} ${it.agent} ${it.title}") }

chat.updates.subscribe { u ->                         // live messages and history of loaded chats
    if (u is ChatUpdate.AgentMessage) print(u.text)
}
chat.sessionChanges.subscribe { s -> println("changed: ${s.id} ${s.state}") }

val loaded = chat.loadSession(sessions.first().id).await()   // history arrives as updates
val result = chat.prompt(sessions.first().id, "run the tests").await(0)  // ends with the turn
println(result.stopReason)

chat.permissionHandler = PermissionHandler { r ->     // the agent asks the person
    java.util.concurrent.CompletableFuture.completedFuture(r.allow())    // or r.deny()
}
chat.cancel(sessions.first().id)                      // stop the turn

val s = chat.newSession(cwd = "/home/me/project", agent = "claude", text = "hello").await()
```

`prompt` has no timeout (`await(0)` waits for ever); a `mid` is attached so that repeating a
prompt after a restart is harmless (`PromptResult.duplicate`). Other calls: `setMode`,
`setModel`, `older`, `diff`, `image`, `close`, `restart`, `hide`, `questionHandler`,
`requestAnswered`, and `request(method, params)` for any ACP or extension method without a
typed call.

### Fleet (`computer.fleet`)

```kotlin
val fleet = computer.fleet
val node = fleet.node().await()                       // the computer: name, os, status
fleet.events.subscribe { e ->
    when (e) {
        is FleetEvent.SessionsChanged -> e.sessions.forEach { println("${it.id} ${it.phase}") }
        is FleetEvent.SessionMessages -> e.messages.forEach { println(it.text) }
        is FleetEvent.Resync -> fleet.messages(e.id, after = null, limit = 50)
        is FleetEvent.Notify -> println(e.text)
        else -> {}
    }
}
val watch = fleet.watch().await()                     // snapshot in watch.sessions; close() ends
fleet.create(SessionSpec(agent = "claude", dir = "/home/me/project"), text = "start").await()
fleet.send("session-id", "continue").await()
fleet.interrupt("session-id").await()
```

Also `sessions`, `session`, `messages`, `answer`, `close`, `setKeep`, `status`, `settings`,
`setSettings` and `request` for any method.

### Plugins and plugin tools (`computer.plugins`)

```kotlin
computer.plugins.list().await().forEach { println("${it.name} ${it.status}") }
computer.plugins.log("com.example.jira", lines = 50).await()
computer.plugins.disable("com.example.jira").await()     // write
```

Call a tool a plugin added, by name (you need a grant on that tool name), and listen to its
events:

```kotlin
val out = computer.callTool("com.example.jira", 1, "PROJ-1").await()   // method 1, args as CBOR values
client.toolEvents.subscribe { e -> println("${e.tool} event ${e.event} ${e.args}") }
```

Method and event numbers belong to the plugin; its author documents them.

### Go

The same three tools in Go: `go get github.com/p2pact/p2pact-go`, package
`github.com/p2pact/p2pact-go/client`. Calls take a `context.Context` and return typed structs
that keep the JSON they were read from in `Raw`. Events are `Events[T]`: `Subscribe(func)` or
`Chan()` (a channel; values queue, so a slow reader never holds up the stream). A runnable
example is [`examples/client-cli`](https://github.com/p2pact/p2pact-go/tree/main/examples/client-cli).

```go
c, _ := client.New(dir, "My app", nil)                 // keys and paired computers live in dir
defer c.Close()
comp, _ := c.Pair(ctx, "p2pact1.…", nil)               // nil asks client.DefaultWants
fmt.Println(comp.Name(), comp.State(), comp.Grants())  // Online / Offline; what the person allowed

chat := comp.Chat()
sessions, _ := chat.ListAllSessions(ctx, "")           // initialize goes first by itself
updates, stop := chat.Updates.Chan()                   // live messages and history of loaded chats
defer stop()
chat.SetPermissionHandler(func(ctx context.Context, r *client.PermissionRequest) (client.PermissionOutcome, error) {
	return r.Allow(), nil                              // ctx ends if another client answered first
})
res, _ := chat.Prompt(ctx, sessions[0].ID, "run the tests") // ends with the turn; ctx is the only timeout

fleet := comp.Fleet()
events, stopFleet := fleet.Events.Chan()
defer stopFleet()
w, _ := fleet.Watch(ctx, false)                        // snapshot in w.Sessions; w.Close(ctx) ends
out, _ := comp.CallTool(ctx, "com.example.jira", 1, "PROJ-1") // a plugin's tool by name; CBOR results
```

A write on a read-only grant fails with `client.ErrForbidden` (`errors.Is`, or `errors.As` to
a `*client.Error` and read `Code`); the tool's own JSON-RPC errors are `*client.RPCError`.
Use `client.Wrap(node, nil)` over a node the application already runs.

## What read and write allow

A tool's spec marks each method *read* or *write*; unmarked methods are write. A read-only
app can look but not touch, and the computer sends it nothing it cannot answer.

- **chat, read:** `initialize`, `session/list`, `session/load`, the extension commands marked
  read (e.g. `_p2pact/older`, `diff`, `image`, `projects`, `agents`), and all events. **Write:**
  `session/new`, `session/prompt`, `session/cancel`, `session/set_mode`, `session/set_model`,
  close, restart, hide. The computer does not send permission requests to a read-only app.
- **fleet, read:** `fleet/hello`, `node/get`, `session/list|get|messages`, `fleet/watch`,
  `fleet/unwatch`. **Write:** everything else, including `engine/status` and `settings/get`
  (they name the data directory and the other devices).
- **plugins, read:** `plugins/list|log|grants`. **Write:** `plugins/enable|disable`.
- **term, read:** `term/list`, `term/attach`, `term/detach`, and the output. **Write:**
  `term/open`, `term/input`, `term/resize`, `term/close` — a shell on the computer.
- **Never offered over P2PACT** (error `-32601`, or `4` to a read-only app): granting and
  revoking devices, pairing, forgetting devices, and `plugins/install|remove`. They are a
  person at the computer.
- **Events** go only to devices with at least read on the tool.

### Swift

The same three tools in Swift (macOS 13+, iOS 16+): add `https://github.com/p2pact/p2pact-swift`
and use the product `P2PACTClient`. Calls are `async` and throw `P2pactCallError` / `RpcError`;
events are `Events<T>`: `subscribe { … }` or `stream()` (an `AsyncStream`; values queue). Models keep
the JSON they were read from in `raw`. A runnable example is
[`Examples/ClientCLI`](https://github.com/p2pact/p2pact-swift/tree/main/Examples/ClientCLI).

```swift
let client = try P2pactClient(dir: dataDir, name: "My app")   // keys and paired computers live in dir
let computer = try await client.pair("p2pact1.…")             // asks for defaultWants
try await computer.whenOnline()
print(computer.name, computer.state, computer.grants)         // .online / .offline; what the person allowed

let chat = computer.chat
chat.permissionHandler = { request in request.allow() }
let loaded = try await chat.loadSession("s1")                 // the whole history, in order
let result = try await chat.prompt("s1", text: "run the tests")

let fleet = computer.fleet
_ = try await fleet.watch()
for await event in fleet.events.stream() { /* … */ }
```

## Errors

P2PACT errors mean the call did not reach the tool (Kotlin: `P2pactException` subclasses; Go:
`*client.Error` with the sentinels `client.ErrForbidden` and so on):

- `1` no such tool or method (`NoMethodException`)
- `2` bad arguments · `3` not found · `4` forbidden by grants (`ForbiddenException`)
- `5` failed on the device · `6` not delivered, e.g. the plugin behind a tool is gone

The chat, fleet and plugins tools carry JSON-RPC 2.0 inside; a JSON-RPC error comes back as a
normal response with `error` (Kotlin: `RpcException`, Go: `*client.RPCError`, Swift: `RpcError`): `-32602` params, `-32601` no such method,
`-32004` no such resource (session, plugin), `-32000` failed.

An unreachable computer is not an error: calls wait for a road (a prompt waits in the queue and
is neither lost nor doubled); the call's timeout (`ClientConfig.callTimeoutMs`, 60 s; prompts
never time out) ends the wait.

## Versions

The protocol identifier is `p2pact/1`. Each tool has its own version, announced in the hello
(`{1: 1}` for chat, …). New methods and fields are only added; skip what you do not know.
The Nomad Code `contract/` folder holds an example JSON file for every message.

## Raw protocol

With no client SDK, use a P2PACT library of your language (Go, TypeScript, Python, Kotlin, Swift — they
implement pairing, sessions, roads, delivery, compression) or the spec. At the level of
calls the chat, fleet and plugins tools are the same:

1. Pair: parse the invite `p2pact1.<base64url(CBOR)>`, run the `Noise_XXpsk2` handshake with
   its secret, and send the payload `[name, boot id, hints, signing key, os, kind, wants]`
   with `kind = 2` and `wants = [["p2pact.chat", 2], …]` ([core §2](../spec/core.md)).
2. Wait for the hello (`0.2 info`, lists the tools and their numbers) and the grants
   (event `0.6`).
3. Call tool 1 method 1 (`1.1 rpc`) with the ACP request as a CBOR map, no `id`:

```
call    [0, 1, 1, 1, {"jsonrpc": "2.0", "method": "initialize",
                      "params": {"protocolVersion": 1, "clientCapabilities": {},
                                 "_meta": {"p2pact": {"ext": 1}}}}]
result  [1, 1, {"jsonrpc": "2.0", "result": {…}}]
```

4. Then `session/list`, `session/load`, `session/prompt`, … ([chat.md](../spec/chat.md)).
   Calls about one session go on its stream: the first 8 bytes of BLAKE2s(sessionId) as an
   unsigned integer, 0 replaced by 1; others on stream 0. `session/prompt` is answered when the
   turn ends; `session/update` notifications arrive as events `1.1 notify` meanwhile.
   The agent may call you (`session/request_permission`): answer it, or error `-32601` to let
   it ask elsewhere.
5. Fleet is the same with tool 2 (`2.1 rpc`, `2.1 notify`), plugins with tool 4. A plugin's
   tool has a number the computer chose (64 and up): read it from the hello's named map
   `{name: [tool, version]}` — never assume it.

Examples of every message are in `contract/tools/` of the Nomad Code repository.
