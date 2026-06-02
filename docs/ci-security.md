# CI security scans (report-only)

Pull request and push workflows include optional security jobs that **report findings without failing CI**. This lets maintainers review noise levels before enabling enforcement gates.

Workflow: [`.github/workflows/ci.yml`](../.github/workflows/ci.yml)

## Jobs overview

| Job | Tools | What it checks |
|-----|-------|----------------|
| `security-static` | gitleaks | Secrets in git history |
| `semgrep` | Semgrep (`semgrep/semgrep` image) | Go and security-audit SAST rules |
| `container-security` | Trivy | CVEs in the `helm-watch:ci` image |
| `test-and-build` | gosec (existing) | Go high-confidence SAST (enforced) |

`container-security` depends on `test-and-build` and scans the Docker image artifact produced in that job.

## gitleaks (secrets)

- **Purpose:** Detect API keys, tokens, and passwords in commits before they are published.
- **Configuration:** Full git history (`fetch-depth: 0`), default gitleaks ruleset.
- **Current behavior:** `continue-on-error: true` — findings are logged; the job does not fail CI.
- **Planned enforcement:** Fail the workflow when any secret is detected.

## Semgrep (SAST)

- **Purpose:** Complement gosec with broader static rules for Go and common security anti-patterns.
- **Configuration:** Job runs in the [`semgrep/semgrep`](https://hub.docker.com/r/semgrep/semgrep) container with `semgrep scan --config p/golang --config p/security-audit`.
- **Current behavior:** `continue-on-error: true`.
- **Planned enforcement:** Fail on ERROR-severity findings after false positives are triaged.

## Trivy (container image)

- **Purpose:** Scan the runtime image for OS and dependency CVEs not covered by unit tests.
- **Configuration:** [`aquasecurity/trivy-action`](https://github.com/aquasecurity/trivy-action) (pinned to a release SHA). Table output in logs; SARIF uploaded as `trivy-image-sarif`.
- **Current behavior:** `exit-code: "0"` — results are informational only.
- **Planned enforcement:** Fail on CRITICAL and HIGH severities, with `.trivyignore` for documented exceptions only.

## Unchanged checks

- **gosec** in `test-and-build` continues to fail on high-severity Go issues.
- Tests, golangci-lint, Helm lint, and image build steps are unchanged.

## Viewing results in GitHub Actions

1. Open the CI run for the branch or pull request.
2. Inspect **Security — secrets**, **Security — SAST**, or **Security — container image**.
3. Review step logs for gitleaks leaks, Semgrep rule IDs, or Trivy severity counts (jobs may show warnings while report-only mode is enabled).

## Enforcement roadmap

| Phase | Change |
|-------|--------|
| Current | Report-only gitleaks, Semgrep, and Trivy image scans |
| Next | Fail CI on secret leaks and Trivy CRITICAL/HIGH |
| Later | Dependency scanning (govulncheck); Semgrep ERROR gate |
