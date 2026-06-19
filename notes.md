# Notes

Notes about this project

## Architecture

The web service fetches chart JSON from the API service **server-side** and embeds
it into the page, so the browser is always same-origin: **no CORS**, and the API
service can stay bound to localhost on a VPS (only the web service is public).
Both binaries live in one Go module and share the `internal/aggregate` types, so
there is a single definition of `View`/`ChartData`.

```
cmd/server            API service entrypoint
cmd/web               web UI entrypoint
internal/
  bls                 BLS API v2 client, response normalization, series catalog
  cache               in-memory series cache
  aggregate           curated views + merge onto a shared axis (THE shared types)
  export              descriptive per-view JSON + CSV files (also the restore source)
  refresh             background refresher
  httpapi             the API service's HTTP handlers
  apiclient           web service's client for the API (TTL-cached)
  web                 web service routes, handlers, templates, embedded static
  config              shared config loader (.config + .env)
```

## API routes (`cmd/server`)

| Path                   | Description                          |
| ---------------------- | ------------------------------------ |
| `GET /api/views`       | List curated views                   |
| `GET /api/views/{key}` | Chart-ready merged data for a view   |
| `GET /api/series/{id}` | A single normalized series           |
| `POST /admin/refresh`  | Force a refresh (need `Admin-Token`) |
| `GET /healthz`         | Liveness                             |

## Web routes (`cmd/web`)

| Path             | Description                       |
| ---------------- | --------------------------------- |
| `/`              | Datasets                          |
| `/d/{dataset}`   | All charts in a dataset, inline   |
| `/v/{key}`       | A single chart (deep link)        |
| `/healthz`       | Liveness                          |

## Test

```sh
go test ./...
```