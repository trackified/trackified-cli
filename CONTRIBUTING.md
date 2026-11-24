# Contributing

1. Fork and branch from `main`.
2. Run `gofmt -l .`, `go vet ./...`, `staticcheck ./...` and `go test -race ./...`; all must be clean.
3. Tests must not hit the network; use `httptest` and `TRACKIFIED_BASE_URL`.
4. Open a pull request with a short description and a CHANGELOG entry.
