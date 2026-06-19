# futuresooth

One place to monitor all the data that matters and pertains to the future. 
A small, dependency-free Go project in two parts:

- **API service** (`cmd/server`) — aggregates data from various open and government sources, 
  merges related series and serves them as clean **chart-ready JSON**. 
  Also writes descriptive per-view JSON + spreadsheet-friendly CSV files.
  TODO: compressed binary data format for efficient data transfer 

- **Web service** (`cmd/web`) — a responsive web app that renders the
  charts with [Observable Plot](https://observablehq.com/plot/).
  TODO: chart controls and rendering high 

Datasets:

- **Unemployment Rate** — U.S. unemployment (overall + by sex, race/ethnicity,
  and age), monthly, full history back to 1948, from the
  [Bureau of Labor Statistics](https://www.bls.gov/).
- **Space Industry** — worldwide orbital launch activity (launches by country,
  mass to orbit by country, and launch outcomes), yearly since 1957, from
  [GCAT](https://planet4589.org/space/gcat) — Jonathan McDowell's General Catalog
  of Artificial Space Objects (CC-BY; cite as
  *data from GCAT (J. McDowell, planet4589.org/space/gcat)*).

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
| `BLS_API_KEY`      | `.env`    | _(none)_                | api     | BLS v2 key (optional)                     |
| `ADMIN_TOKEN`      | `.env`    | _(none)_                | api     | Enables `POST /admin/refresh` if set      |
| `HTTP_ADDR`        | `.config` | `:8080`                 | api     | API listen address                        |
| `DATA_DIR`         | `.config` | `./data`                | api     | Where to cache data                       |
| `REFRESH_INTERVAL` | `.config` | `360h`                  | api     | How often to re-pull series               |
| `START_YEAR`       | `.config` | `1948`                  | api     | First year of history (end = this year)   |
| `FRONTEND_ADDR`    | `.config` | `:3000`                 | web     | Web UI listen address                     |
| `BACKEND_URL`      | `.config` | `http://localhost:8080` | web     | API base URL the web UI fetches from      |

## Run

```sh
# Terminal 1 — API service (populates ./data on first run)
go run ./cmd/server

# Terminal 2 — web UI
go run ./cmd/web
# open http://localhost:3000 in your browser
```

## Extending

### For Now
- **More data** add more series and data sources that matter
- **Forecasting** add linear regression and time series forcasting models to predict trends

### For Later
- Native **iOS** client.
- Native **Android** client.
- Native **Windows** client.
- Native **Mac** client.
