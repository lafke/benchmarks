# Lafke benchmarks

Reproducible performance benchmarks for the [Lafke](https://github.com/lafke/lafke) CLI and
its analysis engine. Every measurement runs against public source code pinned to an exact
commit, so anyone can repeat it and compare the numbers.

This repository measures **speed and memory**. It does not measure detection accuracy, and a
fast run says nothing about how many vulnerabilities were found.

## Reproduce

You need Go (the version in [`go.mod`](go.mod)), `git`, and `lafke` with an engine installed
(`curl -fsSL https://lafke.sh/install | sh`). Run the commands from the repository root.

```sh
go run ./cmd/bench                          # 1 warm-up and 5 measured runs per corpus
go run ./cmd/bench -runs 10                 # more runs
go run ./cmd/bench -only owasp-benchmark-java
go run ./cmd/bench -lafke ./lafke           # a specific executable
```

The program clones each corpus into `.cache/` at the revision in [`corpora.json`](corpora.json),
fails if the checkout does not match it, and writes a JSON result to `results/`.

## Corpora

| Name | Source | Revision | Analyzed path |
| --- | --- | --- | --- |
| `owasp-benchmark-java` | [BenchmarkJava](https://github.com/OWASP-Benchmark/BenchmarkJava) | `acc50f20ec44` | `src/main` |
| `owasp-benchmark-python` | [BenchmarkPython](https://github.com/OWASP-Benchmark/BenchmarkPython) | `f1291485808b` | repository root |

The corpora are downloaded when you run the program. Their source is not copied here and keeps
its own license.

## Method

- Each measurement runs `lafke sast <path> --format json --fail-on critical` in a fresh
  process. Exit codes 0 and 1 (findings) count as success; any other code stops the run.
- One warm-up run is discarded, then the measured runs are recorded.
- **Wall time** is the elapsed time of the whole command, including starting the engine.
- **Peak memory** is the maximum resident set size of `lafke` and the processes it started.
- The result records min, median, mean and standard deviation, every raw sample, and the
  environment: OS, architecture, CPU count, Go, `lafke` and engine versions.
- Peak memory is not measured on Windows; it is reported as 0.

## Reading results

- Compare results only from the same machine, with the same `lafke` and engine versions, and
  with nothing else heavy running. Absolute numbers differ between machines.
- Report the median together with the number of runs, and share the JSON file.
- To compare with another tool, run it on the same revisions and the same path, on the same
  machine, and publish the command you used.

Results depend on the machine, so this repository does not commit any. Add yours to an issue
or a pull request as the JSON file the program wrote.

## Adding a corpus

Add an entry to [`corpora.json`](corpora.json) with the repository URL, the full 40-character
commit hash (never a branch or tag), the path to analyze, and the `lafke` command. Then run the
program once to check that it works.

## License

[Apache-2.0](LICENSE), for the code in this repository.
