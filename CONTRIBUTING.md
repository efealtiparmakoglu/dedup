# Contributing to dedup

Thanks for your interest!

## Ground rules

- **Correctness over speed.** The xxh64 pre-pass may prune, but only full
  SHA-256 digests may declare a duplicate. Any change that lets the fast
  hash decide will be declined.
- **Delete is opt-in and minimal.** `-delete` keeps exactly one file per
  group; destructive behavior must never grow silent or broad.
- **Zero dependencies.** Go standard library plus the repo's own `xxh64`
  package. No third-party imports.
- **Deterministic output.** Groups and duplicate lists are sorted; run twice,
  get the same bytes.

## Workflow

```sh
gofmt -w . && go vet ./...
go test ./...
go test -race ./...
```

## Pull requests

1. One feature or fix per PR.
2. Performance changes need before/after numbers on a real tree.
3. New flags need usage text, README row, and tests.

## Reporting bugs

Open an issue with the command line and, if possible, two of the files
involved (they are duplicates, after all) and the exact output.
