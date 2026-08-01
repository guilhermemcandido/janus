# TODO

Ongoing and planned work for Janus. See [README.md](README.md) for the project pitch and [ARCHITECTURE.md](ARCHITECTURE.md) for how the finished pieces work.

## Done

- **Core matching engine** — price-time priority order book, limit and market orders, cancel, L2 depth snapshots. Property-tested (quantity conservation, book never crosses, FIFO priority) and benchmarked.
- **Concurrency** — channel-based `Engine` (single-goroutine ownership per symbol), mutex-based `Exchange` (goroutine-per-symbol sharding), safe shutdown, non-blocking trade broadcast. Race-tested under concurrent load.
- **gRPC API** — `SubmitOrder`, `CancelOrder`, `GetOrderBook`, `SubscribeTrades` (streaming), `Ping`. Domain errors map to proper status codes. Standard `grpc.health.v1.Health` service registered alongside.
- **Go client library (`pkg/client`)** — public, importable, decoupled types, client-side keepalives, auto-reconnecting trade subscription.
- **CLI (`cmd/cli`)** — interactive, script-file, and one-shot modes, all driving the same parser/dispatcher.
- **Bots** — `spot` and `futures` market makers, `hedger`, `noise` trader, `arbitrage` bot, all sharing one `Trader`/`Strategy` runner.
- **Reliability hardening** — negative-depth panic fixed at all three layers (engine/API/CLI), server graceful shutdown, `Quoter` no longer loses track of an order on a transient cancel error, bounded shutdown timeout on bot cleanup.
- **Tooling** — `Makefile` (`make help` for the full list): build, test, vet, fmt, proto regen, and quiet `run-*` targets for every binary.
- **Bots detect an exchange restart and reset local state.** `Trader` pings the exchange every cycle and, on a detected epoch change, calls `Reset` on any strategy that implements it. `Quoter`, `Hedger`, and `Arbitrage` all implement it, dropping resting order IDs / net position instead of acting on stale assumptions. Verified live (kill/restart the server mid-session).
- **Stress test: concurrent client reconnections.** Several clients hammering the server concurrently, kill/restart mid-flight, assert everyone recovers and each book ends up consistent. Found and fixed a real data race along the way: a resting order's shared pointer could be mutated by a later match on another goroutine's request while the original caller was still reading it for its own gRPC response — fixed by having the book store a private copy once an order rests.

## In progress / next up

Nothing currently in flight.

## Planned

- **Web UI.** A single hand-rolled WebSocket (RFC 6455, over HTTP/1.1) carrying both order commands and live pushes from the browser — one connection, since a resting order's eventual fill is inherently asynchronous and needs per-connection routing regardless of how the order was placed. Comes after the reliability work above, once there's real multi-bot activity worth visualizing.
- **Performance pass.** Benchmarks exist for `Submit` (direct, and through `Engine`'s channel); still open: broader coverage (`Cancel`, multi-symbol `Exchange`, gRPC round-trip), a tick-array price index (replacing the sorted-slice-plus-binary-search index), and object pooling to reduce GC pressure under sustained load.
- **Snapshot persistence.** Currently a server restart loses the entire order book. Needed both for its own sake and to make the restart-recovery story above fully meaningful (right now, recovery means "reconnect to an empty exchange," not "resume where you left off").

## Later — separate projects, sequenced one at a time

Not part of this repo. Deliberate focus choice: finish Janus completely (all bots, then the UI) before starting either.

- **Rust port** of just the core matching engine (`OrderBook`/`PriceLevel`/`BookSide`/`Submit`/`Cancel`, no gRPC/bots/CLI layer) — a new repo, benchmarked against Go to demonstrate the GC-pause-vs-no-GC latency difference.
- **OCaml port** (optional, lower priority) — same scope as the Rust port, if pursued at all. OCaml's trading-industry footprint is concentrated at Jane Street specifically, not an industry-wide expectation, so this is a nice-to-have rather than a gap to close.
