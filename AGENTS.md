# AGENTS.md

Instructions for AI coding agents working in this repository. If a user's request
disagrees with this file, the user wins.

## Project

Reproducible benchmarks for the `lafke` CLI and its analysis engine: wall time, peak memory and
detection accuracy (TP, FP, FN, TN, recall, precision, F1) on public source code pinned to an exact
commit. This repository is public.

## Layout

- `corpora.json`: the corpora, each with a repository URL, a full commit hash, a path, a `lafke`
  command and, for accuracy, the labels file and the rule of each category.
- `cmd/bench`: clones the corpora, runs `lafke`, scores the findings against the labels, writes
  JSON to `results/` and a table to the Actions job summary.
- `README.md`: how to reproduce, the method, how to read results.

## Rules

- Everything here is public. Never mention private repositories, internal documents or
  unreleased results, and never commit a secret.
- A corpus is pinned to a full 40-character commit hash. Never use a branch or a tag.
- Do not copy corpus source into this repository; it is cloned at run time.
- Do not commit measured numbers from one machine as if they were a reference. State the
  machine, the `lafke` and engine versions, and the number of runs next to any figure.
- Present accuracy only with its caveats: the suites are synthetic and Lafke is developed against
  them, so the scores are not a held-out evaluation. Never describe them as parity with, or a
  ranking against, another tool.
- Do not claim another tool is slower or faster without the command, the version and the
  machine used for both.
- Keep the method in `README.md` in sync with `cmd/bench`.
- Use only the Go standard library, with no CGO.
- Before committing a change to `cmd/bench`, run `gofmt -l .` (must print nothing),
  `go vet ./...`, `GOOS=windows go vet ./...` and `go run ./cmd/bench -runs 1`. Say so if one
  cannot run.
- Write the name `Lafke` with a capital L in prose, and lowercase `lafke` only for code,
  commands, package names and URLs.
- Code, comments, commit messages, and everything published to GitHub are in English.
- Every commit message follows Conventional Commits, is all lowercase, and is 35 to 55
  characters long, counting the whole subject line (e.g. `docs: explain how to compare against another tool`).
  Check the length before committing.
