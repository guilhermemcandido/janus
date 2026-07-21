# Janus

A high-performance order matching engine simulating a financial exchange, written in Go.

Implements an in-memory order book with price-time priority matching, limit and market orders, and a channel-based concurrency model. Built to explore matching engine design and market microstructure.

## Architecture

Updated as implementation progresses — currently reflects everything built so far, with no matching logic yet.

```mermaid
classDiagram
    class Exchange {
        -books map~string,OrderBook~
        +Book(symbol) OrderBook
    }
    class OrderBook {
        +Symbol string
        +Bids BookSide
        +Asks BookSide
        +Orders map~uint64,Order~
        -seq uint64
        +Submit(order) Trade[]
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
        +Price int64
        -orders list.List
        -index map~uint64,Element~
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
        +Timestamp int64
    }
    class Trade {
        +ID uint64
        +Timestamp int64
        +Price int64
        +Quantity uint64
        +MakerOrderID uint64
        +TakerOrderID uint64
    }

    Exchange "1" o-- "*" OrderBook : keyed by symbol
    OrderBook "1" o-- "2" BookSide : Bids / Asks
    BookSide "1" o-- "*" PriceLevel
    PriceLevel "1" o-- "*" Order : FIFO queue
    OrderBook "1" o-- "*" Order : Orders map, by ID
    Trade ..> Order : references MakerOrderID / TakerOrderID
```

## Domain concepts

**Price is an integer, never a float.** `Price` is stored as an integer number of ticks (the smallest price increment), not `float64`. Floats introduce rounding error that's unacceptable once you're summing trade values — this is standard practice in real trading systems.

**Quantity vs. Remaining.** `Quantity` is the original size the client asked for and never changes. `Remaining` is how much of that order is still unmatched, and decreases as partial fills happen. Both are needed because an order is rarely filled in one shot — it's usually pieced together from several smaller counter-orders over one or more matching passes, so the engine needs a running counter (`Remaining`) separate from the original request (`Quantity`). This mirrors `OrderQty`/`LeavesQty` in FIX, the real-world protocol most exchanges use for order messaging.

**Maker vs. Taker.** When two orders match, the **maker** is the order that was already resting on the book (it provided liquidity); the **taker** is the incoming order that crossed the spread and caused the match (it consumed liquidity). A trade always executes at the maker's price. This distinction is the basis for maker/taker fee schedules on real exchanges, and for a market-making strategy specifically, tracking your own maker-fill ratio is how you'd measure whether you're actually providing liquidity.

## Status

Early development.
