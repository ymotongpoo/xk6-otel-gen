# U8 (Samples & Distribution) — NFR Design Plan

## ユニットコンテキスト

- **Unit ID**: U8
- **パッケージ / ディレクトリ**: `examples/`, `cmd/xk6-otel-gen-schema/`, project root, `.claude/skills/sync-astroshop/`
- **FD**: `aidlc-docs/construction/u8-samples/functional-design/` (committed caa82a7)
- **NFR-R**: `aidlc-docs/construction/u8-samples/nfr-requirements/` (committed b75c735)
- **Construction order position**: U7 ✓ → U1 ✓ → U4 ✓ → U3 ✓ → U2 ✓ → U5 ✓ → U6 ✓ → **U8 (this — final NFR-D)**

## NFR Design の焦点

FD で「何をする」、NFR-R で「何を達成するか」を確定済。NFR Design は **「どう実装するか」のパターン** を確定する。

- **`cmd/xk6-otel-gen-schema/` の Go 実装構造** — `main()` と testable `run()` 関数の分離
- **examples 検証コードの配置** — `test/examples/` パッケージ or 別配置
- **k8s manifest authoring 方針** — kustomize layout の具体構造
- **Grafana datasource provisioning の具体ファイル**
- **Sample dashboard JSON の最低限内容**
- **dependabot config の YAML 構造**
- **lychee config**
- **`.claude/skills/sync-astroshop/SKILL.md` の内容と format**
- **SPDX header enforce の golangci-lint settings**
- **astroshop topology の structure を user が読みやすくする YAML layout**
- **README の具体 markdown structure**

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/construction/u8-samples/nfr-design/nfr-design-patterns.md`
- [ ] `aidlc-docs/construction/u8-samples/nfr-design/logical-components.md`

---

## 設計確定のための質問

### Question 1: cmd の testable structure

`cmd/xk6-otel-gen-schema/main.go` の test 可能化:

A) **`main()` を thin wrapper、`run(args []string, stdout, stderr io.Writer) int` を testable** (推奨):
```go
func main() {
    os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
    fs := flag.NewFlagSet("xk6-otel-gen-schema", flag.ContinueOnError)
    fs.SetOutput(stderr)
    output := fs.String("output", "", "output file path")
    if err := fs.Parse(args); err != nil { return 2 }
    // ... use stdout / output ...
    return 0
}
```

B) **single `main()`、test 不可** — coverage が低い

C) **interface-based design** — `Runner interface { Run() error }` で test mock 化、over-engineering

X) Other

[Answer]: A

---

### Question 2: examples test code 配置

`topology.yaml` の Validate test を CI で run する場所:

A) **`test/examples/examples_test.go` を新規追加** (推奨):
   - project root の `test/examples/` directory
   - 各 example の `topology.yaml` を loop で読み、Parse + Validate を assert
   - `_test.go` 接尾辞で go test 経由

B) **`cmd/xk6-otel-gen-schema/main_test.go` に同居** — schema との関連性、ただし scope が広がる

C) **`examples/<example>_test.go` を各 example dir 内に置く** — distributed、example dir 内に Go file が混ざる

X) Other

[Answer]: A

---

### Question 3: k8s manifest authoring 方針

`examples/<example>/k8s/*.yaml` の書き方:

A) **kustomize-friendly 単一 manifest per service** (推奨):
   - 1 file 1 service (collector.yaml は Deployment + Service + ConfigMap を含む 1 service set)
   - `kustomization.yaml` で resources list + configMapGenerator
   - inline ConfigMap で Tempo/Loki/Prometheus の config (file 参照より見やすい)

B) **`base/` + `overlays/` 構造** — kustomize 本来の使い方、minimal/astroshop の base を共有
   - DRY だが complexity 増、user 学習 barrier 増

C) **Helm chart 化** — 別 ecosystem、Q7-FD rejected

X) Other

[Answer]: A

---

### Question 4: Grafana datasource ファイル形式

`examples/<example>/k8s/datasources.yaml` の placement:

A) **`datasources.yaml` を k8s/ 直下に置き、Kustomize `configMapGenerator` で ConfigMap 化** (推奨)
   - `grafana.yaml` Deployment に volume mount

B) **`grafana.yaml` 内に直接 ConfigMap inline** — 1 file、ただし長くなる

C) **`grafana-config/` subdir で datasources.yaml + dashboards/ をまとめる** — directory 増

X) Other

[Answer]: A

---

### Question 5: Sample dashboard 内容

`examples/<example>/k8s/dashboard-overview.json` の content:

A) **3 panels (each 1 signal)** (推奨):
   - Panel 1: Tempo "Recent traces" (search query で最新 N traces 表示)
   - Panel 2: Prometheus "Request rate by service" (sum by (service_name) of rate(k6_iterations_total[1m]))
   - Panel 3: Loki "Recent logs" (`{service_name=~".+"}` log stream tail)
   - minimum viable demo

B) **multi-panel rich dashboard** (10+ panels) — 学習 example として overengineering

C) **No dashboard, datasource provisioning only** — user が自分で作る

X) Other

[Answer]: A

---

### Question 6: dependabot config 配置

`.github/dependabot.yml` の content (NFR-U8-7 monthly bump):

A) **5 docker image directory entries + 1 gomod entry** (推奨):
```yaml
version: 2
updates:
  - package-ecosystem: "gomod"
    directory: "/"
    schedule: { interval: "weekly" }
  - package-ecosystem: "docker"
    directory: "/examples/minimal/k8s/"
    schedule: { interval: "monthly" }
  - package-ecosystem: "docker"
    directory: "/examples/astroshop/k8s/"
    schedule: { interval: "monthly" }
```
gomod は weekly (Go SDK / OTel etc.)、docker は monthly。

B) **monthly のみ、gomod は weekly + grouped** — k6 / OTel SDK は同じ PR で

C) **separate config file per ecosystem** — `.github/dependabot.gomod.yml` + `.github/dependabot.docker.yml`

X) Other

[Answer]: A

---

### Question 7: lychee config

README link check の lychee config:

A) **`.lychee.toml`** に exclude pattern + max retry 等 (推奨):
```toml
exclude = ["localhost", "127.0.0.1", "example.com", "example.org"]
max-retries = 3
timeout = 10
include-fragments = true
```
+ CI workflow 内で `lychee README.md examples/*/README.md`

B) **markdown-link-check** (npm tool) — Node.js dependency

C) **手動 review のみ** — automation なし

X) Other

[Answer]: A

---

### Question 8: `.claude/skills/sync-astroshop/SKILL.md` の format

Claude Code skill ファイルの format:

A) **frontmatter + body markdown** (推奨、Claude Code convention):
```markdown
---
name: sync-astroshop
description: Review OTel Demo upstream changes and propose updates to examples/astroshop/topology.yaml. Use when the OTel Demo repository has new releases to evaluate.
---

# Sync astroshop with OTel Demo

## When to use
- Annual OTel Demo upstream review
- ...

## Steps
1. Read the latest OTel Demo dependency graph...
2. Compare against examples/astroshop/topology.yaml...
3. Propose service / journey / fault additions...
4. Apply checklist before opening PR...
```

B) **JSON schema with structured fields** — over-engineered

C) **shell script** — Claude が直接 execute、ただし AI driving の本質と異なる

X) Other

[Answer]: A

---

### Question 9: SPDX header enforce 実装

`golangci-lint` で SPDX header enforce:

A) **`goheader` linter を有効化** (推奨、golangci-lint 標準):
```yaml
# .golangci.yml
linters:
  enable: [goheader, ...]
linters-settings:
  goheader:
    template-path: .goheader.txt
```
`.goheader.txt`:
```
// SPDX-License-Identifier: Apache-2.0
```
+ CI で `golangci-lint run` 実行 → 各 `.go` ファイルが header を持つことを enforce

B) **shell script で grep check** — fragile

C) **header なしを許容** — Q6=A 違反

X) Other

[Answer]: A

---

### Question 10: astroshop topology YAML readability

`examples/astroshop/topology.yaml` (18 services) を user が読みやすくする工夫:

A) **section comments + service grouping** (推奨):
```yaml
# ============================================================
# Frontend & API services
# ============================================================
services:
  frontend:
    ...
  ad:
    ...

# ============================================================
# Core domain services
# ============================================================
  cart:
    ...
  checkout:
    ...
```
- comment header で群分け
- service 内も短い description comment

B) **multiple YAML files で merge** — Schema は 1 YAML 期待、複雑化

C) **TOC コメント** — file 先頭に inline TOC

X) Other

[Answer]: A

---

### Question 11: README markdown structure

README.md の具体 markdown 構造:

A) **single-file with TOC at top** (推奨):
   - 1 page で全 12 sections
   - 冒頭に Table of Contents (markdown links)
   - heading 階層: `#` title → `##` sections → `###` subsections

B) **multi-file with main README + docs/ subdir** — link 多用、user が彷徨う

C) **wiki / GitHub Pages 別途** — project root README は minimal

X) Other

[Answer]: A

---

### Question 12: examples の cleanup script

`examples/<example>/k8s/README.md` での cleanup 手順:

A) **`kubectl delete namespace xk6-otel-gen-demo` + `kind delete cluster` を README に明記** (推奨)
   - shell script 提供は不要、commands を README に書くだけ

B) **`cleanup.sh` script を examples 内に提供** — convenience だが maintenance 負担

C) **`make cleanup`** — Makefile 提供、ただし `make` 依存追加

X) Other

[Answer]: A

---

### Question 13: ファイル分割の最終確認

FD §3 (= NFR-D logical-components で再定義) の deliverable layout:

A) **そのまま採用** (推奨):
   - `examples/minimal/`, `examples/astroshop/` 各々: README.md / topology.yaml / script.js / otel-collector-config.yaml / k8s/ (kustomization + namespace + collector + tempo + loki + prometheus + grafana + datasources + dashboard-overview + README)
   - `cmd/xk6-otel-gen-schema/main.go + main_test.go`
   - project root: README.md, LICENSE
   - `.claude/skills/sync-astroshop/SKILL.md`
   - `.github/dependabot.yml`, `.lychee.toml`, `.goheader.txt`
   - `test/examples/examples_test.go` (新規 test 配置)

B) examples を flat 化、k8s/ subdir なし

C) 他構成

X) Other

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 2 つの NFR Design アーティファクトを生成して承認ゲートへ進みます。
