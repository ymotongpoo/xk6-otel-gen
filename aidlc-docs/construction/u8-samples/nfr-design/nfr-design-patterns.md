# U8 Samples & Distribution — NFR Design Patterns

本書は U8 の **「どう実装するか」** のパターン群を確定する。FD + NFR-R を受けて、cmd 実装 / examples authoring / k8s manifests / CI config / AI skill の各カテゴリで具体パターンを決める。

参照:
- FD: `aidlc-docs/construction/u8-samples/functional-design/`
- NFR-R: `aidlc-docs/construction/u8-samples/nfr-requirements/`
- Plan + Answers: `aidlc-docs/construction/plans/u8-samples-nfr-d-plan.md`

---

## 1. cmd/xk6-otel-gen-schema/ 実装パターン

### 1.1 main / run の分離 (Q1=A)

```go
// SPDX-License-Identifier: Apache-2.0

// Command xk6-otel-gen-schema exports the topology JSON Schema for
// editor integration (yaml-language-server etc.) or CI lint.
package main

import (
    "flag"
    "fmt"
    "io"
    "os"

    "github.com/ymotongpoo/xk6-otel-gen/topology"
)

func main() {
    os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run executes the CLI given args (excluding program name) and IO writers.
// Returns process exit code (0 success, 1 runtime error, 2 flag parse error).
func run(args []string, stdout, stderr io.Writer) int {
    fs := flag.NewFlagSet("xk6-otel-gen-schema", flag.ContinueOnError)
    fs.SetOutput(stderr)
    output := fs.String("output", "", "output file path (default stdout)")
    if err := fs.Parse(args); err != nil {
        return 2  // flag parse error
    }

    schema, err := topology.ExportJSONSchema()
    if err != nil {
        fmt.Fprintf(stderr, "xk6-otel-gen-schema: %v\n", err)
        return 1
    }

    var w io.Writer = stdout
    if *output != "" {
        f, err := os.Create(*output)
        if err != nil {
            fmt.Fprintf(stderr, "xk6-otel-gen-schema: create %q: %v\n", *output, err)
            return 1
        }
        defer f.Close()
        w = f
    }

    if _, err := w.Write(schema); err != nil {
        fmt.Fprintf(stderr, "xk6-otel-gen-schema: write: %v\n", err)
        return 1
    }
    return 0
}
```

利点:
- `run()` は test 可能 (args + writers を inject)
- `main()` は 1 line thin wrapper、test 不要
- exit code semantics 明確 (0/1/2)

### 1.2 main_test.go テストパターン

```go
package main

import (
    "bytes"
    "strings"
    "testing"
)

func TestRun_StdoutDefault(t *testing.T) {
    var stdout, stderr bytes.Buffer
    code := run([]string{}, &stdout, &stderr)
    if code != 0 { t.Fatalf("exit=%d stderr=%s", code, stderr.String()) }
    if !strings.Contains(stdout.String(), "$schema") {
        t.Errorf("stdout missing $schema marker: %s", stdout.String()[:200])
    }
}

func TestRun_OutputToFile(t *testing.T) {
    tmpFile := filepath.Join(t.TempDir(), "schema.json")
    var stdout, stderr bytes.Buffer
    code := run([]string{"-output", tmpFile}, &stdout, &stderr)
    if code != 0 { t.Fatalf("exit=%d", code) }
    data, err := os.ReadFile(tmpFile)
    require.NoError(t, err)
    require.Contains(t, string(data), "$schema")
}

func TestRun_FlagParseError(t *testing.T) {
    var stdout, stderr bytes.Buffer
    code := run([]string{"-unknown"}, &stdout, &stderr)
    if code != 2 { t.Errorf("expected exit 2 (flag parse error), got %d", code) }
}

func TestRun_FileCreateFailure(t *testing.T) {
    var stdout, stderr bytes.Buffer
    // path with /nonexistent/dir/ should fail
    code := run([]string{"-output", "/dev/null/nonexistent/schema.json"}, &stdout, &stderr)
    if code != 1 { t.Errorf("expected exit 1, got %d", code) }
}
```

→ Coverage target NFR-U8-12 (70%) を達成する程度の test surface。

---

## 2. Examples Validation (Q2=A)

### 2.1 `test/examples/examples_test.go`

```go
// SPDX-License-Identifier: Apache-2.0

package examples_test

import (
    "io/fs"
    "os"
    "path/filepath"
    "testing"

    "github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestExamples_TopologyValidates(t *testing.T) {
    t.Parallel()
    examplesRoot := filepath.Join("..", "..", "examples")
    entries, err := os.ReadDir(examplesRoot)
    if err != nil { t.Fatal(err) }

    for _, entry := range entries {
        if !entry.IsDir() { continue }
        name := entry.Name()
        t.Run(name, func(t *testing.T) {
            t.Parallel()
            yamlPath := filepath.Join(examplesRoot, name, "topology.yaml")
            yaml, err := os.ReadFile(yamlPath)
            if err != nil { t.Fatalf("read %s: %v", yamlPath, err) }
            schema, err := topology.Parse(yaml)
            if err != nil { t.Fatalf("parse %s: %v", yamlPath, err) }
            if err := schema.Validate(); err != nil {
                t.Fatalf("validate %s: %v", yamlPath, err)
            }
        })
    }
}
```

→ `examples/<example>/topology.yaml` を loop で読んで `topology.Parse + Validate` に通す。1 example = 1 sub-test。

### 2.2 k8s manifest dry-run (CI workflow)

CI workflow YAML 内で:
```bash
for example_dir in examples/*/; do
  if [ -d "$example_dir/k8s" ]; then
    kubectl apply --dry-run=server -k "$example_dir/k8s/" || exit 1
  fi
done
```

→ CI cluster (kind) を起動した job で実行。

---

## 3. K8s Manifest 構造 (Q3=A)

### 3.1 kustomize-friendly 単一 manifest per service

```text
examples/<example>/k8s/
├── kustomization.yaml   ← entry point
├── namespace.yaml
├── collector.yaml       ← Collector Deployment + Service (ConfigMap は Kustomize generated)
├── tempo.yaml           ← Tempo Deployment + Service + inline ConfigMap
├── loki.yaml            ← Loki Deployment + Service + inline ConfigMap
├── prometheus.yaml      ← Prometheus Deployment + Service + ConfigMap
├── grafana.yaml         ← Grafana Deployment + Service (ConfigMaps は Kustomize generated)
├── datasources.yaml     ← Grafana datasource provisioning (Kustomize configMapGenerator source)
└── dashboard-overview.json  ← Grafana dashboard JSON (Kustomize configMapGenerator source)
```

### 3.2 kustomization.yaml

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: xk6-otel-gen-demo

resources:
  - namespace.yaml
  - collector.yaml
  - tempo.yaml
  - loki.yaml
  - prometheus.yaml
  - grafana.yaml

configMapGenerator:
  - name: otel-collector-config
    files:
      - config.yaml=../otel-collector-config.yaml
  - name: grafana-datasources
    files:
      - datasources.yaml
  - name: grafana-dashboards
    files:
      - dashboard-overview.json
```

### 3.3 単一 manifest 内 multi-resource

例 (`tempo.yaml`):

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: tempo-config
data:
  tempo.yaml: |
    server:
      http_listen_port: 3200
    distributor:
      receivers:
        otlp:
          protocols:
            grpc: { endpoint: 0.0.0.0:4317 }
    storage:
      trace:
        backend: local
        local:
          path: /var/tempo
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: tempo
spec:
  replicas: 1
  selector: { matchLabels: { app: tempo } }
  template:
    metadata: { labels: { app: tempo } }
    spec:
      containers:
        - name: tempo
          image: grafana/tempo:2.6.0
          args: ["-config.file=/etc/tempo/tempo.yaml"]
          ports:
            - { containerPort: 3200 }
            - { containerPort: 4317 }
          volumeMounts:
            - { name: config, mountPath: /etc/tempo }
          resources:
            requests: { memory: 512Mi, cpu: 100m }
      volumes:
        - name: config
          configMap: { name: tempo-config }
---
apiVersion: v1
kind: Service
metadata:
  name: tempo
spec:
  selector: { app: tempo }
  ports:
    - { name: http, port: 3200, targetPort: 3200 }
    - { name: otlp-grpc, port: 4317, targetPort: 4317 }
```

→ `---` で Kubernetes resource を 1 file 内に列挙。

### 3.4 base + overlays は採用しない

`minimal` と `astroshop` の k8s manifests は **独立した複製** とする (B 案不採用)。理由:
- user 学習 barrier 低 (kustomize の overlay 機構を学ばなくて済む)
- minimal と astroshop で resource limits 等が将来 divergence する余地
- duplication コスト < 学習可読性メリット

---

## 4. Grafana Datasources + Dashboard (Q4=A + Q5=A)

### 4.1 `datasources.yaml` (Kustomize configMapGenerator source)

```yaml
apiVersion: 1
datasources:
  - name: Tempo
    type: tempo
    access: proxy
    url: http://tempo:3200
    isDefault: false
  - name: Prometheus
    type: prometheus
    access: proxy
    url: http://prometheus:9090
    isDefault: true
  - name: Loki
    type: loki
    access: proxy
    url: http://loki:3100
    isDefault: false
```

### 4.2 Grafana Deployment volume mount

`grafana.yaml`:
```yaml
spec:
  template:
    spec:
      containers:
        - name: grafana
          image: grafana/grafana:11.3.0
          env:
            - { name: GF_AUTH_ANONYMOUS_ENABLED, value: "true" }
            - { name: GF_AUTH_ANONYMOUS_ORG_ROLE, value: "Editor" }
          volumeMounts:
            - { name: datasources, mountPath: /etc/grafana/provisioning/datasources }
            - { name: dashboards-config, mountPath: /etc/grafana/provisioning/dashboards }
            - { name: dashboards-data, mountPath: /var/lib/grafana/dashboards }
      volumes:
        - name: datasources
          configMap: { name: grafana-datasources }
        - name: dashboards-config
          configMap: { name: grafana-dashboards-config }   # provisioning config
        - name: dashboards-data
          configMap: { name: grafana-dashboards }          # actual JSON
```

### 4.3 Sample dashboard JSON (3 panels)

`dashboard-overview.json` は 3 panel minimal:
```json
{
  "title": "xk6-otel-gen Overview",
  "panels": [
    {
      "type": "table",
      "title": "Recent Traces (Tempo)",
      "datasource": "Tempo",
      "targets": [{ "query": "{}", "limit": 20 }]
    },
    {
      "type": "timeseries",
      "title": "k6 Iterations Rate",
      "datasource": "Prometheus",
      "targets": [{ "expr": "sum by (service_name) (rate(k6_iterations_total[1m]))" }]
    },
    {
      "type": "logs",
      "title": "Recent Logs (Loki)",
      "datasource": "Loki",
      "targets": [{ "expr": "{service_name=~\".+\"}" }]
    }
  ]
}
```

実際の Grafana JSON schema に合わせて NFR Design / Code Generation 時に正確化。

---

## 5. dependabot config (Q6=A)

### 5.1 `.github/dependabot.yml`

```yaml
version: 2
updates:
  - package-ecosystem: "gomod"
    directory: "/"
    schedule: { interval: "weekly" }
    groups:
      otel:
        patterns: ["go.opentelemetry.io/*"]
      k6:
        patterns: ["go.k6.io/*"]

  - package-ecosystem: "docker"
    directory: "/examples/minimal/k8s"
    schedule: { interval: "monthly" }

  - package-ecosystem: "docker"
    directory: "/examples/astroshop/k8s"
    schedule: { interval: "monthly" }

  - package-ecosystem: "github-actions"
    directory: "/"
    schedule: { interval: "weekly" }
```

→ gomod (weekly grouped OTel / k6) + docker (monthly per example) + GitHub Actions (weekly)。

---

## 6. lychee config (Q7=A)

### 6.1 `.lychee.toml`

```toml
# Link check configuration for README.md and examples/*/README.md
exclude = [
  "localhost",
  "127.0.0.1",
  "example.com",
  "example.org",
]
max-retries = 3
timeout = 10
include-fragments = true
verbose = "info"

# Allow some patterns that are reachable from local dev only
exclude_path = [
  "node_modules",
  ".git",
]
```

### 6.2 CI workflow snippet

```yaml
- name: README link check
  uses: lycheeverse/lychee-action@v1
  with:
    args: --config .lychee.toml README.md 'examples/**/README.md'
  env:
    GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
```

実 workflow は Build and Test stage で配置。

---

## 7. `.claude/skills/sync-astroshop/SKILL.md` (Q8=A)

### 7.1 ファイル format

```markdown
---
name: sync-astroshop
description: Review the OpenTelemetry Demo (open-telemetry/opentelemetry-demo) upstream repo and propose updates to examples/astroshop/topology.yaml. Use when the OTel Demo has a new release to evaluate against the current astroshop snapshot.
---

# Sync astroshop with OpenTelemetry Demo upstream

This skill helps drive the annual review of the astroshop topology
against the upstream OpenTelemetry Demo project (open-telemetry/
opentelemetry-demo).

## When to use

- Annual upstream review of OTel Demo (typically aligned with a major
  OTel Demo release tag)
- When a contributor reports that astroshop deviates noticeably from
  the latest OTel Demo

## Out of scope

- Daily / monthly automatic sync (intentionally manual to keep this
  project independent of upstream churn)
- Code-level dependency updates (use dependabot instead)

## Steps

### 1. Survey upstream

- Fetch the latest released tag of open-telemetry/opentelemetry-demo
- Read its docker-compose.yml and the per-service descriptions
- Build a mental model of the dependency graph: services × edges ×
  protocols × known fault scenarios

### 2. Diff against current astroshop

- Open `examples/astroshop/topology.yaml`
- Compare:
  - Service set (additions / removals)
  - Edge set (new dependencies, removed dependencies, protocol changes)
  - Journey set (browse / search / add-to-cart / checkout / place-order)
  - Fault set (any new chaos / failure demonstrations)

### 3. Propose changes

Draft a PR-quality change list with:
- One bullet per service add/remove
- One bullet per edge add/remove/modify
- One bullet per journey add/remove/modify
- One bullet per fault add/remove/modify

Each bullet should reference the upstream commit or release notes that
justifies the change.

### 4. Apply checklist

Before opening a PR:
- [ ] topology.yaml passes `topology.Parse + Validate`
- [ ] `xk6-otel-gen-schema` JSON Schema is still valid for the new
      topology shape (re-export if any new field used)
- [ ] examples/astroshop/script.js still references valid journey
      names
- [ ] k8s manifests under examples/astroshop/k8s/ still reflect
      desired runtime cardinality (memory limits, etc.)
- [ ] examples/astroshop/README.md mentions the upstream release tag
      this snapshot is based on (update the version reference)

## Anti-patterns

- Do NOT attempt full 1:1 reproduction of upstream — the goal is "what
  if you described this topology in xk6-otel-gen YAML?", not running
  the actual OTel Demo workload
- Do NOT block on minor upstream churn (single service rename, etc.) —
  fold into the next annual review

## Output

Produce a Markdown summary that lists the proposed changes with
rationale links, ready for a contributor to copy into a PR description.
```

### 7.2 配置

`.claude/skills/sync-astroshop/SKILL.md` のみ (helpers/snapshot 等は将来検討、NFR Design 時点では single-file)。

---

## 8. SPDX Header Enforce (Q9=A)

### 8.1 `.goheader.txt`

```text
SPDX-License-Identifier: Apache-2.0
```

### 8.2 `.golangci.yml` (project 共通設定に追記)

```yaml
linters:
  enable:
    - goheader
    # ... 既存 linters (revive, govet, staticcheck, errcheck, unused 等)

linters-settings:
  goheader:
    template-path: .goheader.txt
```

### 8.3 効果

- 全 `.go` ファイルに `// SPDX-License-Identifier: Apache-2.0` が必要
- 既存 U1-U6 のファイルも対象 (Code Generation 時に header が無いファイルがあれば backfill)
- CI で `golangci-lint run` が fail → DoD blocker

---

## 9. astroshop topology YAML readability (Q10=A)

### 9.1 section comments + service grouping

```yaml
# ============================================================
# xk6-otel-gen astroshop example topology
#
# Modeled after the OpenTelemetry Demo (astronomy shop) v<X.Y.Z>.
# See .claude/skills/sync-astroshop/SKILL.md for upstream review.
# ============================================================

services:
  # ------------------------------------------------------------
  # 1. Frontend & API tier (5 services)
  # ------------------------------------------------------------
  frontend:
    kind: application
    language: javascript
    framework: nextjs
    replicas: 2
    operations:
      ...

  ad:
    kind: application
    language: java
    ...

  # ------------------------------------------------------------
  # 2. Core commerce services (5 services)
  # ------------------------------------------------------------
  cart:
    kind: application
    ...

  checkout:
    ...

  # ------------------------------------------------------------
  # 3. Support services (4 services)
  # ------------------------------------------------------------
  ...

  # ------------------------------------------------------------
  # 4. Infrastructure dependencies (4 services)
  # ------------------------------------------------------------
  redis-cache:
    kind: cache
    ...

  postgres:
    kind: database
    ...
```

→ user が `Ctrl-F` で section heading 検索可能、500 行の YAML を navigable に。

### 9.2 inline service descriptions

各 service 直下に 1-2 行の comment で「OTel Demo での役割」を簡潔に:

```yaml
  cart:
    # User shopping cart; stores cart contents in Redis.
    kind: application
    language: dotnet
    ...
```

---

## 10. README markdown structure (Q11=A)

### 10.1 single-file with TOC

```markdown
# xk6-otel-gen

[![Go Version](https://img.shields.io/badge/go-1.25%2B-blue)]
[![License](https://img.shields.io/badge/license-Apache--2.0-green)]

> A k6 extension that synthesizes OpenTelemetry traces, metrics, and
> logs from a declarative topology, without running real microservices.

## Table of Contents
1. [Quick Start](#quick-start)
2. [Features](#features)
3. [Building](#building)
4. [Usage](#usage)
5. [Topology YAML Reference](#topology-yaml-reference)
6. [Configuration](#configuration)
7. [Examples](#examples)
8. [Security](#security)
9. [Contributing](#contributing)
10. [License](#license)
11. [Compatibility](#compatibility)

## Quick Start
...

## Features
...

[... 各 section ...]
```

### 10.2 anchor links

GitHub auto-generated anchors を使用 (lower-kebab-case)。lychee で `include-fragments = true` が anchor 整合を verify。

---

## 11. cleanup commands in README (Q12=A)

`examples/<example>/k8s/README.md` 末尾に:

```bash
## Cleanup

```bash
# Delete the demo namespace (removes all resources)
kubectl delete namespace xk6-otel-gen-demo

# Optionally delete the kind cluster entirely
kind delete cluster --name xk6-otel-gen-demo
```
```

→ shell script / Makefile は提供しない、user が copy-paste で十分 (Q12=A)。

---

## 12. NFR-R 各項目との対応

| NFR-R | 対応する Design パターン |
|---|---|
| NFR-U8-1 (API stability) | §1 cmd の args 安定、examples path 固定 |
| NFR-U8-2 (cmd reliability) | §1.2 main_test.go で error cases test |
| NFR-U8-3 (perf — no target) | bench なし、measurement なし |
| NFR-U8-4 (examples validation) | §2 test/examples/examples_test.go |
| NFR-U8-5 (link integrity) | §6 lychee |
| NFR-U8-6 (documentation completeness) | §10 single-file README with TOC + 12 sections |
| NFR-U8-7 (image bump monthly) | §5 dependabot config |
| NFR-U8-8 (SPDX header) | §8 goheader linter |
| NFR-U8-9 (CI build check) | §1 + xk6 build (Build and Test stage で actual CI workflow 定義) |
| NFR-U8-10 (Security) | README §5.x Security section + SECURITY.md placeholder |
| NFR-U8-11 (Astroshop maintainability) | §7 SKILL.md |
| NFR-U8-12 (cmd coverage 70%) | §1.2 main_test.go |
| NFR-U8-13 (Compatibility) | §10 README Compatibility section |

---

## 13. Anti-patterns (採用しない)

| アンチパターン | 不採用理由 |
|---|---|
| Single `main()` test 不可 (Q1 案 B) | Coverage 70% 達成不可 |
| interface-based Runner (Q1 案 C) | over-engineering |
| 各 example 内 _test.go (Q2 案 C) | example dir に Go 混入 |
| kustomize base + overlays (Q3 案 B) | learning barrier |
| Helm chart (Q3 案 C) | Q7-FD で rejected |
| inline grafana ConfigMap (Q4 案 B) | grafana.yaml 巨大化 |
| multi-panel rich dashboard (Q5 案 B) | over-engineering for sample |
| separate dependabot config (Q6 案 C) | single .github/dependabot.yml で十分 |
| markdown-link-check (Q7 案 B) | Node.js dep 追加 |
| shell grep for SPDX (Q9 案 B) | fragile |
| multiple YAML files (Q10 案 B) | Schema は single YAML 期待 |
| multi-file README (Q11 案 B) | user が彷徨う |
| cleanup.sh / Makefile (Q12 案 B/C) | maintenance 負担 |
| examples flat 化 (Q13 案 B) | structure 失う |

---

## 14. Final summary

- U8 は **deliverable focus**: cmd Go file 1 + examples static files + project docs + AI skill
- cmd は thin testable `run()` で 70% coverage 達成
- examples は kustomize-friendly k8s manifests + LGTM-lite stack
- AI skill (`.claude/skills/sync-astroshop/SKILL.md`) で年 1 回 upstream review を automate
- dependabot で monthly docker bump + weekly gomod
- 13 anti-patterns 全て documented
- これが **最終 NFR Design**
