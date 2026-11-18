# FlagForge

FlagForge is a feature flag service written in Go, with PostgreSQL and a Go SDK. I built it to cover what a flag service has to get right: turning a feature on for some users and not others, rolling it out to a percentage of users without anyone flipping back and forth, two admins editing the same flag at once, and getting a change to every running application within milliseconds, even when the service runs as several instances or goes down.

[![CI](https://github.com/hmzckd/flagforge/actions/workflows/ci.yml/badge.svg)](https://github.com/hmzckd/flagforge/actions/workflows/ci.yml)

## What it includes

- Boolean flags with a separate setting per environment (staging and production)
- Targeting rules on user attributes (`in`, `not_in`, `starts_with`, `ends_with`, `contains`), where the first matching rule wins
- Percentage rollouts with stable buckets, so raising the percentage never takes the feature away from a user who already had it
- One evaluation package shared by the server and the SDK, so both always give the same answer
- Safe edits with `ETag` and `If-Match` (412 when someone changed the setting first), and an audit log written in the same transaction as the change
- SDK keys per environment, stored as SHA-256 hashes, and a snapshot endpoint that answers 304 Not Modified
- Live updates over Server-Sent Events, with heartbeats, and slow clients dropped instead of holding up the others
- Several instances kept in step through PostgreSQL `LISTEN`/`NOTIFY`
- A Go SDK that answers from memory in about 30 ns, keeps the last known flags while the service is down, and reconnects with jittered backoff
- Tests against real PostgreSQL with Testcontainers, fuzz and property tests for the evaluation rules, and a goroutine leak check for the SDK

## How it fits together

```mermaid
flowchart LR
    admin([Admin])

    subgraph instances [FlagForge instances]
        a[Instance A]
        b[Instance B]
    end

    db[(PostgreSQL<br/>flags, settings, audit log, SDK keys)]

    subgraph app [Client application]
        sdk[Go SDK<br/>flags in memory]
    end

    admin -->|REST, admin key| a
    a -->|change + NOTIFY<br/>in one transaction| db
    db -->|NOTIFY flag_changes| a
    db -->|NOTIFY flag_changes| b
    b -->|snapshot over SSE| sdk
```

A change to a flag and its notification are committed in one transaction, so no instance hears about a change that was rolled back. Every instance listens on the `flag_changes` channel and sends the environment's full snapshot to the SDKs connected to it. The SDK evaluates flags locally: the network is only used to receive new snapshots, never to answer a question.

## Stack

| Area | Technology |
| --- | --- |
| Service | Go 1.27 standard library (`net/http`, `log/slog`) |
| Database | PostgreSQL 17, pgx 5, goose migrations (embedded, run at startup) |
| Live updates | Server-Sent Events, PostgreSQL `LISTEN`/`NOTIFY` |
| Tests | `go test`, Testcontainers for Go, goleak |
| Container | Multi-stage Dockerfile, distroless image (about 23 MB) |
| CI | GitHub Actions |

## Repository layout

```text
.
├── cmd/flagforge/        the service
├── cmd/ffrollout/        shows how many users a rollout gives true
├── cmd/ffbench/          measures how fast a change reaches an SDK on another instance
├── eval/                 rules, rollouts and evaluation, shared by the server and the SDK
├── sdk/                  Go SDK
├── examples/checkout/    small application that uses the SDK
├── internal/config/      configuration from environment variables
├── internal/httpapi/     HTTP handlers, admin and SDK keys, ETags, SSE
├── internal/store/       PostgreSQL access and migrations
├── internal/stream/      snapshot hub and the LISTEN/NOTIFY listener
├── internal/testdb/      PostgreSQL container for tests
├── docs/adr/             architecture decision records
├── GLOSSARY.md           domain terms
├── Dockerfile
├── compose.yaml          PostgreSQL and two FlagForge instances
└── .github/workflows/ci.yml
```

## Run locally

### Requirements

- Docker Desktop
- Go 1.27 or newer, for the demo application, the benchmark and the tests

### 1. Start PostgreSQL and two instances

```powershell
docker compose up -d --build
```

Instance A listens on `http://localhost:8081`, instance B on `http://localhost:8082` and PostgreSQL on `localhost:5436`. Both instances run the migrations at startup, and an advisory lock makes sure only one of them does it at a time. The compose file sets a development admin key; never use it outside your machine.

### 2. Create a flag and an SDK key on instance A

```powershell
$admin = @{ Authorization = "Bearer hamza-dev-key-0123456789abcdef0123" }
Invoke-RestMethod -Method Post -Uri http://localhost:8081/api/flags -Headers $admin -ContentType "application/json" -Body '{"key": "new-checkout", "name": "New checkout"}'
$sdkKey = (Invoke-RestMethod -Method Post -Uri http://localhost:8081/api/environments/production/sdk-keys -Headers $admin).key
```

The full SDK key is shown only in this response. FlagForge stores its hash.

### 3. Run the demo application against instance B

```powershell
$env:FLAGFORGE_URL = "http://localhost:8082"
$env:FLAGFORGE_SDK_KEY = $sdkKey
go run ./examples/checkout
```

It prints one line per second with the value of `new-checkout` for five users. The flag starts off, so every user gets `false`.

### 4. Turn the flag on for Turkey on instance A

In a second window:

```powershell
$admin = @{ Authorization = "Bearer hamza-dev-key-0123456789abcdef0123" }
$url = "http://localhost:8081/api/flags/new-checkout/environments/production"
$etag = "$((Invoke-WebRequest -UseBasicParsing -Uri $url -Headers $admin).Headers.ETag)"
Invoke-RestMethod -Method Put -Uri $url -Headers ($admin + @{ "If-Match" = $etag }) -ContentType "application/json" -Body '{"enabled": true, "rules": [{"clauses": [{"attribute": "country", "op": "in", "values": ["TR"]}], "serve": {"value": true}}], "fallthrough": {"value": false}}'
```

The demo's next line shows `u-1` and `u-2` (country `TR`) as `true`, without a restart. Stop instance B with `docker compose stop flagforge-b`: the demo keeps printing the same values and logs that it is using the last known flags. Start it again with `docker compose start flagforge-b` and the demo reconnects.

### Run the service with Go instead

```powershell
docker compose up -d postgres
$env:FLAGFORGE_DATABASE_URL = "postgres://flagforge:flagforge@localhost:5436/flagforge?sslmode=disable"
$env:FLAGFORGE_ADMIN_KEYS = "hamza=hamza-dev-key-0123456789abcdef0123"
go run ./cmd/flagforge
```

| Variable | Default | Meaning |
| --- | --- | --- |
| `FLAGFORGE_DATABASE_URL` | required | PostgreSQL connection string |
| `FLAGFORGE_ADMIN_KEYS` | required | Comma-separated `name=key` pairs, each key at least 32 characters; the name is written to the audit log |
| `FLAGFORGE_ADDR` | `:8080` | Listen address |
| `FLAGFORGE_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |

## Checks

Docker must be running, because the integration tests start their own PostgreSQL container:

```powershell
go vet ./...
go test ./...
```

CI also checks formatting and runs the tests with the race detector (`go test -race ./...`).

## Using the SDK

```go
client, err := sdk.New(ctx, sdk.Config{BaseURL: "http://localhost:8082", SDKKey: sdkKey})
if err != nil {
	logger.Warn("starting with default values", "error", err)
}
defer client.Close()

user := sdk.User{Key: "u-1", Attributes: map[string]string{"country": "TR"}}
if client.Bool("new-checkout", user, false) {
	// new checkout
}
```

- `New` downloads the environment's flags once, waiting at most 2 seconds. If FlagForge cannot be reached it still returns a working client, together with `sdk.ErrNotInitialized`, so the application can start.
- `Bool` never makes a network call. It returns the default value the caller passed in when the flag is unknown or no flags have arrived yet.
- A background stream replaces the flags when they change. If the stream breaks, the client keeps the last known flags and reconnects after a random delay between 0 and a ceiling that doubles from 1 to 30 seconds. A stream that sends nothing, not even a heartbeat, for 45 seconds is treated as dead. A rejected SDK key stops the stream instead of retrying forever.
- `Close` stops the stream and waits for its goroutine to finish.

## API overview

Admin routes need `Authorization: Bearer <admin key>`, SDK routes `Authorization: Bearer <SDK key>`.

| Method | Route | Purpose |
| --- | --- | --- |
| POST | `/api/flags` | Create a flag, off in every environment |
| GET | `/api/flags` | List flags with their on/off state per environment |
| GET | `/api/flags/{key}` | Get a flag |
| GET | `/api/flags/{key}/environments/{env}` | Get a flag's setting in one environment, with an `ETag` |
| PUT | `/api/flags/{key}/environments/{env}` | Replace that setting (on/off, rules, fallthrough), `If-Match` required |
| GET | `/api/flags/{key}/audit` | The flag's audit log, newest first |
| GET | `/api/environments` | List environments |
| POST | `/api/environments/{env}/evaluate` | Evaluate a flag for a user and explain the result |
| POST | `/api/environments/{env}/sdk-keys` | Create an SDK key, shown once |
| GET | `/api/environments/{env}/sdk-keys` | List SDK keys by prefix |
| DELETE | `/api/environments/{env}/sdk-keys/{id}` | Revoke an SDK key |
| GET | `/sdk/v1/flags` | All flags of the key's environment, `If-None-Match` answers 304 |
| GET | `/sdk/v1/stream` | The same snapshot over Server-Sent Events, sent again on every change |
| GET | `/health` | The process is running |
| GET | `/ready` | The database can be reached |

Errors are returned as `application/problem+json`: 400 for invalid input, 401 for a missing or wrong key, 404 for an unknown flag, environment or SDK key, 409 for a flag key that already exists, 412 when the setting changed after the `ETag` was read, and 428 when `If-Match` is missing.

## Design decisions

Each decision has a short record in [`docs/adr/`](docs/adr/) with its reasons and consequences. Terms such as rollout, bucket and fallthrough are defined in [`GLOSSARY.md`](GLOSSARY.md).

| ADR | Decision |
| --- | --- |
| [0001](docs/adr/0001-one-evaluation-package-for-server-and-sdk.md) | The server and the SDK run the same public `eval` package, so they cannot disagree |
| [0002](docs/adr/0002-store-targeting-rules-as-one-jsonb-document.md) | A setting's rules and fallthrough are stored as JSONB on the setting row and always replaced as a whole |
| [0003](docs/adr/0003-bucket-users-with-sha256-into-10000-buckets.md) | Rollouts put users into 10,000 buckets with SHA-256 of the flag and user key |
| [0004](docs/adr/0004-stream-full-snapshots-not-patches.md) | The stream sends the full snapshot on every change instead of patches |
| [0005](docs/adr/0005-postgres-listen-notify-between-instances.md) | Instances hear about changes through PostgreSQL `LISTEN`/`NOTIFY`, not Redis or Kafka |

## Measurements

Measured on one 4-CPU Linux machine running PostgreSQL, both instances (in Docker) and the measuring program together.

| What | Result |
| --- | --- |
| 100 changes made on instance A until an SDK connected to instance B returns the new value (`cmd/ffbench`, 4 runs) | p50 2.8 to 3.9 ms, p95 3.7 to 5.1 ms, slowest 4.1 to 6.1 ms |
| `client.Bool` answered from memory | about 31 ns, no memory allocated |
| The same question asked over HTTP to a server on the same machine | 77 to 97 µs, 98 allocations |
| Reading the current snapshot from 4 goroutines at once | 0.16 ns with `atomic.Pointer`, 78 ns with `sync.RWMutex` |

The propagation time starts when the `PUT` is sent, so it includes the request itself. To measure it yourself with the compose setup running:

```powershell
$env:FLAGFORGE_ADMIN_KEY = "hamza-dev-key-0123456789abcdef0123"
go run ./cmd/ffbench
go test -run='^$' -bench=. -benchmem ./sdk ./eval
```

## Roadmap

- String and number flag values, not only on/off
- Reusable user segments, such as "beta testers", shared by many flags
- A small web interface for admins
- Metrics for connected SDKs and propagation time
