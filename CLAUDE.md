# CLAUDE.md

REST controller daemon (`bngblasterctrl`) for the [BNG Blaster](https://github.com/rtbrick/bngblaster).
It creates, starts, stops and inspects multiple `bngblaster` test instances on one host and
wraps each instance's JSON-RPC control socket as a REST API.
It also ships an embedded web UI (served on `/`, on by default, disable with `-ui=false`) that runs
on top of that REST API to manage instances from a browser.

## Commands

```sh
make build            # -> bin/<os>_<arch>/bngblasterctrl (version from latest git tag)
make test             # go test -v -cover ./...  (what CI runs, with make build)
go test ./pkg/server -run TestServer_create   # single test
make lint             # golangci-lint v2 (default: all, see .golangci.yml); CI only fails on new issues
make fumpt            # gofumpt formatting
make gci              # import order: standard, default, github.com/rtbrick
go generate ./pkg/controller   # regenerate repositorymock.go (needs matryer/moq)
```

Run locally without root by pointing at a writable folder:

```sh
./bin/linux_amd64/bngblasterctrl -d /tmp/bngblaster -debug
```

## Layout

- `cmd/bngblasterctrl/` – flag parsing (every flag falls back to `BNGBLASTERCTRL_<FLAG>`, `env.go`),
  zerolog setup (warn+ goes to stderr, rest to stdout), HTTP server, `-health` check (`health.go`).
  `-autostart` (`controller.Autostart`) starts one instance at startup for containers; its state is
  in memory only and reported in the instance status.
- `pkg/controller/` – instance lifecycle on disk and process management.
  - `Repository` interface (`model.go`) is the seam between HTTP and the file system/processes;
    `DefaultRepository` (`repository.go`) is the real implementation.
  - Each instance is a folder `<config-dir>/<name>/` holding `config.json`, `run.json`, `run.pid`,
    `run.sock`, `run.log`, `run_report.json`, `run.pcap`, `run.stdout`, `run.stderr`
    (filename constants in `repository.go`). "Running" is derived from these files, not from in-memory state.
  - `prom.go` – Prometheus metrics collected from running instances via the control socket.
- `pkg/server/` – gorilla/mux router (`server.go` `routes()`), one file per feature
  (streams, sessions, overview, logs, files, ui, apidocs). `cache.go` is a short-TTL,
  per-instance summary cache with in-flight dedup; invalidate it on any lifecycle change.
  `hardening.go` holds the auth-independent protections (cross-origin check for
  state-changing requests, `-allowed-hosts`, security headers/CSP, body size limits); bound every
  new request body with `http.MaxBytesReader`.
  `accesslog.go` writes one log line per request after the handler (status, duration, error message
  taken from the error response). Handlers don't log themselves: record lifecycle changes with
  `audit(r, instance, action)` and extra fields with `logFields(r)`. New read-only bngblaster
  commands belong in `readOnlyCommands`.
- `pkg/generator/` – runs `lspgen`/`bgpupdate`/`ldpupdate` in an instance folder (`_generate`). Requests are
  validated against per-tool option whitelists (`tools.go`, also served to the UI), never passed as argv;
  outputs are written under `.gen-*` temp names and moved into place only on success (no replace while
  running). lspgen socket mode is deliberately out of scope.
- `pkg/server/webui/` – experimental embedded SPA. Vanilla HTML/CSS/JS, **no build step,
  no framework, no npm** – files are `go:embed`ed and served as-is. `index.html` is a Go
  template (`{{.AssetVersion}}` cache-busting).
- `docs/` – `swagger.yaml` + Swagger UI, embedded into the binary and also published via GitHub Pages.
- `debian/` – systemd unit, `/etc/default` env file, logrotate, install scripts (packaged by goreleaser).

## Conventions

- Every Go file starts with:
  ```go
  // SPDX-License-Identifier: BSD-3-Clause
  // Copyright (C) 2020-2026, RtBrick, Inc.
  ```
- Constructors use functional options (`NewServer(repo, WithUI(...))`, `NewDefaultRepository(WithConfigFolder(...))`).
  Optional surface sits behind a flag + option. `-ui`, `-upload`, `-interfaces-api`, `-host-api` and
  `-generators` default to on in the binary (disable with `-flag=false`); the server/repository options
  themselves still default to off.
- Handlers are methods returning `http.HandlerFunc`. Always sanitize the instance path variable with
  `cleanPathVariable`, and file names with `filepath.Base` + `isUnsafeFileName`. There is no auth yet
  (`authMiddleware` is a no-op hook), so path-traversal safety matters.
- Use `JSONError` / `JSONNotFound` for error responses; map `controller.ErrBlaster*` errors to HTTP status
  (running → 412, not exists → 404).
- Logging via `github.com/rs/zerolog/log` with structured fields.
- Comments explain *why*; the codebase uses fairly thorough doc comments – match that density.

## Testing

- Server tests use `controller.RepositoryMock` (moq) plus `httpexpect`/`httptest`; table-driven with `testify/require`.
- Process tests fake `bngblaster` via `controller.ExecCommand` and the `TestHelperProcess` /
  `GO_WANT_HELPER_PROCESS` pattern (`process_test.go`).
- Fixtures live in `pkg/controller/td/`.
- After changing the `Repository` interface, regenerate the mock or the build breaks.

## When changing the API

Update in the same change: the route in `server.go`, `docs/swagger.yaml`, the web UI (`app.js`) if it
consumes the endpoint, and the README if flags or defaults change.

## Release

Tag-driven via goreleaser (`.goreleaser.yaml`, `.github/workflows/release.yml`): linux/amd64 static
binary (`CGO_ENABLED=0`) + `.deb`. `main.Version` is injected through ldflags.
