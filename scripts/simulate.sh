#!/usr/bin/env bash
# Runs a full Janus demo: the exchange, the web UI, and a handful of bots trading
# spot, futures, hedging, and arbitrage across several tickers. Ctrl+C stops everything.
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="$ROOT/bin"
ADDR="${ADDR:-localhost:50051}"
LISTEN="${LISTEN:-:8080}"

pids=()
spawn() {
	"$@" &
	pids+=($!)
}

# wait_for_port blocks until host:port accepts a TCP connection, or timeout_secs elapses - a fixed
# sleep can't tell "still starting up" from "already listening" on a slower machine.
wait_for_port() {
	local host=${1%:*} port=${1##*:} timeout_secs=$2
	local waited=0
	until (exec 3<>"/dev/tcp/$host/$port") 2>/dev/null; do
		sleep 0.2
		waited=$((waited + 1))
		if [ "$waited" -ge "$((timeout_secs * 5))" ]; then
			echo "timed out waiting for $host:$port" >&2
			return 1
		fi
	done
	exec 3>&- 3<&-
}

cleaned_up=0
cleanup() {
	if [ "$cleaned_up" -eq 1 ]; then
		return
	fi
	cleaned_up=1
	echo
	echo "stopping simulation..."
	kill "${pids[@]}" 2>/dev/null
	wait 2>/dev/null
	exit 0
}
trap cleanup EXIT INT TERM

spawn "$BIN/server" -addr "$ADDR"
wait_for_port "$ADDR" 10

spawn "$BIN/web" -addr "$ADDR" -listen "$LISTEN"
wait_for_port "localhost${LISTEN}" 10

"$BIN/cli" -addr "$ADDR" AAPL register Apple Inc. spot equity
"$BIN/cli" -addr "$ADDR" MSFT register Microsoft Corp. spot equity
"$BIN/cli" -addr "$ADDR" GOOG register Alphabet Inc. spot equity
"$BIN/cli" -addr "$ADDR" AMZN register Amazon.com Inc. spot equity
"$BIN/cli" -addr "$ADDR" NVDA register NVIDIA Corp. spot equity
"$BIN/cli" -addr "$ADDR" META register Meta Platforms Inc. spot equity
"$BIN/cli" -addr "$ADDR" AAPLF register AAPL futures contract
"$BIN/cli" -addr "$ADDR" MSFTF register MSFT futures contract
"$BIN/cli" -addr "$ADDR" GOOGF register GOOG futures contract
"$BIN/cli" -addr "$ADDR" AMZNF register AMZN futures contract
"$BIN/cli" -addr "$ADDR" NVDAF register NVDA futures contract
"$BIN/cli" -addr "$ADDR" METAF register META futures contract

spawn "$BIN/spot" -addr "$ADDR" AAPL
spawn "$BIN/spot" -addr "$ADDR" -initial-price 300 MSFT
spawn "$BIN/spot" -addr "$ADDR" -initial-price 140 GOOG
spawn "$BIN/spot" -addr "$ADDR" -initial-price 180 AMZN
spawn "$BIN/spot" -addr "$ADDR" -initial-price 120 NVDA
spawn "$BIN/spot" -addr "$ADDR" -initial-price 250 META
sleep 1

# fallback-price matches each spot's initial-price, so a futures contract's first trade isn't at a
# wildly wrong reference before its spot book is up (which would otherwise show as a fake spike).
spawn "$BIN/futures" -addr "$ADDR" -spot AAPL -fallback-price 100 AAPLF
spawn "$BIN/futures" -addr "$ADDR" -spot MSFT -fallback-price 300 MSFTF
spawn "$BIN/futures" -addr "$ADDR" -spot GOOG -fallback-price 140 GOOGF
spawn "$BIN/futures" -addr "$ADDR" -spot AMZN -fallback-price 180 AMZNF
spawn "$BIN/futures" -addr "$ADDR" -spot NVDA -fallback-price 120 NVDAF
spawn "$BIN/futures" -addr "$ADDR" -spot META -fallback-price 250 METAF

spawn "$BIN/hedger" -addr "$ADDR" -futures AAPLF -spot AAPL
spawn "$BIN/arbitrage" -addr "$ADDR" -spot AAPL -futures AAPLF
spawn "$BIN/noise" -addr "$ADDR" -symbols AAPL,MSFT,GOOG,AMZN,NVDA,META,AAPLF,MSFTF,GOOGF,AMZNF,NVDAF,METAF

url="http://localhost${LISTEN}"
echo
echo "janus simulation running - open $url in your browser"
echo "press Ctrl+C to stop everything"
echo

if command -v open >/dev/null; then
	open "$url"
elif command -v xdg-open >/dev/null; then
	xdg-open "$url"
fi

wait
