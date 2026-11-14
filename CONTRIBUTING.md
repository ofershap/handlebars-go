# Contributing

Issues and pull requests welcome. Please:

- Add a golden fixture for behavior changes: extend `testdata/gen/corpus.json`, regenerate with `cd testdata/gen && npm ci && node gen.js > ../goldens.json`, and make sure `go test ./...` passes.
- If the two engines disagree on purpose, document the dialect difference in the README's "What it doesn't do" instead of forcing parity.
- Keep it stdlib-only.
- Run `go test ./...` and `go vet ./...` before pushing. Fuzz occasionally: `go test -fuzz=FuzzParse -fuzztime=60s`.
