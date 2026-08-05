FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN mkdir /out \
    && go build -o /out/server ./cmd/server \
    && go build -o /out/cli ./cmd/cli \
    && go build -o /out/web ./cmd/web \
    && go build -o /out/spot ./cmd/bots/strategies/spot \
    && go build -o /out/futures ./cmd/bots/strategies/futures \
    && go build -o /out/hedger ./cmd/bots/strategies/hedger \
    && go build -o /out/noise ./cmd/bots/strategies/noise \
    && go build -o /out/arbitrage ./cmd/bots/strategies/arbitrage

FROM alpine:3.20
RUN addgroup -S janus && adduser -S janus -G janus \
    && mkdir -p /data && chown janus:janus /data
COPY --from=build /out/ /app/
USER janus
WORKDIR /app
