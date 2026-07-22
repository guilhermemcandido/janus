# Janus

A high-performance order matching engine simulating a financial exchange, written in Go.

Implements an in-memory order book with price-time priority matching for limit and market orders. Concurrency (a channel-based model) and an API/CLI surface are planned but not built yet — see Status below.

## Architecture

Updated as implementation progresses — currently reflects everything built so far: price-time priority matching exists; concurrency, CLI, and REST API do not yet.

```mermaid
classDiagram
    class Exchange {
        -books map~string,OrderBook~
        +GetOrCreateBook(symbol) OrderBook
    }
    class OrderBook {
        +Symbol string
        -bids BookSide
        -asks BookSide
        -orders map~uint64,Order~
        -seq uint64
        +BestBid() PriceLevel
        +BestAsk() PriceLevel
        +Order(id) Order
        +Submit(order) Trade[]~error~
    }
    class BookSide {
        -side Side
        -levels map~int64,PriceLevel~
        -prices int64[]
        +GetOrCreateLevel(price) PriceLevel
        +RemoveLevel(price)
        +Best() PriceLevel
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

    Exchange "1" o-- "*" OrderBook : keyed by symbol
    OrderBook "1" o-- "2" BookSide : bids / asks
    BookSide "1" o-- "*" PriceLevel
    PriceLevel "1" o-- "*" Order : FIFO queue
    OrderBook "1" o-- "*" Order : orders map, by ID
    Trade ..> Order : references MakerOrderID / TakerOrderID
```

## Domain concepts

**Price is an integer, never a float.** `Price` is stored as an integer number of ticks (the smallest price increment), not `float64`. Floats introduce rounding error that's unacceptable once you're summing trade values — this is standard practice in real trading systems.

**Quantity vs. Remaining.** `Quantity` is the original size the client asked for and never changes. `Remaining` is how much of that order is still unmatched, and decreases as partial fills happen. Both are needed because an order is rarely filled in one shot — it's usually pieced together from several smaller counter-orders over one or more matching passes, so the engine needs a running counter (`Remaining`) separate from the original request (`Quantity`). This mirrors `OrderQty`/`LeavesQty` in FIX, the real-world protocol most exchanges use for order messaging.

**Maker vs. Taker.** When two orders match, the **maker** is the order that was already resting on the book (it provided liquidity); the **taker** is the incoming order that crossed the spread and caused the match (it consumed liquidity). A trade always executes at the maker's price. This distinction is the basis for maker/taker fee schedules on real exchanges, and for a market-making strategy specifically, tracking your own maker-fill ratio is how you'd measure whether you're actually providing liquidity.

**`ID` is engine-assigned and doubles as the ordering key.** `Order.ID` and `Trade.ID` are assigned by the engine from an internal monotonic counter, not supplied by the caller — this makes collisions structurally impossible rather than something to detect and reject. There's no separate timestamp field: since the counter is strictly increasing, `ID` alone already answers "did this happen before that?" (`a.ID < b.ID`), the same way a Kafka offset or a database log sequence number serves as both a unique identifier and an ordering key at once.

## Status

Core matching is implemented and tested: `OrderBook.Submit` matches limit and market orders by price-time priority, assigns each order's `ID` itself (so IDs can't collide or be spoofed by a caller), validates input (rejects zero quantity and mismatched symbols), and returns an error rather than failing silently. Not yet built: order cancellation, concurrency, CLI, REST API.
