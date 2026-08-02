# Architecture

Design decisions, trade-offs, and diagrams for Janus. For what the project is and how to run it, see [README.md](../README.md). For what's left, see [TODO.md](../TODO.md).

## Contents

- [Core matching engine](#core-matching-engine)
- [Market registration](#market-registration)
- [Concurrency model](#concurrency-model)
- [Performance](#performance)
- [gRPC API](#grpc-api)
- [Request flow](#request-flow)
- [Trade subscription and reconnection](#trade-subscription-and-reconnection)
- [Go client library (`pkg/client`)](#go-client-library-pkgclient)
- [CLI (`cmd/cli`)](#cli-cmdcli)
- [WebSocket bridge (`cmd/web`)](#websocket-bridge-cmdweb)
- [Web frontend](#web-frontend)
- [Bots](#bots)
- [Domain concepts](#domain-concepts)
- [Package layout](#package-layout)

## Core matching engine

```mermaid
classDiagram
    class Exchange {
        -mu sync.Mutex
        -engines map~string,Engine~
        -descriptions map~string,string~
        +Epoch uint64
        +Register(symbol, description) Engine
        +Lookup(symbol) Engine, bool
        +Description(symbol) string
        +Symbols() string[]
        +Close()
    }
    class OrderBook {
        +Symbol string
        -bids BookSide
        -asks BookSide
        -orders map~uint64,Order~
        -seq uint64
        -stats MarketStats
        -history tradeRing
        +BestBid() PriceLevel
        +BestAsk() PriceLevel
        +Stats() MarketStats
        +History() Trade[]
        +Order(id) Order
        +Depth(n) BookSnapshot
        +Submit(order) Trade[]~error~
        +Cancel(id) Order~error~
    }
    class BookSide {
        -side Side
        -tick tickArray
        -best int64
        -free PriceLevel[]
        +GetOrCreateLevel(price) PriceLevel
        +Level(price) PriceLevel
        +RemoveLevel(price)
        +Best() PriceLevel
        +Depth(n) PriceLevelSnapshot[]
        +IsEmpty() bool
    }
    class PriceLevel {
        -price int64
        -orders list.List
        -index map~uint64,Element~
        +Price() int64
        +Add(order)
        +Remove(orderID) bool
        +Front() Order
        +PopFront() Order
        +Len() int
        +TotalQuantity() uint64
        +IsEmpty() bool
    }
    class Order {
        +ID uint64
        +Symbol string
        +Side Side
        +Type OrderType
        +Price int64
        +Quantity uint64
        +Remaining uint64
    }
    class Trade {
        +ID uint64
        +Price int64
        +Quantity uint64
        +MakerOrderID uint64
        +TakerOrderID uint64
    }
    class Engine {
        -book OrderBook
        -inbox chan~any~
        -done chan~struct~
        -subs subscribers
        +Run()
        +Stop()
        +Submit(order) Trade[]~error~
        +Cancel(id) Order~error~
        +Depth(n) BookSnapshot
        +Subscribe() chan_Trade,func
    }
    class Server {
        -exchange Exchange
        +SubmitOrder(req) SubmitOrderResponse~error~
        +CancelOrder(req) CancelOrderResponse~error~
        +GetOrderBook(req) GetOrderBookResponse~error~
        +SubscribeTrades(req, stream) error
        +ListSymbols(req) ListSymbolsResponse~error~
        +GetTradeHistory(req) GetTradeHistoryResponse~error~
        +RegisterMarket(req) RegisterMarketResponse~error~
        +Ping(req) PingResponse~error~
    }
    class Client {
        -conn grpc.ClientConn
        -stub pb.ExchangeClient
        +Dial(addr) Client~error~
        +SubmitOrder(...) Order,Trade[]~error~
        +CancelOrder(...) Order~error~
        +GetOrderBook(...) BookSnapshot~error~
        +SubscribeTrades(...) chan_Trade~error~
        +ListSymbols(ctx) MarketSummary[]~error~
        +GetTradeHistory(ctx, symbol) Trade[]~error~
        +RegisterMarket(ctx, symbol, description) MarketSummary~error~
        +Ping(ctx) uint64~error~
        +Close() error
    }

    Exchange "1" o-- "*" Engine : keyed by symbol, one goroutine each
    Engine "1" o-- "1" OrderBook : exclusive access, via channel
    OrderBook "1" o-- "2" BookSide : bids / asks
    BookSide "1" o-- "*" PriceLevel
    PriceLevel "1" o-- "*" Order : FIFO queue
    Trade ..> Order : references MakerOrderID / TakerOrderID
    Server "1" o-- "1" Exchange : routes each request by symbol
    Client ..> Server : gRPC, over the network
```

## Market registration

**A symbol has to be listed before anything can happen to it.** `Exchange.Register(symbol, description)` is the only thing that creates an `Engine`; every other entry point - `SubmitOrder`, `CancelOrder`, `GetOrderBook`, `SubscribeTrades`, `GetTradeHistory` - calls `Exchange.Lookup(symbol)` instead, which never creates one, and the API layer returns `NotFound` if it comes up empty. This replaced an earlier design where the first order for any symbol silently created it.

**Nobody but the operator, via the CLI's `register` command, ever calls `Register`.** Bots never register a market themselves - a market-maker unilaterally deciding to list an instrument doesn't match how real exchanges work; listing is an administrative act, participants just trade what's already listed. A bot pointed at an unlisted symbol gets an ordinary `NotFound`, which its existing retry loop already tolerates.

**`Register` is idempotent.** Calling it again for an already-registered symbol is a no-op - the first description wins. This lets `Restore` (replaying a persisted snapshot) and a re-run of a startup script both call it freely without checking "does this already exist" first.

## Concurrency model

**`OrderBook` is protected by single-goroutine ownership, not a lock.** Once wrapped in an `Engine`, the *only* goroutine that ever calls `Submit`/`Cancel` - and therefore the only one that ever touches `PriceLevel.index`, `BookSide.tick`/`free`, or `OrderBook.orders` - is `Engine.Run()`'s own goroutine. Every other caller sends a command over a channel and blocks for the reply. This is Go's "share memory by communicating": instead of locking those maps, the code structurally guarantees only one goroutine can ever reach them, so there's nothing to lock.

**`Exchange` is protected by a plain `sync.Mutex` instead**, because it's a genuinely different shape of problem. `GetOrCreateEngine`'s critical section is just a map lookup (and, once per symbol, an insert plus spawning a goroutine) - nanoseconds of work, called very frequently, with no real logic to serialize. Routing that through a dedicated channel and goroutine the way `OrderBook` does would be pure overhead for no benefit. Neither approach is "more correct" in general - the right synchronization primitive follows from the shape of the critical section it's protecting.

**Shutdown doesn't close the channel callers send on.** `inbox` has many senders (every `Submit`/`Cancel`/etc. call) and exactly one closer, and sending on a closed channel panics regardless of who closed it. Instead, `Stop()` closes a separate `done` channel that every public method also watches via `select` alongside its actual send/receive - a call racing with shutdown either completes normally or returns `ErrEngineStopped`, but never panics. `Stop()` itself is wrapped in `sync.Once` since closing an already-closed channel is a separate panic.

**Trade subscriptions are broadcast, not delivered reliably.** After every `Submit`, `Run` tries to send each resulting trade to every subscriber with a non-blocking `select`/`default` - if a subscriber's buffer is full (or it isn't registered yet), that trade is dropped *for that subscriber* rather than blocking. The alternative (a blocking send) would mean one slow bot could stall matching for every other participant. This mirrors how real market data feeds work: fall behind and you miss messages, you don't get the exchange to wait for you. See [Trade subscription and reconnection](#trade-subscription-and-reconnection) for how the client side copes with this.

**Each Engine command has its own type instead of one shared struct.** `Engine`'s inbox carries `any`, and `Run` dispatches with a type-switch (`submitCommand`, `cancelCommand`, `depthCommand`, ...), each with a reply channel of exactly the type it needs. This started as one shared `result` struct; it grew unwieldy once it reached 7 fields of genuinely different shapes for 8 command kinds that each only used 2-3 of them. A small generic helper (`call[R any]`) keeps the shutdown-safe send/receive logic in one place despite each command having its own reply type.

**`Engine.BestBid`/`BestAsk` return a snapshot, not the live `PriceLevel`.** `BookSide.Best()` returns a pointer into the engine's own map - fine as long as it's only ever read from inside `Run()`'s goroutine, which was true until `ListSymbols` started calling it from the gRPC handler's goroutine while the engine kept mutating that same object on every subsequent order at that price. Found by code review, not a test: nothing exercised `ListSymbols` concurrently with active trading. Fixed with a `types.PriceLevelSnapshot{Price, Quantity}` copied while still on the engine's own goroutine, before the value ever crosses to the caller - the same "share memory by communicating" principle as everything else here, just applied one level down, where a raw pointer had leaked past the channel boundary undetected until a new caller actually exercised it concurrently with trading.

## Performance

**Benchmarks cover the full stack, not just direct calls into the book.** Alongside `Submit` (direct, and through `Engine`'s channel), benchmarks exist for `Cancel`, concurrent multi-symbol load through `Exchange` (`BenchmarkExchangeSubmit`, `b.RunParallel` spread across several symbols' `Engine`s), and a full gRPC `SubmitOrder` round-trip over an in-process `bufconn` connection - the real client/server stack, not a shortcut through the engine underneath it.

**`BookSide`'s price index is a tick array, not a sorted slice.** Inserting or removing a price level used to mean a binary search into a sorted `[]int64` plus an `O(n)` shift to keep it sorted - fine at the small book widths in `make simulate`, but it degrades as the number of distinct resting prices grows. It's now a dense array indexed directly by price (`tickArray`), giving `O(1)` lookup and insert; `BookSide` tracks the current best price directly instead of reading it off the front of a sorted slice, and scans outward from it - one direction for bids, the other for asks - when the best level is removed or `Depth` needs more than one level. That scan is only expensive if real resting prices are far apart, which real order flow generally isn't. Measured with a synthetic wide spread (100,000 distinct ticks) to make the old cost visible: `Cancel` dropped from ~770ns to ~110ns/op.

**The tick array is capped, deliberately.** A dense array indexed by raw price has one real weakness: a single order priced far from everything else would force it to allocate space for the entire gap. `maxTickPrice` (~1,048,576 ticks) bounds that, playing the same role a price collar plays on a real exchange. A limit order priced above it is rejected with `ErrPriceOutOfRange` *before* any matching starts, even though a marketable order that far out-of-range would never actually need to rest - checking early keeps `Submit` atomic (trades executed, then a rejection for the unrested remainder, was judged worse than occasionally over-rejecting a case that doesn't occur at any price real bots or an operator would use).

**Object pooling targets were identified using `pprof`.** An allocation profile of `BenchmarkEngineSubmit` showed the reply channel `make(chan submitResult, 1)` - freshly allocated on every single `Submit`/`Cancel` call - as the single largest source of engine allocations, well ahead of anything in the matching logic itself. It's now recycled with `sync.Pool`, but only put back after a real reply was received: `Engine.Run`'s own select doesn't have to prefer a pending message over a closed `done`, so a call that returns early via shutdown might still get a delayed write from the engine after it's already returned - recycling that channel could hand a stale value to a completely unrelated future call. The profile's next-largest source was an emptied `PriceLevel` being thrown away and reallocated (fresh `container/list.List`, fresh index map) the next time an order rested at that price - `BookSide` now keeps a plain freelist of them instead (no `sync.Pool` needed; a book is only ever touched by its own `Engine` goroutine), capped at 1024 entries so a market that keeps drifting to new prices forever can't turn it into an unbounded leak. Together these cut `BenchmarkEngineSubmit` from 7 allocations/op to 3.

## gRPC API

The service is defined in `proto/janus.proto` (source of truth) and generated into `internal/api/proto` via `protoc` - nothing there is hand-edited (`make proto` regenerates it). `internal/api/server.go` implements the service by routing each request to the right `Engine` via `Exchange.GetOrCreateEngine(symbol)`.

**Domain errors map to gRPC status codes.** `ErrInvalidQuantity`/`ErrInvalidPrice`/`ErrSymbolMismatch` become `InvalidArgument`, `ErrOrderNotFound` becomes `NotFound`, `ErrEngineStopped` becomes `Unavailable` - a client gets a structured status it can branch on instead of parsing an error string.

**`SubscribeTrades` is a server-streaming RPC**, not polling. It sends response headers explicitly (`stream.SendHeader`) the moment it's actually registered with the engine, *before* entering its send loop - closing a real race where a trade fired immediately after subscribing could be silently dropped by the non-blocking broadcast before the server had gotten around to registering. See [Trade subscription and reconnection](#trade-subscription-and-reconnection).

**`Ping` is a lightweight liveness/identity check.** It returns `Epoch`, a random value the `Exchange` picks once at startup. A different epoch than one previously seen is unambiguous proof the exchange restarted and lost all state - something a plain "is the connection alive" check can't tell you, since restarting a process and rebinding to the same port looks identical to a still-alive connection from the outside. The server also registers the standard `grpc.health.v1.Health` service (`google.golang.org/grpc/health`) alongside this - free interoperability with tooling like `grpcurl --health` or Kubernetes-style liveness probes, separate from our own epoch mechanism.

**`ListSymbols`, `GetTradeHistory`, and `RegisterMarket` round out the API.** `ListSymbols` lists every registered market with its live stats (used to drive the web UI's markets grid); `GetTradeHistory` returns a symbol's recent trades (used to seed its price chart on open); `RegisterMarket` is the sole entry point described in [Market registration](#market-registration). A request for a symbol that isn't registered gets `NotFound` from the API layer itself, before ever reaching an `Engine` - a different source than the engine's own `ErrOrderNotFound`, but deliberately the same status code, since from a caller's perspective both mean "the thing you asked about doesn't exist."

**Unary handlers check `ctx.Err()` before doing any work**, but don't propagate context into `Engine` itself - engine calls complete in hundreds of nanoseconds, far faster than any client could realistically observe and cancel mid-flight.

## Request flow

A `SubmitOrder` call, end to end:

```mermaid
sequenceDiagram
    participant Caller as CLI / bot
    participant Server as gRPC Server
    participant Exchange
    participant Engine as Engine goroutine
    participant Book as OrderBook

    Caller->>Server: SubmitOrder(symbol, side, price, qty)
    Server->>Exchange: GetOrCreateEngine(symbol)
    Exchange-->>Server: Engine (created on first use)
    Server->>Engine: submitCommand{order, reply}
    Engine->>Book: Submit(order)
    Book-->>Engine: trades, error
    Engine-->>Server: reply (only goroutine touching Book)
    Server-->>Caller: SubmitOrderResponse{order, trades}
```

`CancelOrder` and `GetOrderBook` follow the same shape with a different command type; `SubscribeTrades` is different enough to warrant its own diagram below.

## Trade subscription and reconnection

Subscribing, and recovering from the exchange restarting mid-session:

```mermaid
sequenceDiagram
    participant Client as pkg/client
    participant Server as gRPC Server
    participant Engine

    Client->>Server: SubscribeTrades(symbol)
    Server->>Engine: Subscribe()
    Engine-->>Server: trade channel (registered)
    Server-->>Client: response headers sent
    Note over Client: Header() unblocks - subscription confirmed live
    loop trades occur
        Engine-->>Server: broadcast(trade)
        Server-->>Client: stream.Send(trade)
    end
    Note over Server,Client: exchange process restarts
    Client->>Client: Recv() returns an error
    loop until reachable
        Client->>Server: Ping()
        Server-->>Client: new Epoch (or connection refused)
    end
    Client->>Server: SubscribeTrades(symbol) again
    Server-->>Client: response headers sent
    Note over Client: resumes delivering trades transparently
```

Two separate mechanisms make this reliable, both covered above: `SendHeader`/`Header()` closes the *registration* race (never miss a trade because the server hadn't subscribed yet), and `Ping`'s `Epoch` closes the *identity* race (know for certain the exchange restarted, not just that the connection blipped). Client-side gRPC keepalives (`Time: 5s`, `Timeout: 3s`, `PermitWithoutStream: true`) bound how long it takes to even notice the connection is dead in the first place, rather than depending on the OS to ever signal it.

## Go client library (`pkg/client`)

Unlike `internal/api` (the server), this is meant to be imported - by the CLI and by every bot. That's why it lives under `pkg/` instead of `internal/`: consumers talk to the server over the network in any language, but a Go client library specifically needs to be importable, including from a separate module later.

**Three type representations exist end to end, not two.** `internal/types` is the engine's own domain vocabulary; the generated protobuf types are the wire contract; `pkg/client`'s `Order`/`Trade`/`PriceLevel`/`BookSnapshot` are a third, hand-written set, deliberately decoupled from both, so the public API's contract can evolve independently of the wire format. `pkg/client/convert.go` is the small mapper between them.

**`SubscribeTrades` returns a plain `<-chan Trade`**, not a raw gRPC stream, and reconnects automatically on failure (see above) - every consumer just ranges over a channel; none of them need to know gRPC streaming exists, let alone that it can fail and recover.

## CLI (`cmd/cli`)

The first real consumer of `pkg/client` - a REPL, script-file runner, and one-shot command runner, nothing more. `internal/cli/parse.go` turns one line of text into a `Command` as a pure function with no I/O.

**The symbol is a required positional argument** (`janus AAPL ...`), not a flag with a default - a wrong instrument traded silently is a worse failure mode than a wrong address, which just fails loudly to connect.

**Three invocation modes**, all driving the same parse-and-dispatch code path: interactive (`janus AAPL`), script file (`janus -script f.txt AAPL`), and one-shot (`janus AAPL buy 10 @ 105`, runs once and exits - flags must come before the symbol, since the Go `flag` package stops parsing flags at the first bare argument).

**`watch` only stops on an OS signal (`Ctrl+C`)**, scoped via `signal.NotifyContext` to just that one command, so it drops back to the prompt rather than killing the whole session.

## WebSocket bridge (`cmd/web`)

```mermaid
sequenceDiagram
    participant Browser
    participant WS as internal/websocket
    participant Bridge as internal/web
    participant Client as pkg/client
    participant Server as Exchange

    Browser->>WS: HTTP Upgrade (RFC 6455 handshake)
    WS-->>Browser: 101 Switching Protocols
    Browser->>WS: {"type":"subscribe","symbol":"AAPL"}
    WS->>Bridge: ReadMessage() - text frame
    Bridge->>Client: SubscribeTrades(symbol) + GetOrderBook(symbol)
    Bridge-->>Browser: {"type":"book", ...}
    Browser->>WS: {"type":"submit", side:"buy", ...}
    Bridge->>Client: SubmitOrder(...)
    Client->>Server: gRPC SubmitOrder
    Server-->>Client: order, trades
    Bridge-->>Browser: {"type":"ack","trades":[...]}
    Client-->>Bridge: same trade, via the SubscribeTrades channel
    Bridge-->>Browser: {"type":"trade","trades":[...]}
```

**`internal/websocket` is a hand-rolled RFC 6455 implementation, not a library** - understanding the protocol is part of what this project demonstrates. It has zero knowledge of Janus: just the handshake (`Sec-WebSocket-Accept`), frame encode/decode, masking, ping/pong, and fragmentation, kept as a sibling package rather than nested under `internal/web` since nothing about it is web-specific.

**`internal/web` is the layer that knows about Janus.** It translates a small JSON schema (`submit`/`cancel`/`subscribe`/`unsubscribe` from the browser; `ack`/`trade`/`book`/`error` back) into `pkg/client` calls - the same library the CLI and every bot already use, so the browser is architecturally just another consumer, never touching the engine directly.

**A submit's own trades are embedded in its `ack`, never re-broadcast as a `trade` message.** Found live, not by a test: a connection that submits an order while also subscribed to that symbol would otherwise see the same fill twice - once as the submit's own result, once via the subscription feed. Keeping "what happened to my order" (`ack.trades`) and "the live market tape" (`trade` broadcasts) as separate concerns fixes it structurally rather than by deduplicating after the fact.

`cmd/web` is a separate binary, like every bot - it dials the exchange over gRPC via `pkg/client`, with no special access.

## Web frontend

Vanilla JS/HTML/CSS, no framework, no build step - embedded into the `cmd/web` binary via `embed.FS` (`internal/web/static`) and served alongside the WebSocket endpoint, one binary, one port.

![Markets home view: a live-updating grid of every registered symbol](img/markets.png)

**Two views, no page reloads.** A "Markets" home view lists every registered symbol as a live-updating card (price, % change since session open, best bid/ask, volume), polling `list_symbols` every 2 seconds. Clicking one switches - via `#/SYMBOL` hash routing, so it's back-button- and bookmark-friendly - to a per-symbol detail view: a hand-rolled canvas price chart seeded from `GetTradeHistory` on subscribe, a colored trade tape (green/red by uptick/downtick), an order-entry form, and a depth ladder.

![Symbol detail view: price chart, depth ladder, trade tape, and order entry](img/detail.png)

**The depth ladder shows a continuous price scale, not just resting levels.** Real order books have gaps between resting prices; showing only the actual levels left the panel mostly blank and made the spread's position jump around as the number of levels changed. Each render synthesizes a fixed number of consecutive price ticks outward from the best bid/ask (falling back to just past the *other* side's best price if one side is completely empty), with a bar only appearing where an order actually rests - the spread stays pinned at a fixed vertical position no matter how many real levels currently exist.

**Every ack, trade, book, and history message is tagged with its symbol, and the client drops anything that doesn't match the symbol it's currently viewing.** A late response for a symbol you've since navigated away from would otherwise get misattributed to whatever you're looking at now - including a cancel button sending a cancel for the wrong symbol's order ID, since order IDs are per-symbol sequences and a collision isn't an edge case. Navigating away also clears any submit/cancel still in flight, for the same reason a dropped WebSocket connection does: the original request's response can never arrive on a new connection.

**Trade IDs de-duplicate the history-backlog/live-feed overlap.** Subscribing does three things in sequence - register the live feed, fetch trade history, fetch the book - and a trade landing in the gap between the first two would otherwise show up twice, once via history and once live. The client tracks trade IDs it's already rendered per symbol and skips repeats.

**Symbol and description are rendered with `textContent`, never `innerHTML`.** Both are free-form strings an operator controls via `RegisterMarket` - not something the browser should ever trust enough to parse as markup. Found by code review: an early version interpolated them straight into a template string.

## Bots

Five independent programs, each a standalone `pkg/client` consumer, trading against each other and any human via the CLI to create organic price movement - this is what makes Janus a market instead of just an order book with an API in front of it.

```mermaid
classDiagram
    class Strategy {
        <<interface>>
        +Act(ctx, out) error
    }
    class Closer {
        <<interface>>
        +Close(ctx, out)
    }
    class PriceSource {
        <<interface>>
        +Price(ctx) int64, error
    }
    class Trader {
        -strategy Strategy
        -interval Duration
        +Run(ctx, out) error
    }
    class Quoter {
        -source PriceSource
        -bidID uint64
        -askID uint64
        +Act(ctx, out) error
        +Close(ctx, out)
    }
    class RandomWalk {
        +Price(ctx) int64, error
    }
    class SpotMidPriceSource {
        -basis int64
        -last int64
        +Price(ctx) int64, error
    }
    class Hedger {
        -position int64
        +Act(ctx, out) error
    }
    class Noise {
        +Act(ctx, out) error
    }
    class Arbitrage {
        -inPosition bool
        -long bool
        +Act(ctx, out) error
    }

    Trader o-- Strategy : runs Act on a timer
    Quoter ..|> Strategy
    Quoter ..|> Closer : cancels resting orders on shutdown
    Quoter o-- PriceSource
    RandomWalk ..|> PriceSource
    SpotMidPriceSource ..|> PriceSource
    Hedger ..|> Strategy
    Noise ..|> Strategy
    Arbitrage ..|> Strategy
```

**`Trader` + `Strategy` is the one runner for every bot**, maker and taker alike. `Strategy` is one decision-and-trade cycle; `Closer` is an optional extension (checked via type assertion, like `io.Closer`) for bots that need cleanup before exiting - only `Quoter` needs it, to cancel its resting orders rather than leaving stale liquidity behind.

```mermaid
sequenceDiagram
    participant Trader
    participant Strategy
    participant Client as pkg/client
    participant Server as Exchange

    loop every interval
        Trader->>Strategy: Act(ctx, out)
        Strategy->>Client: SubmitOrder / CancelOrder
        Client->>Server: gRPC call
        Server-->>Client: response
    end
    Note over Trader: ctx cancelled (Ctrl+C)
    opt Strategy implements Closer
        Trader->>Strategy: Close(ctx, out)
        Strategy->>Client: cancel resting orders
    end
```

**The five bots and what each one does:**

- **`spot`** - `Quoter` + `RandomWalk`. Continuously quotes both sides of the spot book around a synthetic reference price, since there's no external market to derive one from yet. Cancels its old quotes *before* submitting new ones each cycle; submitting first would let a fast-moving reference price cross the bot's own still-resting orders (a self-trade).
- **`futures`** - `Quoter` + `SpotMidPriceSource`. Same quoting loop, but its reference price is the spot book's mid-price plus a fixed basis, coupling the futures instrument to the spot market. Holds the last known-good price during a momentary gap in the spot book (mid-requote) rather than a fixed fallback.
- **`hedger`** - takes random flow positions in futures (standing in for "some reason to have a position," deliberately not a real signal) and, once net exposure crosses a threshold, flattens it with an offsetting trade in spot. Tracks one combined exposure number rather than separate futures/spot books, since the two instruments are defined to move together.
- **`noise`** - stateless; submits random-direction market orders across whichever symbols it's configured for, adding uninformed volume with no view.
- **`arbitrage`** - watches the spot/futures spread against a configured fair basis, enters a position (both legs) once the deviation crosses a threshold, and closes once the spread has reverted halfway back - a fixed hysteresis band rather than a separate config knob, so it doesn't flip-flop right at the entry boundary.

**Order sizes are randomized, not fixed.** `bots.JitterQuantity` scales a configured base quantity by roughly ±50% on every order - a book where every resting order is exactly the same size doesn't look or behave like a real one. Arbitrage's two legs still share one jittered value per trade, since they need to stay balanced against each other; only the size varies *trade to trade*, never *within* a pair.

```mermaid
flowchart LR
    Spot[spot] -->|quotes around random walk| AAPL[(AAPL book)]
    Futures[futures] -->|reads mid price| AAPL
    Futures -->|quotes around spot mid + basis| AAPLF[(AAPLF book)]
    Hedger[hedger] -->|random flow| AAPLF
    Hedger -->|hedges net exposure| AAPL
    Noise[noise] -->|random market orders| AAPL
    Noise -->|random market orders| AAPLF
    Arbitrage[arbitrage] -.->|watches spread| AAPL
    Arbitrage -.->|watches spread| AAPLF
    Arbitrage -->|trades both legs on mispricing| AAPL
    Arbitrage -->|trades both legs on mispricing| AAPLF
```

## Domain concepts

**Price is an integer, never a float.** `Price` is stored as an integer number of ticks, not `float64` - floats introduce rounding error that's unacceptable once you're summing trade values.

**Quantity vs. Remaining.** `Quantity` is the original size requested and never changes; `Remaining` is how much is still unmatched and decreases with partial fills. This mirrors `OrderQty`/`LeavesQty` in FIX, the real-world protocol most exchanges use.

**Maker vs. Taker.** The **maker** was already resting on the book (provided liquidity); the **taker** is the incoming order that crossed the spread (consumed liquidity). A trade always executes at the maker's price.

**`ID` is engine-assigned and doubles as the ordering key.** Assigned from an internal monotonic counter, not supplied by the caller, so collisions are structurally impossible. There's no separate timestamp field - since the counter is strictly increasing, `ID` alone already answers "did this happen before that," the same way a Kafka offset does.

**`Depth` is an L2 view, not L3.** Reports a price and its *total* resting quantity per level, not the individual orders making it up - the same distinction real market data feeds draw between L2 (aggregated depth) and L3 (every individual order).

## Package layout

```
cmd/
    server/                    gRPC server entrypoint
    cli/                       CLI entrypoint
    web/                       web UI entrypoint: serves the embedded frontend + WebSocket bridge
    bots/strategies/
        spot/, futures/,
        hedger/, noise/,
        arbitrage/             one thin entrypoint per bot
internal/
    types/                     engine's own domain vocabulary, incl. MarketStats
    engine/                    OrderBook, BookSide, PriceLevel, Engine, Exchange, trade history ring
    api/                       gRPC server implementation
        proto/                 generated code (never hand-edited)
    persistence/               snapshot save/load/restore, background save loop
    websocket/                 hand-rolled RFC 6455 transport (no Janus knowledge)
    web/                       browser JSON <-> pkg/client bridge, plus the embedded frontend
        static/                index.html, app.js, style.css - the actual page a browser loads
    cli/                       parse/run/format for the CLI
    bots/                      shared bot infrastructure: Quoter/PriceSource, Trader/Strategy, JitterQuantity
        strategies/
            spot/, futures/    PriceSource implementations
            hedger/, noise/,
            arbitrage/         Strategy implementations
pkg/
    client/                    public, importable Go client for the gRPC API
proto/
    janus.proto                service definition, source of truth
scripts/
    simulate.sh                one-command demo: server + web UI + a full bot fleet across 12 markets
docs/
    ARCHITECTURE.md            this file
    img/                       screenshots and the demo GIF used in the docs
```
