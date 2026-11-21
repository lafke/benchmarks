# Lafke benchmarks

Reproducible performance benchmarks for the [Lafke](https://github.com/lafke/lafke) CLI and
its analysis engine. Every measurement runs against public source code pinned to an exact
commit, so anyone can repeat it and compare the numbers.

This repository measures **speed and memory**, and **detection accuracy** on suites whose
vulnerable and safe cases are labeled. A fast run says nothing about what was found, so read
the two together.

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

## Results in GitHub Actions

The [Benchmark workflow](.github/workflows/bench.yml) runs the same program on a GitHub-hosted
runner every Monday, on pushes that change it, and on demand (**Actions > Benchmark > Run
workflow**, where you can set the number of runs and the `lafke` version). Open a run to see:

- a table of median, minimum and standard deviation of the wall time, and the peak memory, in
  the **job summary**,
- a table of true positives, false positives, false negatives, true negatives, recall,
  precision and F1, with a breakdown by category, in the same summary,
- the JSON with every sample, as the `benchmark-results` **artifact**.

Shared runners are noisy, so compare runs of the same workflow with each other, not with your
own machine.

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
- Accuracy comes from the warm-up run's report, which is the same analysis as every measured
  run. See [Accuracy](#accuracy).
- The result records min, median, mean and standard deviation, every raw sample, and the
  environment: OS, architecture, CPU count, Go, `lafke` and engine versions.
- Peak memory is not measured on Windows; it is reported as 0.

## Accuracy

Each corpus has a labeled list of test cases (`expectedresults-*.csv` in the suite). A case is
vulnerable or safe, and belongs to a category. [`corpora.json`](corpora.json) maps every
category to the lafke rule that should report it.

| Term | Meaning |
| --- | --- |
| TP (true positive) | a vulnerable case where the expected rule reports a finding in the case's file |
| FP (false positive) | a safe case where the expected rule reports a finding |
| FN (false negative) | a vulnerable case with no finding of the expected rule |
| TN (true negative) | a safe case with no finding of the expected rule |
| Recall | TP / (TP + FN): the share of vulnerable cases found |
| Precision | TP / (TP + FP): the share of reported cases that are vulnerable |
| F1 | 2 x recall x precision / (recall + precision) |

Every label is in the denominator, including a case lafke could not analyze. Findings outside
the labeled files, and findings of other rules, are not part of the score. The JSON also holds
the number of `sast:analysis-incomplete` findings, which should be 0.

Read the scores with care:

- The suites are synthetic, and their ratio of vulnerable to safe cases differs from a real
  repository. The scores do not predict accuracy on an arbitrary application.
- Lafke's analysis is developed against suites like these, so this is not a held-out evaluation.
- A score only means something next to another tool that ran on the same revisions and the same
  cases, scored the same way.

## Reading results

- Compare results only from the same machine, with the same `lafke` and engine versions, and
  with nothing else heavy running. Absolute numbers differ between machines.
- Report the median together with the number of runs, and share the JSON file. Accuracy does
  not depend on the machine, only on the `lafke` and engine versions.
- To compare with another tool, run it on the same revisions and the same path, on the same
  machine, and publish the command you used.

Results depend on the machine, so this repository does not commit any. Add yours to an issue
or a pull request as the JSON file the program wrote.

## Adding a corpus

Add an entry to [`corpora.json`](corpora.json) with the repository URL, the full 40-character
commit hash (never a branch or tag), the path to analyze, and the `lafke` command. To score
accuracy, add `labels`: an OWASP Benchmark `expectedresults` file and the rule for each category. Then run the
program once to check that it works.

## License

[Apache-2.0](LICENSE), for the code in this repository.
