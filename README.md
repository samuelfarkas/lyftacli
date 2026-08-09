# Lyfta CLI

[![CI](https://github.com/samuelfarkas/lyftacli/actions/workflows/ci.yml/badge.svg)](https://github.com/samuelfarkas/lyftacli/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/samuelfarkas/lyftacli)](https://github.com/samuelfarkas/lyftacli/releases/latest)
[![License: MIT](https://img.shields.io/github/license/samuelfarkas/lyftacli)](LICENSE)

`lyfta` is an unofficial, JSON-first CLI for the [Lyfta Developer and Coach APIs](https://my.lyfta.app/community/api#coach-api-beta). Use it to export workout history, check progress, or create programs from a shell script or AI agent.

- Successful commands print compact JSON to stdout. Errors go to stderr as JSON with a documented exit code.
- The CLI covers workouts, exercises, progress, coaching clients, collections, and workout templates.
- Write commands validate payloads locally and support `--dry-run`. They do not retry non-idempotent requests.
- `lyfta capabilities` returns the command contracts, exit codes, and JSON Schemas without asking for credentials.

## Quick start

```console
$ lyfta capabilities | jq -c '[.commands[].command]'
["capabilities","mappings","workouts list","workouts summary","exercises list","exercises library","exercises progress","clients list","collections create","templates create","request"]

$ export LYFTA_API_TOKEN='your-api-key'
$ lyfta workouts summary --all |
    jq '[.workouts[] | {date: .workout_perform_date, workout: .title, volume: .total_volume}]'
```

Generate an API key on the [Lyfta API Access page](https://my.lyfta.app/community/api#api-key). Generating a new key revokes the previous key.

## Install

On macOS or Linux, run the installer (no runtime or root access required). It verifies the downloaded archive against the release SHA-256 checksums.

```bash
curl -fsSL https://raw.githubusercontent.com/samuelfarkas/lyftacli/master/install.sh | sh
```

It installs the latest release to `~/.local/bin/lyfta`. Add `export PATH="$HOME/.local/bin:$PATH"` to your shell startup file if needed. To select another release or destination:

```bash
curl -fsSL https://raw.githubusercontent.com/samuelfarkas/lyftacli/master/install.sh \
  | LYFTA_VERSION=0.0.0 LYFTA_INSTALL_DIR="$HOME/bin" sh
```

### Manual install

Download the archive for your platform and `checksums.txt` from the [latest release](../../releases/latest):

| System | Archive |
| --- | --- |
| macOS Apple Silicon | `lyfta_<version>_darwin_arm64.tar.gz` |
| macOS Intel | `lyfta_<version>_darwin_amd64.tar.gz` |
| Linux ARM64 / x86-64 | `lyfta_<version>_linux_{arm64,amd64}.tar.gz` |
| Windows ARM64 / x86-64 | `lyfta_<version>_windows_{arm64,amd64}.zip` |

Verify the SHA-256 against `checksums.txt`, then (macOS/Linux):

```bash
archive=lyfta_0.1.0_darwin_arm64.tar.gz
tar -xzf "$archive"
install -d "$HOME/.local/bin"
install -m 0755 "${archive%.tar.gz}/lyfta" "$HOME/.local/bin/lyfta"
"$HOME/.local/bin/lyfta" --version
```

GitHub also signs build provenance for release archives. If you have the GitHub CLI, verify a downloaded archive with:

```bash
gh attestation verify "$archive" --repo samuelfarkas/lyftacli
```

- **macOS Gatekeeper**: binaries are not notarized. If Gatekeeper blocks a manually downloaded binary after checksum verification, run `xattr -d com.apple.quarantine "$HOME/.local/bin/lyfta"`.
- **Windows**: expand the ZIP and move `lyfta.exe` to a directory on `PATH`.

### Build from source

Requires Go 1.25+:

```bash
go install github.com/samuelfarkas/lyftacli/cmd/lyfta@latest
```

From a clone of the repository:

```bash
make install                          # installs to ~/.local/bin (override with BINDIR=...)
make uninstall                        # removes it
go build -trimpath -o bin/lyfta ./cmd/lyfta   # without Make
```

Source builds report version `dev`; tagged release builds inject the tag version. To inject a version locally:

```bash
go build -trimpath -ldflags '-X github.com/samuelfarkas/lyftacli/internal/version.Value=0.1.0' -o dist/lyfta ./cmd/lyfta
```

Cross-compile with the normal Go variables (`CGO_ENABLED=0 GOOS=... GOARCH=...`).

## Authentication

Prefer a mounted secret file so the token never appears in arguments or process listings:

```bash
lyfta --token-file /run/secrets/lyfta workouts summary
```

Or use an environment variable: `LYFTA_API_TOKEN` (or `LYFTA_API_KEY`). There is intentionally no `--token` flag.

Do not commit tokens, `.env` files, or output containing personal workout data. Generating a new key in Lyfta revokes the previous one.

## Usage

Compact JSON is the default; add `--pretty` for interactive reading. Global flags work before or after subcommands.

### Discovery (no credentials required)

```bash
lyfta capabilities   # command inventory, contracts, exit codes, JSON Schemas
lyfta mappings       # equipment, body-part, and muscle lookup tables
```

### Fetch data

```bash
lyfta workouts list --limit 20 --page 1
lyfta workouts summary --all --max-pages 20
lyfta exercises list --all
lyfta exercises library --search 'bench press' --limit 10
lyfta exercises progress --exercise-id 31 --duration 365
```

`--all` combines pages and reports `pages_fetched` and `truncated` (`true` means `--max-pages` stopped early). Page-based commands put these fields at the top level; `exercises library` puts them in `data.pagination`.

### Coach API

Requires a paid Coach or Scale plan:

```bash
lyfta clients list --active
lyfta workouts list --client-id dj152auh --all
lyfta exercises progress --client-id dj152auh --exercise-id 31 --duration 90
```

### Create a collection/program

```bash
lyfta collections create --title 'Strength block' --goal strength --dry-run --pretty
```

Remove `--dry-run` to create it. Creation is non-idempotent and never automatically retried. Options: `--client-id` for a coaching client, `--image-file` to base64-encode a cover image, `--payload -` to pipe the complete API payload via stdin.

### Create a workout template

Copy exercise metadata exactly from `lyfta exercises library`: `exercise_id`, the intentionally misspelled `excercise_name`, `exercise_type`, and `exercise_image` map to catalog `id`, `name`, `exercise_type`, and `image_name`.

```json
{
  "title": "Push day",
  "exercises": [
    {
      "exercise_id": 123,
      "excercise_name": "Bench Press",
      "exercise_type": "weight_reps",
      "exercise_image": "https://apilyfta.com/example.png",
      "exercise_rest_time": 90,
      "sets": [
        { "set_type_id": 0, "reps": "10", "weight": "60" },
        { "set_type_id": 0, "reps": "8", "weight": "65" }
      ]
    }
  ]
}
```

```bash
lyfta templates create --collection-id 42 --workout workout.json --dry-run
lyfta templates create --collection-id 42 --workout workout.json
```

For small payloads, pass fields directly with `--title` and `--exercise-json`. Options: `--client-id` for a coaching client, `--picture-file` to encode a local image as a data URI, `--payload` for the complete request.

Write commands validate payload structure locally, including during `--dry-run`, before reading credentials or contacting Lyfta.

### Forward-compatible requests

```bash
lyfta request GET /api/v1/example --query limit=10
lyfta request POST /api/v1/example --data request.json --dry-run
```

Absolute URLs must match the `--base-url` origin. Cross-origin redirects are refused. HTTPS is required except for loopback.

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success |
| `2` | Invalid arguments or input |
| `3` | Missing/invalid configuration or transport failure |
| `4` | Lyfta API error |
| `130` | Interrupted with Ctrl-C |

Example error:

```json
{"error":{"message":"Invalid API key","status_code":401,"type":"api_error"},"ok":false}
```

Reliability: per-attempt timeout, Ctrl-C cancellation, a 16 MiB response cap, and `Retry-After` support. The CLI retries 429 responses, transient 5xx responses, and connection failures only for idempotent methods. It never replays POST or PATCH requests.

## Development

See [CONTRIBUTING.md](CONTRIBUTING.md) for the contribution workflow and [SECURITY.md](SECURITY.md) for private vulnerability reports.

```bash
go test ./...
go test -race ./...
go vet ./...
govulncheck ./...
```

Tests use injected HTTP transports; no real token or network port required.

### Releasing

Push a semantic version tag to trigger the GoReleaser workflow:

```bash
git tag v0.1.0 && git push origin v0.1.0
```

It builds the six archives, generates `checksums.txt`, injects the version, and publishes a GitHub Release using the built-in `GITHUB_TOKEN`.

## License

[MIT](LICENSE). Lyfta does not maintain or endorse this independent project.
