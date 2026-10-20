# Tool 3 `family` — the family of devices, v1 (draft)

Runs on top of the core (`core.md`). Devices pair once — with the family, not with every
device. *(Not yet in the reference implementation.)*

**Why.** There are more than two devices; each must know all of its own without a QR code per
pair, and losing one (even the main one) must not break the rest. This is what keeps a swarm
of agents running across your devices.

## 1. Roles

- **Parent** — exactly one, and **always a computer**, never a phone: a phone is often off,
  offline or in a pocket, and training a dictionary is too heavy for it. The parent keeps the
  family book (§2): admits, removes, appoints deputies; publishes the family dictionary (§6).
- **Deputies** — any number, **ordered** (first, second…). They can do what the parent does,
  except edit the parent or deputies above themselves. When the parent is unreachable, the
  first reachable computer deputy acts for it (§4). A phone may be a deputy (invite, edit the
  book) but never acts as parent and never becomes one.
- **Children** — everyone else. All have the same rights: everything a device announced in
  its hello (core §8). They cannot change the book.

Roles are only about **membership**. Everyone talks to everyone directly over the best live
road (core §7): the parent issues passes but is not a middleman. With the parent switched off,
two laptops still hand a chat to each other.

## 2. The family book

A signed list; every device holds a copy:

```
[generation, version, [member…], signer, signature, dictionary]
member     = [address, key, signing_key, name, role, order, added, os]
dictionary = [id, published]
```

- `role`: `0` parent, `1` deputy, `2` child; `order` — the deputy's number (1, 2…), 0 for
  others. `os` — as in the hello (`android` and `ios` are phones).
- `signing_key` — **Ed25519**, separate from the X25519 session key; created on first start
  together with it. It is part of the pairing payload (core §2).
- `generation` grows when the parent changes, `version` with every edit within a generation.
  Newer means a larger `(generation, version)`.
- `dictionary` — the current family dictionary (§6); omitted when there is none.
- The signature is Ed25519 over the CBOR of everything except the signature itself.
- The parent must be a computer: a book whose parent has a phone `os` is invalid.

**Accepting a book** — only if it is newer than ours and signed by someone allowed **by our
current book**: the parent, or a deputy (a deputy without edits to the parent or deputies
above it). Otherwise drop it and answer with our own version.

**Spreading.** Every device sends its book to a device it meets (event `3.1 book` after the
hello); the receiver keeps whichever is newer. A new edit goes to everyone reachable as event
`3.1 book`; the rest get it when they meet.

## 3. Joining and leaving

- **Invite** — only the parent or a deputy: a core invite (§2) bound to the role the newcomer
  will get (child by default).
- After pairing the inviter publishes a book with the newcomer (`version` +1) and sends it to
  everyone. The newcomer gets the book at once and meets the others by itself: their keys are
  in the book, so no pairing handshake is needed (core §4.2 with keys from the book).
- **Remove** — a book without the member; the removed device gets `0.3 bye`. The others stop
  answering it under the new book (silence to strangers, core §4.2).
- **Change a role** (child → deputy, deputy order) — a book edit by the parent.

## 4. The parent is unreachable

- "Reachable" — per core keepalives (§5): something arrived over any road within 60 s.
- The parent unreachable for **10 minutes** from every device a deputy can see → the first
  reachable **computer** deputy becomes **acting parent**: admits newcomers, edits the book,
  plans. If deputies above it are unreachable too, the next in order acts; phones are skipped.
- Edits by the acting parent are ordinary book versions (same `generation`) signed by it.
- **The parent returns** — it receives the book, sees the acting edits, accepts them (the
  deputies' signatures are valid under its own book) and keeps the book again. The acting
  deputy steps down as soon as the parent is reachable. Nothing is re-signed.
- **Concurrent edits** (the parent reachable for some devices but not others): of two books
  with the same version, take the union; for the same member the parent's edit wins; the
  parent publishes the result as the next version.
- **The parent is lost for good** — only by a human decision on any deputy: "the parent is
  lost". A computer deputy chosen by the human becomes parent, `generation` +1, the old parent
  is removed. Books of the old generation are no longer accepted from anyone. A device never
  loses the parent role automatically: a laptop off for a week or a phone in a drawer is not
  lost.

## 5. Methods and events

- `3.1 get` → the book;
- `3.2 put [book]` → `[]` — an edit by the parent or a deputy; error `4` — wrong signer,
  `3` — not newer;
- `3.3 invite [role, order?]` → invite `p2pact1.…` (parent and deputies only);
- event `3.1 book [book]` — a new version;
- event `3.2 acting [address]` — who keeps the book now (the parent or the acting deputy).

## 6. The family dictionary

The compression dictionary (core §5) trained on the family's own conversations: it compresses
better than the common one, but never leaves the family — phrases could be recovered from it.

- **Only the parent publishes it** (or the acting deputy): it trains zstd on the
  conversations it has (64 KB) and publishes a book with the new `dictionary` (`version` +1).
  The id inside the signed book guarantees the dictionary is genuine; the bytes may come from
  any family member and are checked against the id.
- When: once a month, or when compression of recent conversations dropped 15 % compared to
  the previous dictionary.
- Devices that get the book fetch the bytes with `0.5 dict [id]` from the parent or from
  anyone who has it, and announce it (`0.5 dicts`). Until then they use the common one.
- Strangers (not in the family) never get the family dictionary: `0.5 dict` → error `4`.

## 7. Open questions

- How long to wait before acting: 10 minutes is a guess.
- Read-only rights for children — later, when needed (core §8, per-tool permissions).
