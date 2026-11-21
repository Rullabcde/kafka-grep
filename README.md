# KafkaGrep

Fast multi-topic Kafka message search for humans and pipelines. Scan several topics in parallel, filter on raw bytes before parsing, and stream matches to a terminal or a small web UI.

It never joins a consumer group: no coordinator, no rebalance, no committed offsets, so it is invisible to `rpk group list` and safe to run against production.

```
$ kafkagrep -brokers=kafka.example.com:9092 -topics=news-raw \
    -q='value.id == "3f9c2a1b7d4e48f6b0a5c9e2d1f8a7b64"' -limit=1 -json

{"topic":"news-raw","partition":5,"offset":2500043,"timestamp_ms":1794742813000,...}
```

## Why it is fast

- **One client per topic, not per partition.** Connections scale with topics instead of partitions.
- **Manual partition assignment.** `kgo.ConsumePartitions` pins exact offsets, so there is no group join and no offset commit.
- **Bytes first, JSON second.** A substring check runs on the raw payload. Records that cannot contain the needle are never unmarshalled, which is what makes HTML-heavy payloads (news content, LLM annotations) cheap to scan.
- **Broker-side time jump.** `-since` is resolved by the broker into a start offset, so older segments are never transferred at all.

Measured on a 4-partition topic holding 574 MB (20k records, ~30 KB each):

| Query                                                         | Time    |
| ------------------------------------------------------------- | ------- |
| `-mode=from-start` over full history                          | ~230 ms |
| `-mode=from-start -since=5m`                                  | ~3 ms   |
| `-mode=from-start -since=1m`, matching a just-produced record | ~11 ms  |

Time is the only lever that removes bytes. Anything that keeps the same byte count (bigger fetch buffers, more workers) does not help on a bandwidth-bound scan. Raising `kgo` fetch buffers to 8 MiB/64 MiB was tested and reverted: it made the scan slower and tripled RSS.

## Quick start with Docker

The image contains the binary and its embedded fonts. It talks to any Kafka cluster; nothing else is bundled.

```bash
# Build
docker build -t kafkagrep:local .

# One-off search
docker run --rm kafkagrep:local \
  -brokers=kafka.example.com:9092 -topics=news-raw -q="error" -limit=10

# Web UI on http://localhost:8080
docker run --rm -p 8080:8080 kafkagrep:local \
  -brokers=kafka.example.com:9092 -web=:8080
```

Compose is for the web UI only, with the broker address in `KAFKA_BROKERS`:

```bash
KAFKA_BROKERS=kafka.example.com:9092 docker compose up --build
# one-off search through compose
KAFKA_BROKERS=kafka.example.com:9092 docker compose run --rm kafkagrep \
  -brokers=kafka.example.com:9092 -topics=orders -q="order-" -limit=5
```

A broker running on the Docker host is reachable as `host.docker.internal` (Compose adds the host-gateway mapping; on Linux `docker run` needs `--add-host=host.docker.internal:host-gateway`).

Tear down: `docker compose down`

## Quick start with Go

Requires Go 1.27 or newer.

```bash
go build -o kafkagrep ./cmd/kafkagrep

./kafkagrep -brokers=localhost:9092 -topics=orders -q="order-" -limit=10
```

Or without building:

```bash
go run ./cmd/kafkagrep -brokers=localhost:9092 -topics=orders -q="order-"
```

`cmd/seed` is an optional development helper that writes sample records to a topic you already have. It is not built into the image.

## CLI

```
-brokers string        comma-separated brokers (default "localhost:9092")
-topics string         comma-separated topic names
-regex string          topic name regex, e.g. "prod-.*"
-q string              query: substring, /regex/, or value.field == "x"
-mode string           from-end (default) | from-start | from-offset
-from-offset int       start offset for from-offset mode
-since string          only scan messages newer than this: 1d, 12h, 30m, RFC3339, unix seconds
-limit int             stop after this many matches (default 50)
-per-partition int     messages scanned per partition in from-end mode (default 5000)
-workers int           max concurrent topic scanners (default 32)
-ci                    case-insensitive substring matching
-json                  NDJSON on stdout, logs on stderr
-web string            serve the web UI, e.g. ":8080"
```

### Choosing a mode

| Situation                                    | Use                                      |
| -------------------------------------------- | ---------------------------------------- |
| Message is recent, you want the newest match | `-mode=from-end` (default)               |
| You know roughly when it was produced        | `-mode=from-start -since=6h`             |
| You want a bounded window in a huge topic    | `-mode=from-start -since=1d`             |
| You have an offset already                   | `-mode=from-offset -from-offset=1234567` |
| You do not know when it was produced         | `-mode=from-start`                       |

`-since` narrows any mode; it never widens the window. With `from-end` it clamps the window to the requested time, so `-mode=from-end -since=6h` scans the last 6 hours rather than the last 5000 records.

### Query syntax

| Form           | Example                         | Behaviour                      |
| -------------- | ------------------------------- | ------------------------------ |
| Substring      | `failed`                        | matches key, value, or headers |
| Regex          | `/order-[0-9]{4}/`              | Go `regexp` syntax             |
| Field equality | `value.userId == "123"`         | JSON field match               |
| Field contains | `headers.source contains "pay"` | header substring               |
| Empty          | _(blank)_                       | matches every message in range |

JSON paths use dots for nested objects: `value.user.id`.

Field expressions get a fast path: if the target string is not present in the raw payload, the JSON is not parsed at all.

## Web UI

```bash
./kafkagrep -brokers=localhost:9092 -web=:8080
```

One query bar (Topic, Filter, Start, Since, Limit), a status line with match count, scanned count, and elapsed time, a per-partition scan strip while running, and a flat result list. Click a row to expand the JSON with the matched field highlighted.

- `Enter` runs the search from any field
- `Esc` stops a running search and keeps partial results
- `/` focuses the filter field
- Last topic, filter, start, since, and limit are remembered in `localStorage`

Fonts (Geist, Geist Mono) are self-hosted under `cmd/kafkagrep/web/fonts` and embedded in the binary, so the UI works offline and needs no CDN.

## HTTP API

`GET /api/search?topics=orders,payments&q=order-&mode=from-end&limit=100&since=1d`

Streams server-sent events:

| Event      | Data                                                                                 |
| ---------- | ------------------------------------------------------------------------------------ |
| `match`    | `{topic, partition, offset, timestamp_ms, key, value, matched_field, matched_value}` |
| `progress` | `{scanned, partitions: [{topic, partition, scanned, total, done}]}`, every 250 ms    |
| `done`     | `{matches, scanned, elapsed_ms}`                                                     |
| `error`    | `{kind: "broker" \| "topic" \| "filter" \| "internal", message}`                     |

```bash
curl -N "http://localhost:8080/api/search?topics=orders&q=order-&limit=3"
```

## JSON output for pipelines

`-json` writes NDJSON to stdout and moves all logs to stderr, so it composes with `jq`:

```bash
./kafkagrep -brokers=localhost:9092 -topics=news-raw \
  -q='value.id == "3f9c2a1b..."' -limit=1 -json \
  | jq -r '.value | fromjson | .title'
```

## Project layout

```
.
├── cmd/
│   ├── kafkagrep/       CLI and web server (package main)
│   │   ├── main.go         flags, CLI output, server bootstrap
│   │   ├── search.go       scan engine: partition planning, progress
│   │   ├── filter.go       substring, regex, expression matching
│   │   ├── query.go        expression parser, JSON field extraction
│   │   ├── sse.go          SSE handler, error classification
│   │   ├── ui.go           embedded page (HTML, CSS, JS)
│   │   ├── assets.go       embedded fonts served at /fonts/
│   │   └── web/fonts/      Geist and Geist Mono (latin subsets)
│   └── seed/               optional sample data generator (dev only)
├── compose.yml             web UI against an external broker
├── Dockerfile              multi-stage build
├── go.mod
└── README.md
```

## Testing

```bash
go test ./...
```

Tests cover filtering, query parsing, offset math, progress accounting, error classification, and `-since` parsing.

End-to-end check against a real broker:

```bash
docker build -t kafkagrep:local .
docker run --rm kafkagrep:local -brokers=<broker> -topics=<topic> -q="order-" -limit=5
```

## Limitations

- The scan is a full read of the selected byte range. If the target sits in the middle of a large topic and its timestamp is unknown, everything before it must cross the network. Narrow the range with `-since`, or index `id → offset` outside Kafka.
- The web UI is single-user by design: no auth, no multi-tenancy. Do not expose it to an untrusted network.
- Scan progress is per partition; a partition that finishes early is removed from the strip.
- No TLS or SASL options. Front the tool with your own proxy if your cluster requires them.
