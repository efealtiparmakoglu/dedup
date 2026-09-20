# dedup

**A content-hash duplicate file finder in pure Go — fast xxh64 pre-pass, SHA-256 verified, zero dependencies.**

[![CI](https://github.com/efealtiparmakoglu/dedup/actions/workflows/ci.yml/badge.svg)](https://github.com/efealtiparmakoglu/dedup/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.22%2B-00ADD8.svg)](https://go.dev)

```
$ dedup ~/photos
photos/b/IMG_0042.jpg  [duplicate of photos/a/IMG_0042.jpg]
photos/c/copy.jpg      [duplicate of photos/a/IMG_0042.jpg]

1200 files scanned (4182737920 bytes); 3 duplicate groups, 5 duplicate files, 82134220 wasted bytes found
```

`dedup` finds identical files in three cheap-to-expensive passes: a size
prefilter, a hand-written **XXH64** hash of each file's first 4 KiB, and a
full **SHA-256** verification. Only identical SHA-256 digests are ever
reported as duplicates — the fast pass never decides alone.

## Why dedup

- **Never wrong.** Every reported duplicate is confirmed by a full-content
  SHA-256 digest; the xxh64 head pass only prunes obviously different files.
- **Delete is explicit and safe.** Nothing is touched without `-delete`,
  and even then exactly one file per group is kept.
- **Machine-readable.** `-json` emits a complete report (groups, hashes,
  sizes, wasted bytes) for scripts and dashboards.
- **Concurrent where it pays.** Hashing scales across workers; results stay
  deterministic and sorted.
- **Zero dependencies.** Go standard library plus the repo's own from-
  scratch XXH64 implementation, tested against the official vectors.

## Install

```sh
go install github.com/efealtiparmakoglu/dedup@latest
# or from a clone:
go build -o dedup .
```

Requires Go 1.22+. No other prerequisites.

## Usage

```sh
dedup ~/photos                    # report duplicates
dedup -json ~/photos > out.json   # JSON report
dedup -delete -min 1M ~/downloads # remove duplicates >= 1 MiB
dedup -workers 16 /data1 /data2   # several roots, more concurrency
```

| Option | Meaning |
|---|---|
| `-delete` | remove duplicates, keeping the first file of each group |
| `-json` | JSON report instead of the default listing |
| `-min BYTES` | skip files smaller than BYTES (default 1) |
| `-workers N` | hashing concurrency (default 8) |
| `-q` | suppress the group listing |

The first file of every group (sorted by path) is the suggested keeper.
With `-delete`, exactly that file survives; everything else is removed.
Unreadable files are reported on stderr and never stop the run.

## The three-pass pipeline

1. **Size prefilter** — files can only be duplicates of the same size;
   everything else is grouped away before a single byte is read.
2. **xxh64 head pass** — the first 4 KiB of each same-size file is hashed
   with a from-scratch XXH64 (spec-conformant, tested against the official
   vectors); different head hashes prune the group.
3. **SHA-256 verification** — surviving candidates are hashed fully;
   identical digests form a duplicate group.

This ordering means the expensive pass touches a tiny fraction of the data
on typical trees, while correctness never depends on the fast hash.

## Development

```sh
go test ./...          # xxh64 vectors + engine semantics
go test -race ./...
go build -o dedup .
```

Tests cover the XXH64 known-answer vectors, stripe-boundary inputs, the
three-pass pipeline, min-size filtering, deterministic group ordering, and
the delete flow (keepers survive, duplicates go).

## License

MIT — see [LICENSE](LICENSE).
