# futuresooth

One place to monitor U.S. government data. A small, dependency-free Go project in
two parts:

- **API service** (`cmd/server`) — aggregates data from the U.S. Bureau of Labor
  Statistics (BLS), merges related series onto a shared monthly axis, caches to
  disk, and serves clean **chart-ready JSON**. Also writes descriptive per-view
  JSON + spreadsheet-friendly CSV files.
- **Web service** (`cmd/web`) — a responsive, mobile-first viewer that renders the
  charts with [Observable Plot](https://observablehq.com/plot/).

First dataset: the U.S. unemployment rate (overall + by sex, race/ethnicity, and
age), full history back to 1948.

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

## Configuration

Settings are split by concern across two files in the working directory:

- **`.config`** — non-secret settings, committed.
- **`.env`** — secrets, gitignored (create it yourself; never commit).

A real environment variable overrides either. Create `.env`:

```sh
cat > .env <<'EOF'
BLS_API_KEY=your_key_here   # optional; free at https://data.bls.gov/registrationEngine/
ADMIN_TOKEN=
EOF
```

| Variable           | File      | Default                 | Service | Meaning                                   |
| ------------------ | --------- | ----------------------- | ------- | ----------------------------------------- |
| `BLS_API_KEY`      | `.env`    | _(none)_                | api     | BLS v2 key (optional; keyless = lower limits) |
| `ADMIN_TOKEN`      | `.env`    | _(none)_                | api     | Enables `POST /admin/refresh` if set      |
| `HTTP_ADDR`        | `.config` | `:8080`                 | api     | API listen address                        |
| `DATA_DIR`         | `.config` | `./data`                | api     | Where cache + CSV/JSON exports are written |
| `REFRESH_INTERVAL` | `.config` | `360h`                  | api     | How often to re-pull series (~twice a month) |
| `START_YEAR`       | `.config` | `1948`                  | api     | First year of history (end = this year)   |
| `FRONTEND_ADDR`    | `.config` | `:3000`                 | web     | Web UI listen address                     |
| `BACKEND_URL`      | `.config` | `http://localhost:8080` | web     | API base URL the web UI fetches from      |

## Run

```sh
export PATH="$PATH:/usr/local/go/bin"   # Go lives here on this machine

# Terminal 1 — API service (populates ./data on first run)
go run ./cmd/server

# Terminal 2 — web UI
go run ./cmd/web
# open http://localhost:3000
```

## API routes (`cmd/server`)

| Path                   | Description                          |
| ---------------------- | ------------------------------------ |
| `GET /api/views`       | List curated views                   |
| `GET /api/views/{key}` | Chart-ready merged data for a view   |
| `GET /api/series/{id}` | A single normalized series           |
| `POST /admin/refresh`  | Force a refresh (needs `X-Admin-Token`) |
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

## Extending

- **More data:** add series IDs to `internal/bls/catalog.go`.
- **More charts:** add a `View` to `internal/aggregate/view.go`.
- **More datasets (web):** add a `Dataset` to `internal/web/datasets.go`.
