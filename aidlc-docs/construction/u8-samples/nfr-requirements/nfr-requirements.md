# U8 Samples & Distribution — Non-Functional Requirements

本書は U8 (`examples/` + `cmd/xk6-otel-gen-schema/` + project root) の **deliverable 品質要件** を確定する。

> **NOTE on Performance budgets** (per Q1=C user-relaxed): cmd CLI の latency target は持たない (best-effort、~1 sec まで許容)。U8 はそもそも build 時 / one-shot 系の deliverable 中心、hot path がない。

参照:
- FD: `aidlc-docs/construction/u8-samples/functional-design/`
- Plan + Answers: `aidlc-docs/construction/plans/u8-samples-nfr-r-plan.md`
- Tech stack: `tech-stack-decisions.md` (本ディレクトリ内)

---

## 1. Applicable NFR (本 unit で扱う)

### NFR-U8-1: API Stability (Q11=A)

| 項目 | 内容 |
|---|---|
| 要件 | post-v1 公開後 SemVer 厳守 |
| CLI args | `xk6-otel-gen-schema -output <path>` の flag set は contract。flag 追加は minor、削除/意味変更は major |
| Examples path | `examples/minimal/`, `examples/astroshop/` 等の **directory 名 / file 名** も SemVer 対象 (user の手元 script が依存) |
| 検証 | unit test + `apidiff` 相当の CLI diff (manual review) |

### NFR-U8-2: cmd/xk6-otel-gen-schema/ Reliability

| 項目 | 内容 |
|---|---|
| 要件 | `xk6-otel-gen-schema` が **schema 出力に成功 or 明確 error message + exit code 1** で終了 |
| Error sources | `topology.ExportJSONSchema` 失敗 / file create 失敗 / write 失敗 |
| 検証 | `cmd/xk6-otel-gen-schema/main_test.go` で table-driven test |

### NFR-U8-3: Performance (Q1=C — soft, best-effort)

| Operation | 性質 |
|---|---|
| `xk6-otel-gen-schema [-output ...]` 実行時間 | **no explicit target** (Q1=C "1秒とかでも全然問題ない、best effort") |
| build only — measurement なし | — |

### NFR-U8-4: Examples Validation (Q2=A)

| 項目 | 内容 |
|---|---|
| topology.yaml validation | `go test ./test/examples/...` (新規追加可) 内で各 example の `topology.yaml` を `topology.Parse` + `Schema.Validate()` に通す。失敗で CI fail |
| k8s manifest dry-run | `kubectl apply --dry-run=server -k examples/<example>/k8s/` を CI workflow で実行 (cluster 必要) |
| script.js syntax | `xk6 build --with .` + `k6 archive examples/<example>/script.js` で syntax check (CI で実行) |
| Triggering | PR + main branch push |

### NFR-U8-5: README Link Integrity (Q3=A)

| 項目 | 内容 |
|---|---|
| 要件 | `README.md` (project root + examples 内) の URL が全て resolvable |
| Tool | `lychee` (推奨) or `markdown-link-check` |
| External link | HEAD request、3xx/4xx/5xx で fail |
| Internal link | file/anchor existence check |
| Trigger | weekly cron + PR diff |
| 例外 | `localhost`, `127.0.0.1`, `*.example.com` は skip |

### NFR-U8-6: Documentation Completeness (Q5=A)

| 項目 | 内容 |
|---|---|
| 要件 | README が以下を含む: |
| - Project description | (1-2 段落) |
| - Quick Start | xk6 build + kind + kubectl apply + k6 run の 5-step |
| - Features | bullet list |
| - Building | `xk6 build --with github.com/ymotongpoo/xk6-otel-gen` |
| - Usage (JS API) | configure / load / runJourney / stats / journeys |
| - Topology YAML Reference | brief + link to detailed doc + `xk6-otel-gen-schema` 使い方 |
| - Configuration | JS API > `--out` > env > default priority |
| - Examples | examples/minimal/, examples/astroshop/ への link + brief description |
| - Security | self-build only stance + rationale |
| - Contributing | brief contribution guide |
| - License | Apache-2.0 |
| 各 section の最低限 example | 1 つ以上の concrete example/code block |
| 検証 | manual review at PR、+ `markdown-toc` 系で section 構造 lint (任意) |

### NFR-U8-7: Image Tag Pinning + Monthly Bump (Q4=A user-modified)

| 項目 | 内容 |
|---|---|
| 要件 | Collector / Tempo / Loki / Prometheus / Grafana の image は **specific tag pin** (`grafana/tempo:2.6.0` 等) |
| Bump 頻度 | **monthly dependabot PR** (Q4 user change: 半年 → 1ヶ月) |
| Bump 手順 | dependabot が PR 作成 → CI で kubectl dry-run + k6 build green を確認 → manual merge (breaking change なら手動 adjust) |
| 検証 | `.github/dependabot.yml` に docker tag bump ecosystem を設定 |
| 影響 file | `examples/*/k8s/*.yaml` のすべての `image:` 行 |

### NFR-U8-8: License & SPDX Header Consistency (Q6=A)

| 項目 | 内容 |
|---|---|
| 要件 | 全 `.go` ファイルに SPDX header `// SPDX-License-Identifier: Apache-2.0` |
| 適用範囲 | `cmd/xk6-otel-gen-schema/*.go` + test file 含む |
| Optional | examples 内 `.js` ファイル (SPDX header は optional、project LICENSE で covers) |
| 検証 | `golangci-lint` (project 既設定の goheader linter 等) で CI enforce |

### NFR-U8-9: CI Build Check (Q7=A)

| 項目 | 内容 |
|---|---|
| `go build ./cmd/...` | cmd binary が build 成功 |
| `xk6 build --with .` | k6 binary に linkable (`xk6` が利用可能な CI environment で) |
| Trigger | PR + main branch |
| Timeout | 5 分以内 |
| 検証 | CI green が DoD blocking |

### NFR-U8-10: Security & SBOM (Q8=A)

| 項目 | 内容 |
|---|---|
| Pre-built binary 配布 | **しない** (FD Q10=A + README で明示) |
| README security section | supply chain reasoning + xk6 build 推奨 |
| `SECURITY.md` | **placeholder** (project が成熟したら詳細追加、現状 TODO 状態を明記) |
| SBOM | **将来検討** (cyclonedx-gomod 等で CI 生成案あり、本 unit では実装しない) |
| GPG signature | **不要** (prebuilt なしのため) |
| 検証 | README content review at PR |

### NFR-U8-11: Astroshop Maintainability (Q9=A + project skill)

| 項目 | 内容 |
|---|---|
| Upstream OTel Demo 追随 | 作成時 snapshot を base、**年 1 回程度 manual review** |
| **AI-assisted sync skill** | `.claude/skills/sync-astroshop/SKILL.md` を Code Generation で作成。Claude Code 等 AI ツールが OTel Demo upstream の commit diff を読んで本 project の `examples/astroshop/topology.yaml` に反映する手順を encode |
| Skill 内容 | (1) OTel Demo repo の dependency graph を読む、(2) 本 project の topology.yaml 形式と差分対比、(3) 追加 service / journey / fault の提案、(4) 不一致 review checklist |
| Trigger | manual (user が "year-end review" 等のタイミングで AI ツール経由で実行) |
| 検証 | skill 自体は documentation file、syntax check のみ |

> **NOTE**: project skill は Claude Code の skill 機構を活用。`.claude/skills/<name>/SKILL.md` ファイルが skill 定義となり、frontmatter で `name` / `description` を持つ。Code Generation 時に skill の draft を作成。

### NFR-U8-12: cmd/xk6-otel-gen-schema/ Coverage (Q10=A)

| 項目 | 内容 |
|---|---|
| Coverage target | **70%** (U1-U6 の 80% より低めに設定、CLI は test 簡素) |
| 理由 | `main()` 自体は test 困難 (`run(args, stdout, stderr) error` を分離して test するパターン) |
| 検証 | `go test -cover ./cmd/...` |

### NFR-U8-13: Compatibility Constraints (Q13=A)

| Tool | Minimum version | 用途 |
|---|---|---|
| Go | 1.25+ | xk6 build dependency |
| xk6 | latest (k6 pin) | k6 extension build |
| kubectl | 1.27+ | manifest apply |
| kind | 0.20+ | local cluster (optional) |
| Docker | latest stable | xk6 build container backend |

README に **minimum version table** を明記。

---

## 2. Non-Applicable NFR (Q12=A)

| カテゴリ | 理由 |
|---|---|
| Persistence | N/A — deliverable は static file |
| Authentication / Authorization | N/A |
| Encryption at rest | N/A |
| Encryption in transit | N/A (U4 が担保) |
| Internationalization (i18n) | N/A — 英語固定 |
| Accessibility (a11y) | N/A — 例外: Grafana dashboards に最低限の color contrast |
| GDPR / SOC2 / PCI | N/A — 合成データのみ |
| Production monitoring SLO | N/A — sample / docs unit |
| Disaster recovery | N/A |
| Multi-region | N/A |
| Backup / Restore | N/A |
| Capacity planning | N/A — user 側の cluster sizing |

---

## 3. Project NFR トレーサビリティ

| Project NFR | U8 で対応する項目 |
|---|---|
| R-NFR-001 (Performance) | NFR-U8-3 best-effort (no strict target) |
| R-NFR-002 (Reliability) | NFR-U8-2 cmd reliability, NFR-U8-4 example validation |
| R-NFR-003 (Observability) | examples が Grafana 統合で 3 signals を可視化 demonstrate |
| R-NFR-004 (Maintainability) | NFR-U8-7 monthly image bump, NFR-U8-11 project skill |
| R-NFR-005 (Compatibility) | NFR-U8-13 minimum version table |

---

## 4. Definition of Done (DoD)

U8 の Code Generation 完了条件:

- [ ] `go build ./cmd/...` succeeds
- [ ] `go vet ./cmd/...` clean
- [ ] `go test -race -count=1 ./cmd/...` passes
- [ ] `go test -cover ./cmd/...` shows ≥ 70%
- [ ] `xk6 build --with .` succeeds (CI で実行可能なら)
- [ ] `golangci-lint run ./cmd/...` passes (with SPDX header enforce)
- [ ] `examples/minimal/topology.yaml` + `examples/astroshop/topology.yaml` が `topology.Parse + Validate` を passes
- [ ] `kubectl apply --dry-run=server -k examples/*/k8s/` clean (cluster available env で)
- [ ] README.md (project root) に 12 sections + 各 section concrete example
- [ ] `examples/<example>/README.md` (per-example) 完備
- [ ] LICENSE (Apache-2.0 fulltext) at project root
- [ ] `.claude/skills/sync-astroshop/SKILL.md` 作成 (NFR-U8-11)
- [ ] dependabot config に docker tag bump ecosystem 追加 (monthly cadence)
- [ ] Broken link check (lychee 等) が CI で green

---

## 5. Risks & Mitigations

| Risk | Mitigation |
|---|---|
| OTel Demo upstream の大規模変更で astroshop が outdated | NFR-U8-11 の AI-assisted sync skill で年 1 回 manual review、不一致は note 化 |
| Image tag bump で breaking change | dependabot PR で CI dry-run + 手動 merge、changelog review |
| README link rot | weekly cron で lychee 実行、broken link は issue 自動作成 |
| User が prebuilt binary を期待 | README Security section で明示、xk6 build 案内 |
| Kubernetes cluster 不要 user (Docker のみの user) | future revisit、現状は kind cluster guidance のみ |

---

## 6. 関連他 unit への要求

| 依頼先 | 内容 |
|---|---|
| U1 (topology) | `ExportJSONSchema()` API が cmd から呼べる (済) |
| U5/U6 integration test | `--out otel-gen=...` flow が examples/<example>/script.js で動作 (済) |
| Build and Test stage | dependabot config 設定 + CI workflow (kubectl dry-run / lychee / xk6 build) |
| **Claude Code skill 機構** | `.claude/skills/sync-astroshop/SKILL.md` 形式の skill ファイルを認識 (project skill convention) |
