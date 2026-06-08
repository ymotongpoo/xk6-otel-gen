# Unit of Work Plan — xk6-otel-gen

## 目的

Application Design で識別した 6 コンポーネント (+補助) を **Construction フェーズの per-unit loop の対象集合 (= ユニット)** に確定する。各ユニットは Functional Design / NFR Requirements / NFR Design / Code Generation を 1 ループとして経るため、ユニット境界・依存関係・実装順序を明確にしておく。

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/inception/application-design/unit-of-work.md` — ユニット定義、責務、コード配置、Definition of Done
- [ ] `aidlc-docs/inception/application-design/unit-of-work-dependency.md` — ユニット間依存マトリクスと推奨ビルド順序 (Mermaid 図含む)
- [ ] `aidlc-docs/inception/application-design/unit-of-work-traceability.md` — User Stories は不在なので代替として、**requirements.md の FR/NFR とユニットの追跡マッピング**
- [ ] **Greenfield code organization**: ワークスペース直下のディレクトリレイアウト確定 (Application Design で既定 — 確認のみ)
- [ ] ユニット境界・依存・FR 網羅の検証

---

## 暫定ユニット候補 (Application Design からの引き継ぎ)

| Unit ID | 名前 | パッケージ | レイヤ | Application Design でのコンポーネント |
|---|---|---|---|---|
| U1 | Topology Schema & Parser | `topology/` | Domain | C1 |
| U2 | Journey Engine | `journey/` | Application | C2 |
| U3 | Signal Synthesizer | `synth/` | Application | C3 |
| U4 | OTLP Exporter Pipeline | `exporter/` | Infrastructure | C4 |
| U5 | k6 JS Module | `k6otelgen/` | Boundary | C5 |
| U6 | k6 Output Module | `k6output/` | Boundary | C6 |
| U? | (Test Utilities) | `testutil/generators/` | — | 補助 |
| U? | (Shared Registry) | `registry/` | — | 候補のみ |
| U? | (Samples & Distribution) | `examples/`, `cmd/`, build config | — | 補助 |

? のついた 3 つは「独立ユニットにするか / 他ユニットに統合するか」を Q4・Q5・Q6 で確定します。

---

## 設計確定のための質問

### Question 1: 6 主要ユニットをそのまま採用するか

Application Design で確定した 6 コンポーネント (U1〜U6) をそのまま Construction の per-unit loop の対象にしますか?

A) **そのまま採用** — U1〜U6 を 6 ユニットとして per-unit loop を回す (推奨)

B) 統合: U1+U2 をまとめて 1 ユニットにする (topology+journey は密結合)

C) 分割: U3 (synth) を信号別 (traces / metrics / logs) のサブユニットに分けたい

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 2: 実装順序 (Construction の進行順)

Construction フェーズの per-unit loop は順次実行 (= 1 ユニット完了してから次へ) です。どの順序で進めますか?

A) **依存ボトムアップ順** — `U1 topology` → `U4 exporter` → `U3 synth` → `U2 journey` → `U5 k6otelgen` → `U6 k6output`。下位レイヤから順に実装、上位ユニットは下位の API を直接使える状態で着手 (推奨)

B) **業務優先順** — `U1 topology` → `U2 journey` → `U3 synth` → `U4 exporter` → `U5 k6otelgen` → `U6 k6output`。ドメインロジックから boundary 方向へ

C) **リスクファースト** — 不確実性の高いものから着手 (例: `U6 k6output` のデュアル機能、`U4 exporter` の OTel SDK 統合)

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 3: 並列着手の可能性

Multi-agent ワークフロー (Codex + Cursor) で複数ユニットを並列に進める可能性はありますか?

A) **完全逐次** — 1 ユニットずつ承認ゲート経由で進める。レビュー単位がクリアになる (推奨、設計レビューが追いつきやすい)

B) **依存無しユニットの並列着手** — 例: `U1 topology` 完了後、`U3 synth` と `U4 exporter` は互いに非依存なので並行で着手可能

C) **完全並列** — 全ユニットの Functional Design を先に出してから、Code Generation を一気に走らせる

X) Other (please describe after [Answer]: tag below)

[Answer]: A 

---

### Question 4: `testutil/generators/` の位置づけ

PBT 用のドメインジェネレータは複数ユニットで共有されます。これを独立ユニットにしますか?

A) **独立ユニット (U7) として最初に設計** — `topology` 等の Functional Design で出てくる "Testable Properties" を受けて、PBT-07 (Generator Quality) に従う集約パッケージを設ける。**Topology より前**に骨格だけ作り、各ユニットの FD 進行と並走で拡張 (推奨)

B) **独立ユニットだが最後に設計** — 各ユニット完成後にまとめてジェネレータを集約

C) **独立させない** — 各ユニットがそれぞれ自前のジェネレータを持つ (重複容認)

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 5: `registry/` (共有 Pipeline holder) の位置づけ

`k6otelgen` と `k6output` が同一 `exporter.Pipeline` を共有するための singleton holder です。

A) **独立ユニットにせず、`U4 exporter` の内部 API として実装** — Pipeline 構築の責務に近いので、別パッケージにする必要性が薄い (推奨、最も簡潔)

B) **小さな独立ユニット `registry/` として切り出す** — `Set/Get` だけの薄いパッケージ、k6 統合系の対称性を保つため C5/C6 の両方が均等にアクセス

C) **`pkg/k6otelgen` 内に `GetSharedPipeline()` を公開し、C6 が C5 を import** — 非推奨 (component-dependency.md で除外済み)

X) Other (please describe after [Answer]: tag below)

[Answer]: A 

---

### Question 6: `examples/` と Distribution の位置づけ

サンプルトポロジー、サンプル k6 スクリプト、README、xk6 ビルド手順、CI、リリースは Construction の per-unit loop に乗せますか?

A) **独立ユニット (U_dist) として Construction 末尾** — Code Generation で examples を作成し、Build and Test ステージで CI/release を完成させる (推奨)

B) **各ユニットの Code Generation 内で都度サンプル更新** — examples が各ユニット完成と同時に育つ

C) **Construction 外で扱う** — Build and Test ステージで全部完成させる (per-unit loop に含めない)

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 7: ユニットの "Definition of Done" (DoD)

各ユニットが Construction の per-unit loop を完了したと判定する基準は?

A) **AGENTS.md の §7 と同じ** — `go build`, `go test -race`, `golangci-lint`, PBT 完備, code-generation-plan.md の全 checkbox `[x]`, 残課題は `TODO(agent):` (推奨、既に文書化済み)

B) A に加えて **integration test pass** を必須にする — 他ユニットとの結合テストまで通すまで完了とみなさない

C) A に加えて **NFR-1 (1k RPS) のローカル検証** を完了基準にする — 性能テストまで含める

X) Other (please describe after [Answer]: tag below)

[Answer]: A 

---

### Question 8: Traceability マッピングの粒度

User Stories がない代わりに、FR/NFR とユニットを対応付ける `unit-of-work-traceability.md` をどの粒度で作りますか?

A) **FR-1〜FR-9 / NFR-1〜NFR-6 をユニットにマップ** — Application Design の §4 表を流用・更新するレベル (推奨、コンパクト)

B) FR / NFR の **個別サブ項目 (FR-2.1, FR-2.2 ...) 単位** で詳細マップ — 監査向けに細かく

C) PBT-01〜PBT-10 のルールもユニットに対応付けて含める — PBT 拡張 (Full enforcement) の compliance 追跡を兼ねる

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 9: ユニット内 sub-module の事前定義

Application Design では `synth/attributes`, `synth/resources`, `synth/trace`, `synth/metric`, `synth/log` のような sub-module を示唆しました。これらは Units Generation で確定しますか、Functional Design で各ユニット内で確定しますか?

A) **Functional Design に委ねる** — Units Generation ではトップレベルのパッケージ境界 (`synth/`) だけを確定し、内部分割は各ユニットの Functional Design で決める (推奨、過剰設計を避ける)

B) **Units Generation で詳細まで確定** — `synth/attributes` 等の sub-package 分割もここで決め、Functional Design の起点を明確にする

X) Other (please describe after [Answer]: tag below)

[Answer]: A 

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 3 つのアーティファクトを生成して承認ゲートへ進みます。
