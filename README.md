# trackified

[![CI](https://github.com/trackified/trackified-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/trackified/trackified-cli/actions/workflows/ci.yml)

`trackified` is the command-line client for the [Trackified](https://trackified.net) parcel-tracking API,
built on the official Go SDK, [trackified-go](https://github.com/trackified/trackified-go).

API docs: https://forumulate.com/p/trackified/docs

## Install

```sh
# Homebrew (macOS, Linux)
brew install trackified/tap/trackified

# Go 1.26+
go install github.com/trackified/trackified-cli/cmd/trackified@latest
```

Prebuilt archives for macOS, Linux and
Windows (amd64 and arm64) are on the [releases page](https://github.com/trackified/trackified-cli/releases).
Each one has build provenance you can verify:

```sh
gh attestation verify trackified_0.2.0_linux_amd64.tar.gz --repo trackified/trackified-cli
```

## Quickstart

```sh
trackified login                                   # paste your live_... key; stored with 0600
trackified track 9434650110837017316201 --carrier usps
trackified list --status in_transit
trackified watch trk_123                           # prints new events until delivered
```

`TRACKIFIED_API_KEY` overrides the stored key. `TRACKIFIED_BASE_URL` points at another API host
(with or without a trailing `/v1`).

## Commands

| Command | What it does |
| --- | --- |
| `login` | Validate an API key and save it to the OS config dir (`trackified/config.json`, mode 0600) |
| `track <number> [--carrier c] [--custom-id id]` | Start tracking a parcel and print its status |
| `get <id>` | Show a tracking with its events |
| `list [--status s] [--carrier c] [--limit n] [--all]` | List trackings (`--all` fetches every page) |
| `watch <id> [--interval 60s]` | Poll and print new events until delivered |
| `retrack <id>` | Ask for the carrier to be checked again now |
| `stop <id>` | Stop tracking a parcel |
| `carriers [search]` | List supported carriers, or search the full catalog |
| `detect <number>` | Guess the carrier of a tracking number |
| `resolve <name>` | Map a carrier name (e.g. `USPS Priority Mail`, `dhl_express`) to a carrier id (exit 1 if none matches) |
| `webhooks list` / `add <url> [--events a,b]` / `rm <id>` / `test <id>` | Manage webhook endpoints (`add` defaults to `tracking.updated,tracking.delivered`; `test --json` gives `{signature, body}` with `body` as the exact signed string) |
| `webhooks update <id> [--url u] [--events a,b] [--enable\|--disable]` | Change a webhook; the secret is unchanged |
| `webhooks rotate-secret <id> [--expires-in 24h]` | Issue a new signing secret (printed once). The old one keeps verifying for `--expires-in` (0h-168h; 0h drops it now) |
| `usage` | This month's usage and limits |
| `health` | Check that the API is up (no API key needed) |
| `verify-webhook --secret s --header h --body-file f` | Check a webhook signature locally; prints the reason (`missing`, `malformed`, `timestamp`, `mismatch`) and exits 1 if invalid |
| `version` | Print the CLI and SDK versions |

Every command takes `--json` for machine-readable output.

## Releasing

Push a `v*` tag. GoReleaser builds the archives, checksums and SBOMs, creates the GitHub Release and
pushes the formula to [trackified/homebrew-tap](https://github.com/trackified/homebrew-tap) using the
`HOMEBREW_TAP_DEPLOY_KEY` secret (a write deploy key on the tap). Without that secret the tap push is
skipped. The archives and checksums get GitHub build-provenance attestations.

## License

MIT
