let ws;
let currentView = "markets"; // "markets" or "detail"
let currentSymbol = null;
let pendingAction = null; // "submit" or "cancel", set just before sending; an ack alone can't tell them apart
let pendingSubmit = null; // {side, price}, stashed just before a submit, matched to the next ack
const myOrders = new Map(); // orderId -> {id, side, price, remaining}

let marketsCache = []; // latest list_symbols result, re-rendered into whichever view is active
let chartData = []; // recent trade prices for the open symbol, oldest first
let lastTapePrice = null; // previous trade's price, for coloring the next tape row
let seenTradeIds = new Set(); // dedupes a trade that arrives via both the history backlog and the live feed

const MAX_TAPE_ROWS = 50;
const MAX_CHART_POINTS = 200;
const LADDER_ROWS_PER_SIDE = 6;

function connect() {
  ws = new WebSocket(`ws://${location.host}/ws`);
  ws.onopen = onOpen;
  ws.onclose = onClose;
  ws.onerror = () => ws.close();
  ws.onmessage = (event) => handleMessage(JSON.parse(event.data));
}

function onOpen() {
  setStatus(true);
  // A request sent on the old connection can never get its response on this new one.
  clearPending();
  refreshMarkets();
  if (currentView === "detail" && currentSymbol) send({ type: "subscribe", symbol: currentSymbol });
}

function onClose() {
  setStatus(false);
  setTimeout(connect, 1000);
}

function send(msg) {
  if (!ws || ws.readyState !== WebSocket.OPEN) return;
  ws.send(JSON.stringify(msg));
}

function handleMessage(msg) {
  switch (msg.type) {
    case "book":
      if (msg.symbol === currentSymbol) renderBook(msg.book);
      break;
    case "history":
      if (msg.symbol === currentSymbol) {
        chartData = [];
        lastTapePrice = null;
        seenTradeIds = new Set();
        document.getElementById("tape-list").innerHTML = "";
        recordTrades(msg.trades);
      }
      break;
    case "trade":
      if (msg.symbol === currentSymbol) recordTrades(msg.trades);
      break;
    case "markets":
      marketsCache = msg.markets || [];
      if (currentView === "markets") renderMarketsGrid();
      else renderDetailHeader();
      break;
    case "ack":
      if (msg.symbol === currentSymbol) handleAck(msg);
      break;
    case "error":
      showError(msg.error);
      clearPending();
      break;
  }
}

function handleAck(msg) {
  if (msg.order) {
    if (pendingAction === "cancel" || msg.order.remaining === 0) {
      myOrders.delete(msg.order.id);
    } else {
      const base = pendingSubmit || { side: "?", price: null };
      myOrders.set(msg.order.id, { id: msg.order.id, side: base.side, price: base.price, remaining: msg.order.remaining });
    }
    renderMyOrders();
  }
  clearPending();
  recordTrades(msg.trades);
}

// clearPending resets in-flight tracking and re-enables the form - called on both an ack and an
// error, since a rejected submit/cancel never reaches handleAck.
function clearPending() {
  pendingAction = null;
  pendingSubmit = null;
  setFormDisabled(false);
}

function setFormDisabled(disabled) {
  document.querySelector('#order-form button[type="submit"]').disabled = disabled;
  document.querySelectorAll("#my-orders-table button").forEach((btn) => (btn.disabled = disabled));
}

// showMarkets switches to the home view and leaves any open symbol subscription.
function showMarkets() {
  if (currentView === "detail" && currentSymbol) send({ type: "unsubscribe", symbol: currentSymbol });
  currentView = "markets";
  currentSymbol = null;
  // Any submit/cancel still in flight for the symbol we're leaving can never be acted on here.
  clearPending();
  hideError();
  document.getElementById("markets-view").classList.remove("hidden");
  document.getElementById("detail-view").classList.add("hidden");
  renderMarketsGrid();
}

// showDetail switches to symbol's detail view, resetting per-symbol UI state and (re)subscribing.
function showDetail(symbol) {
  if (currentView === "detail" && currentSymbol && currentSymbol !== symbol) {
    send({ type: "unsubscribe", symbol: currentSymbol });
  }
  currentView = "detail";
  currentSymbol = symbol;
  myOrders.clear();
  chartData = [];
  lastTapePrice = null;
  seenTradeIds = new Set();
  // Any submit/cancel still in flight for the symbol we're leaving can never be acted on here.
  clearPending();
  renderMyOrders();
  document.getElementById("tape-list").innerHTML = "";
  document.getElementById("ladder-asks").innerHTML = "";
  document.getElementById("ladder-bids").innerHTML = "";
  document.getElementById("ladder-spread-value").textContent = "-";
  drawChart();
  hideError();
  document.getElementById("markets-view").classList.add("hidden");
  document.getElementById("detail-view").classList.remove("hidden");
  document.getElementById("detail-symbol").textContent = symbol;
  resetDetailHeader();
  renderDetailHeader();
  send({ type: "subscribe", symbol });
}

function routeFromHash() {
  const symbol = decodeURIComponent(location.hash.replace(/^#\/?/, "")).trim().toUpperCase();
  if (symbol) showDetail(symbol);
  else showMarkets();
}

function refreshMarkets() {
  send({ type: "list_symbols" });
}

function submitOrder(side, orderType, price, quantity) {
  pendingAction = "submit";
  pendingSubmit = { side, price: orderType === "limit" ? price : null };
  setFormDisabled(true);
  send({ type: "submit", symbol: currentSymbol, side, orderType, price: Number(price) || 0, quantity: Number(quantity) });
}

function cancelOrder(orderId) {
  pendingAction = "cancel";
  setFormDisabled(true);
  send({ type: "cancel", symbol: currentSymbol, orderId });
}

function setStatus(connected) {
  const el = document.getElementById("status");
  el.textContent = connected ? "connected" : "disconnected";
  el.className = "status " + (connected ? "connected" : "disconnected");
}

function showError(message) {
  const el = document.getElementById("error-banner");
  el.textContent = message;
  el.classList.remove("hidden");
}

function hideError() {
  document.getElementById("error-banner").classList.add("hidden");
}

const MAX_LADDER_BAR_PX = 100;

// ladderPrices returns rows consecutive prices starting at best and stepping away from the
// spread, so the ladder shows a continuous price scale instead of only the resting levels.
function ladderPrices(best, step) {
  if (best === undefined) return [];
  return Array.from({ length: LADDER_ROWS_PER_SIDE }, (_, i) => best + step * i);
}

// renderBook draws the depth ladder into two fixed-height zones either side of a pinned spread
// row, so the spread never moves and levels never shift as the number of levels changes.
function renderBook(book) {
  const bids = book.bids || [];
  const asks = book.asks || [];
  const maxQty = Math.max(1, ...bids.map((l) => l.quantity), ...asks.map((l) => l.quantity));

  // If one side is empty, anchor its scale just past the other side's best price (an ask always
  // sits above the best bid and vice versa), so the ladder never just goes blank on one side.
  const askAnchor = asks[0] ? asks[0].price : bids[0] ? bids[0].price + 1 : undefined;
  const bidAnchor = bids[0] ? bids[0].price : asks[0] ? asks[0].price - 1 : undefined;

  const asksEl = document.getElementById("ladder-asks");
  const bidsEl = document.getElementById("ladder-bids");
  asksEl.innerHTML = "";
  bidsEl.innerHTML = "";

  const askPrices = [...ladderPrices(askAnchor, 1)].reverse();
  for (const price of askPrices) {
    asksEl.appendChild(ladderRow(asks.find((l) => l.price === price) || { price, quantity: 0 }, "ask", maxQty));
  }
  for (const price of ladderPrices(bidAnchor, -1)) {
    bidsEl.appendChild(ladderRow(bids.find((l) => l.price === price) || { price, quantity: 0 }, "bid", maxQty));
  }

  document.getElementById("ladder-spread-value").textContent = bids[0] && asks[0] ? asks[0].price - bids[0].price : "-";
}

// ladderRow renders one price tick. Ticks with nothing resting show only the price, keeping the
// scale continuous without implying a resting order that isn't there.
function ladderRow(level, side, maxQty) {
  const row = document.createElement("div");
  row.className = "ladder-row";

  let bidSide = "", askSide = "";
  if (level.quantity > 0) {
    const barPx = Math.max(2, Math.round((level.quantity / maxQty) * MAX_LADDER_BAR_PX));
    if (side === "bid") bidSide = `<span class="ladder-value">${level.quantity}</span><span class="ladder-bar" style="width:${barPx}px"></span>`;
    else askSide = `<span class="ladder-bar" style="width:${barPx}px"></span><span class="ladder-value">${level.quantity}</span>`;
  }

  row.innerHTML = `
    <div class="ladder-side bid">${bidSide}</div>
    <div class="ladder-price">${level.price}</div>
    <div class="ladder-side ask">${askSide}</div>`;
  return row;
}

// recordTrades feeds a batch of trades into both the price chart and the tape, in order. Skips
// anything already seen, since the same trade can arrive via both the history backlog and the
// live feed if it lands in the brief window between subscribing and reading the backlog.
function recordTrades(trades) {
  if (!trades || !trades.length) return;
  let added = false;
  for (const trade of trades) {
    if (seenTradeIds.has(trade.id)) continue;
    seenTradeIds.add(trade.id);
    chartData.push(trade.price);
    if (chartData.length > MAX_CHART_POINTS) chartData.shift();
    addTapeRow(trade);
    added = true;
  }
  if (added) drawChart();
}

function addTapeRow(trade) {
  const direction = lastTapePrice === null || trade.price === lastTapePrice ? "flat" : trade.price > lastTapePrice ? "up" : "down";
  lastTapePrice = trade.price;

  const list = document.getElementById("tape-list");
  const item = document.createElement("li");
  item.className = "tape-row " + direction;
  item.innerHTML = `<span class="tape-price">${trade.price}</span><span class="tape-qty">${trade.quantity}</span>`;
  list.prepend(item);
  while (list.children.length > MAX_TAPE_ROWS) list.removeChild(list.lastChild);
}

// drawChart renders chartData as a filled line, colored by net direction since the first visible point.
function drawChart() {
  const canvas = document.getElementById("price-chart");
  const ctx = canvas.getContext("2d");
  const rect = canvas.getBoundingClientRect();
  const dpr = window.devicePixelRatio || 1;
  canvas.width = rect.width * dpr;
  canvas.height = rect.height * dpr;
  ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

  const w = rect.width, h = rect.height;
  ctx.clearRect(0, 0, w, h);

  if (chartData.length < 2) {
    ctx.fillStyle = "#5a6070";
    ctx.font = "13px ui-monospace, monospace";
    ctx.fillText("waiting for trades...", 12, h / 2);
    return;
  }

  const min = Math.min(...chartData);
  const max = Math.max(...chartData);
  const pad = (max - min) * 0.1 || 1;
  const lo = min - pad, hi = max + pad;
  const color = chartData[chartData.length - 1] >= chartData[0] ? "#3ecf8e" : "#e5484d";

  const x = (i) => (i / (chartData.length - 1)) * (w - 8) + 4;
  const y = (v) => h - 4 - ((v - lo) / (hi - lo)) * (h - 8);

  ctx.beginPath();
  chartData.forEach((v, i) => (i === 0 ? ctx.moveTo(x(i), y(v)) : ctx.lineTo(x(i), y(v))));
  ctx.strokeStyle = color;
  ctx.lineWidth = 2;
  ctx.stroke();

  ctx.lineTo(x(chartData.length - 1), h);
  ctx.lineTo(x(0), h);
  ctx.closePath();
  const gradient = ctx.createLinearGradient(0, 0, 0, h);
  gradient.addColorStop(0, color + "33");
  gradient.addColorStop(1, color + "00");
  ctx.fillStyle = gradient;
  ctx.fill();
}

function renderMyOrders() {
  const tbody = document.querySelector("#my-orders-table tbody");
  tbody.innerHTML = "";
  for (const order of myOrders.values()) {
    const row = document.createElement("tr");
    const cancelCell = document.createElement("td");
    const cancelBtn = document.createElement("button");
    cancelBtn.textContent = "Cancel";
    cancelBtn.onclick = () => cancelOrder(order.id);
    cancelCell.appendChild(cancelBtn);

    row.innerHTML = `<td>${order.id}</td><td>${order.side}</td><td>${order.price ?? "market"}</td><td>${order.remaining}</td>`;
    row.appendChild(cancelCell);
    tbody.appendChild(row);
  }
}

function pctChange(m) {
  return ((m.lastPrice - m.openPrice) / m.openPrice) * 100;
}

function renderMarketsGrid() {
  const grid = document.getElementById("markets-grid");
  grid.innerHTML = "";
  if (!marketsCache.length) {
    grid.innerHTML = '<p class="muted">No symbols traded yet - waiting for activity...</p>';
    return;
  }
  for (const m of marketsCache) grid.appendChild(marketCard(m));
}

// marketCard builds the card via a template for the trusted, numeric-derived fields, then sets
// symbol/description with textContent - both are free-form strings an operator controls via
// RegisterMarket, so they must never go through innerHTML unescaped.
function marketCard(m) {
  const card = document.createElement("a");
  card.href = "#/" + encodeURIComponent(m.symbol);
  card.className = "market-card";

  const changeClass = !m.hasTraded ? "flat" : pctChange(m) >= 0 ? "up" : "down";
  const changeText = !m.hasTraded ? "-" : `${pctChange(m) >= 0 ? "+" : ""}${pctChange(m).toFixed(2)}%`;
  const priceText = m.hasTraded ? m.lastPrice : "-";
  const bidText = m.bestBid ? m.bestBid.price : "-";
  const askText = m.bestAsk ? m.bestAsk.price : "-";

  card.innerHTML = `
    <div class="market-card-top">
      <span class="market-symbol"></span>
      <span class="market-change ${changeClass}">${changeText}</span>
    </div>
    <div class="market-description"></div>
    <div class="market-price">${priceText}</div>
    <div class="market-sub">
      <span>Bid <b>${bidText}</b></span>
      <span>Ask <b>${askText}</b></span>
      <span>Vol <b>${m.volume}</b></span>
    </div>`;
  card.querySelector(".market-symbol").textContent = m.symbol;
  card.querySelector(".market-description").textContent = m.description || "";
  return card;
}

// resetDetailHeader clears stale numbers from a previously viewed symbol before the next
// markets update (or a failed subscribe) has a chance to populate them for the new one.
function resetDetailHeader() {
  document.getElementById("detail-description").textContent = "";
  document.getElementById("detail-price").textContent = "-";
  document.getElementById("detail-change").textContent = "-";
  document.getElementById("detail-change").className = "detail-change flat";
  document.getElementById("detail-high").textContent = "-";
  document.getElementById("detail-low").textContent = "-";
  document.getElementById("detail-volume").textContent = "-";
}

function renderDetailHeader() {
  const m = marketsCache.find((x) => x.symbol === currentSymbol);
  if (!m) return;

  document.getElementById("detail-description").textContent = m.description || "";
  document.getElementById("detail-price").textContent = m.hasTraded ? m.lastPrice : "-";
  const changeEl = document.getElementById("detail-change");
  if (m.hasTraded) {
    changeEl.textContent = `${pctChange(m) >= 0 ? "+" : ""}${pctChange(m).toFixed(2)}%`;
    changeEl.className = "detail-change " + (pctChange(m) >= 0 ? "up" : "down");
  } else {
    changeEl.textContent = "-";
    changeEl.className = "detail-change flat";
  }
  document.getElementById("detail-high").textContent = m.hasTraded ? m.high : "-";
  document.getElementById("detail-low").textContent = m.hasTraded ? m.low : "-";
  document.getElementById("detail-volume").textContent = m.volume;
}

document.getElementById("back-btn").addEventListener("click", () => {
  location.hash = "";
});

document.getElementById("jump-form").addEventListener("submit", (event) => {
  event.preventDefault();
  const symbol = document.getElementById("jump-input").value.trim();
  if (symbol) location.hash = "/" + symbol;
});

document.getElementById("order-form").addEventListener("submit", (event) => {
  event.preventDefault();
  const side = document.getElementById("side-select").value;
  const orderType = document.getElementById("type-select").value;
  const price = document.getElementById("price-input").value;
  const quantity = document.getElementById("quantity-input").value;
  submitOrder(side, orderType, price, quantity);
});

document.getElementById("type-select").addEventListener("change", (event) => {
  document.getElementById("price-input").disabled = event.target.value === "market";
});

window.addEventListener("hashchange", routeFromHash);
window.addEventListener("resize", () => {
  if (currentView === "detail") drawChart();
});

connect();
setInterval(refreshMarkets, 2000);
routeFromHash();
