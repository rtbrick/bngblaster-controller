# Contributing to the BNG Blaster Controller

Thanks for taking the time to contribute!

The BNG Blaster controller follows the general guidelines of the
[BNG Blaster project](https://github.com/rtbrick/bngblaster/blob/main/CONTRIBUTING.md),
in particular its work flow: pull requests go to the development branch
`dev`, and changes are merged from there to `main` and released. You can
open a draft pull request for work in progress or to ask for early review.

This document covers what is specific to this repository, a Go daemon with
an embedded web UI.

## Development

You need Go (see `go.mod` for the version) and, for running instances, the
[BNG Blaster](https://github.com/rtbrick/bngblaster) itself.

```sh
make build            # -> bin/<os>_<arch>/bngblasterctrl
make test             # go test -v -cover ./...
make lint             # golangci-lint v2 (.golangci.yml)
make fumpt            # gofumpt formatting
make gci              # import order: standard, default, github.com/rtbrick
```

To try the controller without root, point it at a writable folder:

```sh
./bin/linux_amd64/bngblasterctrl -d /tmp/bngblaster -debug
```

and open `http://localhost:8001/` for the web UI, `/docs/` for the API
documentation.

Before you submit, make sure that `make build` and `make test` pass (CI
runs both on every push and pull request) and that `make lint` reports no
new issues (CI fails on new issues only).

## Coding Conventions

- Every Go file starts with the license header:

  ```go
  // SPDX-License-Identifier: BSD-3-Clause
  // Copyright (C) 2020-2026, RtBrick, Inc.
  ```

- Format with `make fumpt` and `make gci`.
- Comments explain *why*, not *what*. Exported identifiers have doc
  comments; match the density of the surrounding code.
- Constructors use functional options (`NewServer(repo, WithUI(...))`).
  Optional features sit behind a command line flag and an option.
- Log with `github.com/rs/zerolog/log` and structured fields. HTTP
  handlers do not log themselves; the access log writes one line per
  request.
- The web UI (`pkg/server/webui/`) is vanilla HTML, CSS and JavaScript with
  no build step, framework or npm dependency. The files are embedded into
  the binary as they are.

### Security

The controller usually runs as root and its API has no authentication yet,
so every request must be treated as untrusted input:

- Sanitize the instance name with `cleanPathVariable` and file names with
  `filepath.Base` plus `isUnsafeFileName`; never let a request choose a path
  outside the instance folder.
- Bound every request body with `http.MaxBytesReader`.
- Never pass request data to a shell or as an unchecked command line
  argument.
- Render data in the web UI as text (`textContent`, the `el()` helper),
  never as HTML.

## Testing

- Server tests use the `controller.RepositoryMock` (generated with
  [moq](https://github.com/matryer/moq)) together with `httptest` and
  `testify/require`, mostly table-driven.
- Process tests fake `bngblaster` through `controller.ExecCommand` and the
  `TestHelperProcess` pattern (`pkg/controller/process_test.go`).
- Test fixtures live in `pkg/controller/td/`.
- After changing the `Repository` interface, regenerate the mock with
  `go generate ./pkg/controller`. If moq does not work with your Go
  version, extend `repositorymock.go` by hand in the same style.

## Changing the API

Update in the same change:

- the route in `pkg/server/server.go`,
- the API definition `docs/swagger.yaml` (see [Swagger](#swagger)),
- the web UI (`pkg/server/webui/static/js/app.js`) if it uses the endpoint,
- the README if flags or defaults change.

### Swagger

`docs/swagger.yaml` is the OpenAPI definition of the REST API. It is
embedded into the binary and served on `/docs/`, and GitHub Pages
publishes the `docs/` folder of `main` as the
[API documentation](https://rtbrick.github.io/bngblaster-controller).

For every API change, document the new or changed path, its parameters,
request body, every response status it can return, and an example. Then
check the result:

```sh
go test ./pkg/server -run TestServer_apiDocs   # the definition is embedded and served
make build && ./bin/linux_amd64/bngblasterctrl -d /tmp/bngblaster
```

and open `http://localhost:8001/docs/`: Swagger UI shows parse errors at
the top of the page, and every operation can be tried out against the
running controller.

The Swagger UI itself is vendored in `docs/swagger-ui/`, so the page loads
no third-party script; it is updated before every release (see
[Release](#release), step 3).

## Commit Messages

The release changelog is generated from the commit subjects, so write
them for the release notes: `<area>: <what changed>`, e.g.
`webui: add filter to command builder` or `server: reject empty instance
names`. Subjects starting with `docs:` or `test:` are left out of the
changelog.

## Release

Releases are built by [GoReleaser](https://goreleaser.com) from a version
tag: pushing a tag `X.Y.Z` (no `v` prefix) starts the
[release workflow](.github/workflows/release.yml), which builds the static
linux/amd64 binary and the Debian package and publishes them with the
changelog as a GitHub release. The version is injected into the binary at
build time (`main.Version`), so nothing in the code needs to change.

Before tagging a release:

1. **Merge `dev` into `main`** once it has passed testing, and make sure
   the build of `main` is green.

2. **Update the API version** in `docs/swagger.yaml` to the release
   version:

   ```yaml
   info:
     version: X.Y.Z
   ```

   Check that every API change since the last release is documented
   there (`git diff <last tag> -- pkg/server/server.go docs/swagger.yaml`)
   and that the page renders without errors on `/docs/` (see
   [Swagger](#swagger)). The API documentation on GitHub Pages is
   updated from `main` automatically.

3. **Update the vendored Swagger UI** in `docs/swagger-ui/` to the latest
   [swagger-ui-dist](https://www.npmjs.com/package/swagger-ui-dist)
   release, so the API documentation page does not ship known
   vulnerabilities. The files are copied unmodified from the npm package,
   after checking the package against the checksum the npm registry
   publishes:

   ```sh
   # latest version, its tarball and its published SHA-512
   VERSION=$(npm view swagger-ui-dist version)
   TARBALL=$(npm view swagger-ui-dist@$VERSION dist.tarball)
   INTEGRITY=$(npm view swagger-ui-dist@$VERSION dist.integrity)

   # download into an empty folder outside the repository and verify it
   TMP=$(mktemp -d)
   curl -sSfL -o $TMP/swagger-ui-dist.tgz $TARBALL
   test "sha512-$(openssl dgst -sha512 -binary $TMP/swagger-ui-dist.tgz | base64 -w0)" = "$INTEGRITY" \
     && echo "checksum ok" || echo "CHECKSUM MISMATCH - do not use"

   # copy only the files the page needs, plus the license files
   tar -xzf $TMP/swagger-ui-dist.tgz -C $TMP
   for f in swagger-ui-bundle.js swagger-ui-bundle.js.LICENSE.txt swagger-ui.css LICENSE NOTICE; do
     cp $TMP/package/$f docs/swagger-ui/
   done
   rm -rf $TMP
   ```

   Only continue if the script printed `checksum ok`. Then:
   - set the new version in `docs/swagger-ui/README.md`;
   - run `go test ./pkg/server -run TestServer_apiDocs`, which checks
     that the page still loads only the embedded files;
   - build and start the controller, open `http://localhost:8001/docs/`
     and check that all operations are listed and that the browser
     console shows no errors, in particular no Content-Security-Policy
     violations. The page allows no inline or third-party script
     (`apiDocsCSP` in `pkg/server/hardening.go`); a new major version
     that needs more must be reviewed before relaxing that policy.
   - commit the update separately, e.g. `docs: update Swagger UI to
     <version>`.

   If the version did not change, there is nothing to do.

4. **Update the README:**
   - the usage output, if flags or their defaults changed: replace the
     `bngblasterctrl -h` block with the output of
     `./bin/linux_amd64/bngblasterctrl -h`;
   - the environment variable table, if flags were added or removed;
   - the minimum BNG Blaster version in [Installation](README.md#installation),
     if the controller now relies on a newer BNG Blaster;
   - the screenshot `docs/controller.png`, if the web UI changed visibly.

5. **Build and test the release locally:**

   ```sh
   make test
   goreleaser release --snapshot --clean
   sudo dpkg -i dist/bngblaster-controller_*_amd64.deb
   ```

   Then check the installed service (`systemctl status
   rtbrick-bngblasterctrl`), the web UI and `/docs/`, and start and stop
   at least one instance.

6. **Tag and push** the release on `main`:

   ```sh
   git checkout main && git pull
   git tag X.Y.Z
   git push origin X.Y.Z
   ```

7. **Check the GitHub release:** the workflow attaches the `.deb`, the
   `tar.gz` archive and `checksums.txt`; review the generated changelog and
   edit the release notes if needed (for example to point out changed
   defaults).

8. **Merge `main` back into `dev`**, so the next development builds are
   versioned after the new tag.

Development builds from CI and `goreleaser release --snapshot` are
versioned `<next patch>~dev.<timestamp>.<commit>`, which Debian sorts below
the upcoming release, so installing the release upgrades over a dev build.
