# Changelog

## 0.2.0 - 2026-11-24

- Built on trackified-go v0.2.1.
- `login` accepts real `live_...` / `test_...` keys (it wrongly required `tk_`); any non-empty key is
  checked against the API, with a warning for an unusual prefix.
- `webhooks test` prints the signed body exactly as received (`--json`: `body` is that string), so it
  can be fed to `verify-webhook`. Corrected the `--events` help: the default is `tracking.updated,tracking.delivered`.
- `resolve <name>` (T-5), `health` (T-4), `webhooks update <id> [--url] [--events] [--enable|--disable]` (T-11)
  and `webhooks rotate-secret <id> [--expires-in 24h]` (T-10).
- `verify-webhook` prints why a signature was rejected (T-9), also as `reason` with `--json`;
  headers with several `v1=` signatures verify if any matches.
- `TRACKIFIED_BASE_URL` may end in `/v1` (T-3).

## 0.1.0 - 2026-11-24

- First release of `trackified`: login, track, get, list, watch, retrack, stop, carriers, detect,
  webhooks (list/add/rm/test), usage, verify-webhook and version, with `--json` on every command.
  Built on trackified-go v0.1.0.
