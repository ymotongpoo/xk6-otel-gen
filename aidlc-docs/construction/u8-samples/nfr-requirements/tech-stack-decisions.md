# U8 Samples & Distribution — Tech Stack Decisions

本書は U8 (`examples/` + `cmd/xk6-otel-gen-schema/` + project root) が依存するパッケージ・採用された代替案・却下された案を確定する。

---

## 1. 依存モジュール (Production code)

### 1.1 `cmd/xk6-otel-gen-schema/` の依存

| Module | Version | 用途 | 必要性 |
|---|---|---|---|
| `flag` (stdlib) | Go 1.25 | CLI args parsing | 必須 |
| `fmt`, `io`, `os` (stdlib) | Go 1.25 | I/O | 必須 |
| `github.com/ymotongpoo/xk6-otel-gen/topology` | (local) | `ExportJSONSchema()` | 必須 |

→ 外部依存 zero (k6 SDK / sobek / OTel SDK もここでは不要)。

### 1.2 examples/ の依存

| 依存 | 用途 |
|---|---|
| YAML files | 静的 file、Go dep 不要 |
| k6 binary (xk6 build 経由) | runtime — user が build |
| LGTM-lite stack containers (Docker images) | runtime — kind/Kubernetes 経由で pull |

→ Go code dependency なし、container image のみ。

---

## 2. Container Image Dependencies (NFR-U8-7 monthly bump policy)

| Image | Initial pinned version | Bump cadence | Update process |
|---|---|---|---|
| `otel/opentelemetry-collector-contrib` | `0.105.0` | monthly | dependabot docker ecosystem |
| `grafana/tempo` | `2.6.0` | monthly | dependabot |
| `grafana/loki` | `3.2.0` | monthly | dependabot |
| `prom/prometheus` | `v2.55.0` | monthly | dependabot |
| `grafana/grafana` | `11.3.0` | monthly | dependabot |

NFR Design / Code Generation 時に latest stable に refresh。

---

## 3. テスト依存 (Test-only)

### 3.1 `cmd/xk6-otel-gen-schema/` test

| Module | Version | 用途 |
|---|---|---|
| `testing` (stdlib) | Go 1.25 | basic test framework |
| `github.com/stretchr/testify` | latest stable | assertion |

→ PBT 不採用 (Q12=A in FD)、rapid 依存なし。

### 3.2 examples test (NFR-U8-4)

| Module | Version | 用途 |
|---|---|---|
| `testing` | stdlib | go test framework |
| `github.com/ymotongpoo/xk6-otel-gen/topology` | (local) | Parse + Validate for example yamls |

`go test ./test/examples/...` (新規 test ディレクトリ) を CI で run。

---

## 4. CI / Tool 依存

### 4.1 build / test tools

| Tool | Minimum version | 用途 |
|---|---|---|
| Go | 1.25+ | build |
| xk6 | latest (k6 pin) | xk6 build verification (CI 環境で integration phase) |
| golangci-lint | latest stable | lint + SPDX header enforce |

### 4.2 example validation tools

| Tool | Version | 用途 |
|---|---|---|
| `kubectl` | 1.27+ | k8s manifest dry-run |
| `kind` (optional) | 0.20+ | local cluster for full integration |
| `lychee` (or `markdown-link-check`) | latest | README link check |
| Docker | latest stable | container backend for xk6 build |

### 4.3 release / supply chain (将来検討)

| Tool | 状態 |
|---|---|
| `cyclonedx-gomod` (SBOM) | future — NFR-U8-10 で言及、本 unit では実装しない |
| `cosign` (signing) | not adopted — prebuilt 配布しないため (FD Q10=A) |

---

## 5. AI-assisted Maintenance Tools (NFR-U8-11)

### 5.1 `.claude/skills/sync-astroshop/SKILL.md`

| 項目 | 内容 |
|---|---|
| 配置場所 | `.claude/skills/sync-astroshop/SKILL.md` (Claude Code skill convention) |
| 用途 | OTel Demo upstream (`open-telemetry/opentelemetry-demo`) との sync を AI ツールが実行できるよう手順を encode |
| Trigger | manual (`/sync-astroshop` 等の slash command で Claude が呼び出す) |
| 出力 | astroshop topology.yaml の修正提案 + review checklist |

### 5.2 Skill 構造 (NFR Design で詳細化、本 NFR-R では要件のみ)

```text
.claude/skills/sync-astroshop/
├── SKILL.md          ← frontmatter (name, description) + content
└── (optional helpers, e.g., reference snapshot)
```

Claude Code が skill を発見・呼び出す convention に従う。

---

## 6. Version 戦略

### 6.1 Container image bump cadence (NFR-U8-7 Q4=A user-modified)

**Monthly** dependabot PR を期待。具体的 dependabot config:

```yaml
# .github/dependabot.yml (Build and Test stage で配置)
version: 2
updates:
  - package-ecosystem: "docker"
    directory: "/examples/minimal/k8s/"
    schedule:
      interval: "monthly"
  - package-ecosystem: "docker"
    directory: "/examples/astroshop/k8s/"
    schedule:
      interval: "monthly"
```

### 6.2 Go module dependency

- `cmd/xk6-otel-gen-schema/` は project root の `go.mod` を共有 (single module)
- 外部依存なし、bump 不要

### 6.3 k6 SDK / OTel SDK (transitive)

- U4/U5/U6 が pin、U8 は transitive のみ
- xk6 build 経由で resolve、U8 の責務外

---

## 7. 代替案 (Rejected)

### 7.1 Docker Compose (rejected by user)

- User 指摘により Kubernetes manifests を採用
- Docker Compose は simple だが production-like deployment 学習価値が低い

### 7.2 Jaeger (rejected by user)

- User 指摘により Tempo + Grafana の LGTM-lite stack を採用
- Tempo は OTLP native、Grafana は 3 signals 統一 UI

### 7.3 spf13/cobra (rejected, FD Q8)

- 案: `xk6-otel-gen-schema` を cobra で実装
- 却下: subcommand なしの single binary、stdlib `flag` で十分

### 7.4 Pre-built binary distribution (rejected, FD Q10)

- 案: GitHub release で k6+xk6-otel-gen binary 配布
- 却下: supply chain risk、user に xk6 build を強く推奨

### 7.5 Automated OTel Demo sync (rejected, NFR Q9)

- 案: script で OTel Demo の構造を pull、topology.yaml 自動生成
- 却下: 実装コスト大、年 1 回 manual review + AI skill で十分

### 7.6 Helm chart 配布 (rejected, Out of Scope)

- 案: examples を Helm chart として packaging
- 却下: kustomize で十分、Helm の operational complexity を user に課したくない

### 7.7 Six-monthly image bump (rejected by user)

- 案: dependabot 半年ごと
- 却下: user 指摘 "半年は遅い、1ヶ月" → monthly に変更

---

## 8. CI / Lint 統合

### 8.1 必須 CI ジョブ

| ジョブ | コマンド | DoD blocking? |
|---|---|---|
| Build | `go build ./cmd/...` | Yes |
| Unit test | `go test -race -count=1 ./cmd/...` | Yes |
| Coverage | `go test -cover ./cmd/...` ≥ 70% | Yes |
| Lint | `golangci-lint run ./cmd/...` (with SPDX header enforce) | Yes |
| xk6 build | `xk6 build --with .` | Yes |
| Examples validate | `go test ./test/examples/...` (Parse + Validate) | Yes |
| k8s dry-run | `kubectl apply --dry-run=server -k examples/*/k8s/` | nightly / on PR with cluster |
| Broken link | `lychee README.md examples/*/README.md` | weekly cron |
| Image tag bump | dependabot monthly PR | informational (auto-PR) |

### 8.2 lint rules

`.golangci.yml` で project 共通設定 (U2-U6 と同じ):
- `revive`, `govet`, `staticcheck`, `errcheck`, `unused`
- + `goheader` (SPDX header enforce for cmd/)

---

## 9. Cross-unit dependency summary

```text
U8 imports:
  - cmd/xk6-otel-gen-schema/main.go uses:
    - flag, fmt, io, os (stdlib)
    - github.com/ymotongpoo/xk6-otel-gen/topology

U8 does NOT import:
  - k6/x/otel-gen module (cmd は k6 と独立した CLI)
  - exporter / synth / journey / k6output (cmd は schema export のみ)
  - external Go dependencies (zero ext deps)

U8 deliverables consumed by:
  - end-users via xk6 build + kubectl apply
  - AI tools via `.claude/skills/sync-astroshop/SKILL.md` (NFR-U8-11)
```

---

## 10. Migration / Upgrade Notes

### 10.1 OTel Collector / Tempo / Loki / Grafana version bump

- monthly dependabot PR
- breaking change は changelog 確認、k8s manifest 調整 (config schema 変更等)
- 例: Tempo の `storage.trace.backend` の API 変更

### 10.2 OTel Demo upstream sync

- 年 1 回 manual review
- AI skill (`.claude/skills/sync-astroshop`) を活用
- diff が大きければ separate PR で adjustment

### 10.3 topology.ExportJSONSchema 変更

- U1 の `ExportJSONSchema()` signature 変更時、`cmd/xk6-otel-gen-schema/main.go` を更新
- minor SemVer bump on U1

---

## 11. Open questions for Future revisit

| 質問 | 想定 trigger |
|---|---|
| SBOM generation (cyclonedx-gomod) | release process が成熟したら |
| Helm chart 提供 | community demand があれば |
| Pre-built binary + cosign signing | release pipeline が安定したら検討 |
| OTel Demo automated sync | manual sync が負担になったら検討 |
| LGTM stack 全部 (Mimir 追加) | Prometheus → Mimir migration 要求があれば |
| Grafana cloud integration example | community demand があれば |
