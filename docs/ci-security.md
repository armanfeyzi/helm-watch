# CI security scans (report-only)

Helm Watch PR/push CI includes three **report-only** security jobs (Linear [ARM-5](https://linear.app/armanfeyzi/issue/ARM-5)). They run on every workflow but do **not** fail the build yet. Follow-up [ARM-6](https://linear.app/armanfeyzi/issue/ARM-6) turns on hard gates for secrets and critical image CVEs.

Workflow: [`.github/workflows/ci.yml`](../.github/workflows/ci.yml)

## Jobs overview

| Job | Tools | What it checks |
|-----|-------|----------------|
| `security-static` | gitleaks, Semgrep | Git secrets; Go/Kubernetes SAST rules |
| `container-security` | Trivy | CVEs and misconfig in the `helm-watch:ci` image |
| `test-and-build` | gosec (existing) | Go-specific high-confidence SAST |

`container-security` depends on `test-and-build` and reuses the saved Docker image tarball.

## gitleaks (secrets)

- **Why:** Catches API keys, tokens, and passwords committed to git — before they reach a public OSS repo or GHCR.
- **How:** Scans full git history (`fetch-depth: 0`). Uses the default gitleaks ruleset.
- **Report-only:** `continue-on-error: true` — findings appear in the job log; CI stays green.
- **Later (ARM-6):** Remove `continue-on-error` so any leak fails the workflow.

## Semgrep (SAST)

- **Why:** Broader static analysis than gosec alone (logic bugs, unsafe patterns, security anti-patterns in Go and YAML-adjacent rules).
- **How:** Rulesets `p/golang` and `p/security-audit` via `semgrep/semgrep-action`.
- **Report-only:** `continue-on-error: true`.
- **Later (ARM-10):** Enable Semgrep `--error` / fail on ERROR severity after tuning false positives.

## Trivy (container image)

- **Why:** The runtime image can include vulnerable OS packages or dependencies not visible in `go test`.
- **How:** After `docker build`, the image is saved as an artifact, loaded in `container-security`, and scanned with `aquasecurity/trivy-action`. Table output goes to the log; SARIF is uploaded as `trivy-image-sarif` for optional review in GitHub / external tools.
- **Report-only:** `exit-code: "0"` — never fails the job regardless of severity.
- **Later (ARM-6):** Fail on CRITICAL and HIGH; use `.trivyignore` only for documented exceptions.

## What stays unchanged

- **gosec** remains in `test-and-build` and already fails on high-severity Go issues.
- **golangci-lint**, tests, Helm lint, and image build behavior are unchanged.

## Reading results in GitHub Actions

1. Open the CI run for your PR or push.
2. Open **Security — secrets & SAST** or **Security — container image**.
3. Expand the failing-looking steps — with report-only mode the job is still green; read the log for `leak`, `finding`, or Trivy `TOTAL`.

## Rollout plan (from practice lab)

| Week | Change |
|------|--------|
| 1 (now) | Report-only: gitleaks, Semgrep, Trivy image |
| 2 | Fail on gitleaks + Trivy CRITICAL/HIGH; add govulncheck (ARM-7) |
| 3 | Semgrep error gate (ARM-10) |

Training notes: see Notion page under **Practice · Lab CI/CD (GitHub Actions)** (helm-watch Week 1 implementation).
