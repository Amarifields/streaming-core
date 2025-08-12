# streaming-core

Streaming, done simply. A compact Go service that emits a continuous stream of numbers over Server‑Sent Events (SSE). Designed for clarity, testability, and production readiness.

## Features

- Incrementing integer feed over SSE
- Configurable timing and CORS via environment
- Resume support with `Last-Event-ID`
- Optional `start` and `limit` query params
- Graceful shutdown on SIGINT/SIGTERM

## API

`GET /stream`

Sends `event: number` messages; each message contains the current integer. The `id` equals the integer value to enable simple resume.

Query params:

- `intervalMs`: integer; delay between events. Default: 100
- `start`: integer; first number to emit. Default: 0
- `limit`: integer; maximum messages before the stream ends. Default: unlimited

Headers:

- `Last-Event-ID`: resume from the next integer after this id

Other endpoints:

- `/` index
- `/health` liveness probe

## Configuration

Environment variables:

- `PORT` server port. Default: 8080
- `STREAM_INTERVAL_MS` default emit interval. Default: 100
- `CORS_ALLOW_ORIGIN` value for `Access-Control-Allow-Origin`. Default: `*`

## Getting started

```bash
go run .
```

```bash
curl -N http://localhost:8080/stream
```

Custom interval and bounded stream:

```bash
curl -N "http://localhost:8080/stream?intervalMs=250&start=10&limit=5"
```

Browser example:

```html
<script>
  const s = new EventSource('/stream');
  s.addEventListener('number', e => console.log(e.lastEventId, e.data));
</script>
```

## Production notes

- SSE requires response streaming; ensure proxies do not buffer
- Prefer a process manager to forward signals for clean shutdown
- For cross‑origin use, pin `CORS_ALLOW_ORIGIN` to known origins

## License

MIT
