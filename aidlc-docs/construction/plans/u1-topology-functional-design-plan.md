# U1 (topology) — Functional Design Plan

## ユニットコンテキスト

- **Unit ID**: U1
- **パッケージ**: `topology/`
- **位置づけ**: Domain layer (Clean Architecture)。プロジェクト全体の型供給元
- **U7 からの引き継ぎ**: 型定義 (`Schema`, `Service`, `ServiceID`, `Operation`, `CallNode`, `Edge`, `Journey`, `Step`, `FaultTarget`, `FaultSpec`, `FaultOverlay`, `RecoveryPolicy`, `LatencyDist`, `SeverityParams`) と enum (`ServiceKind`, `Protocol`, `ExhaustedAction`, `FaultKind`, `TargetKind`, `BackoffPolicy`) は実装済み。**メソッド本体 (`Parse`, `ParseFile`, `Validate`, `MarshalYAML`, `Equal`, `ApplyFaults`, `ExportJSONSchema`, `FindServiceByName`, `JourneyNames`) は panic スタブ状態**。本 FD でその実装の業務ロジックを設計する
- **U1 が達成する PBT compliance**: PBT-01 (property identification)、PBT-02 (round-trip via `Equal(Parse(Marshal(s)), s)`)、PBT-03 (structural invariants)、PBT-04 (idempotency of Validate / ApplyFaults)

## 今回の FD スコープ

**Parse の業務ロジック**:
- YAML 1 パス目: `rawSchema` への decoding
- 2 パス目: 参照解決 (`{service, operation}` 文字列 → `*Operation` ポインタ)
- 解決失敗時のエラー形式とコンテキスト情報
- DAG 検証 (R-STR-4)
- ジャーニー/ファルト到達可能性

**MarshalYAML の業務ロジック**:
- `*Operation` ポインタ → `{service: <name>, operation: <name>}` への変換
- ネストされたファルバックチェーンの正しい順序維持
- YAML タグ・並び順の決定

**Validate の業務ロジック**:
- 構造的不変条件 (R-STR-1〜8) のチェックリスト
- エラー集約 vs fail-fast
- エラー位置情報 (YAML 行番号 or struct path)

**Equal の業務ロジック**:
- identifier-based deep equality (循環ポインタ回避)
- マップキー差分・スライス順序の扱い

**ApplyFaults の業務ロジック**:
- `FaultOverlay` の lookup 辞書構造
- カスケードは pre-compute せず実行時解決 (Application Design Q8 確定済み)
- O(1) アクセスのインデックス設計

**JSON Schema エクスポート**:
- どの仕様 (Draft 07 / 2020-12) を使うか
- description / example 含有

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/construction/u1-topology/functional-design/business-logic-model.md` — Parse 2-pass フロー、Marshaler 逆変換、Validate / ApplyFaults アルゴリズム、Equal の比較規則
- [ ] `aidlc-docs/construction/u1-topology/functional-design/business-rules.md` — 構造的不変条件 (R-STR-1..8 を topology 視点で再記述)、Validate のエラー分類、Default 値、Topological Properties (PBT-01)
- [ ] `aidlc-docs/construction/u1-topology/functional-design/domain-entities.md` — 既存型 (U7 scaffold) の方法論ベース解説、各メソッドの contract、エラー型階層
- [ ] **U7 への generator 追加リクエスト** — `domain-entities.md` 末尾に「U7 に必要な追加ジェネレータ」セクションを設け、U1 で必要になる `ValidOperation` / `ValidEdge` / `ValidCallNode` / `ValidRecoveryPolicy` / `ValidJourney` / `ValidStep` / `ValidFaultSpec` / `ValidFaultTarget` / `ValidFaultOverlay` を一覧化

---

## 設計確定のための質問

### Question 1: Parse のエラー報告スタイル

YAML パース or 参照解決でエラーが出たとき:

A) **fail-fast** — 最初のエラーで停止、行番号 + コンテキスト付きの単一エラーを返す (シンプル、シェル使用感に近い)

B) **集約報告** — すべての invalid 参照や不整合を集めて 1 つの multi-error として返す (大規模 YAML で全エラーを一度に確認可能)

C) **2 段階** — YAML 構文エラーは fail-fast、参照解決と Validate は集約 (推奨、人間に優しい)

X) Other (please describe after [Answer]: tag below)

[Answer]: C

---

### Question 2: 未知フィールドの取り扱い (YAML 厳密性)

YAML に **スキーマで未定義のキー** が含まれていた場合:

A) **strict** — エラーで停止 (タイポ防止に有効、`yaml.v3` の `KnownFields(true)` 相当)

B) **lax** — 警告ログを残して無視 (将来の拡張に強い、後方互換性を保ちやすい)

C) **lax だが lint 用に validator API を別に提供** — Parse 自体は lax、`topology.Lint(s)` で未知キー警告を取得 (推奨、両立)

X) Other (please describe after [Answer]: tag below)

[Answer]: C

---

### Question 3: Default 値の自動補完

YAML で省略可能なフィールド (例: `replicas`, `latency`, `error_rate`, `retries`) のデフォルト値は誰が設定?

A) **Parse 直後にデフォルトを書き込み** — `Schema` 内の値は常に explicit (Parse 後の Schema を見れば最終値がわかる、推奨)

B) **Zero value のまま** — 利用側が必要なときにデフォルトを適用 (`replicas=0` 等が許容され、解釈に文脈依存)

C) **Default 構造体を別に持つ** — Parse 結果は原値、`schema.ApplyDefaults()` を別 API として提供

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 4: Topological 順序の保持 (Parse 出力)

`Schema.Services` は map なので順序を持ちません。出力 (MarshalYAML / JSON Schema / デバッグ表示) で順序が必要になります。

A) **Marshal 時にキー名のアルファベット昇順** — 決定論的、PBT round-trip に有利 (推奨)

B) **元の YAML の登場順を保持** — `Schema` に `serviceOrder []ServiceID` の補助スライスを追加

C) **順序保持なし** — 出力は非決定論的でも OK (PBT round-trip は Equal が順不同で比較)

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 5: `Equal` の比較規則

`topology.Equal(a, b *Schema) bool` の正確な意味:

A) **identifier-based 厳密** — 全フィールドの値を識別子経由で比較。Service/Operation の `Calls` スライスは順序維持必須。`Operations` map は同じ ServiceID で同じ Operation 集合 (順不同 OK) (推奨、PBT round-trip に必要十分)

B) **構造的厳密** — `Calls` も `Operations` も順序維持を要求 (Marshal 安定性に依存)

C) **lenient** — 同じ業務的意味を持つなら true (例: 並列ブロックを Sequential に展開して同じなら等価) — オーバーキル

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 6: Validate の責務範囲

`Validate(s *Schema) error` が検証する事項は?

A) **構造的のみ** — R-STR-1〜8 (back-pointer, DAG, 参照解決, journey 到達可能性, fault target 実在, CallNode variant, RecoveryPolicy fallback ownership)

B) A + **ドメイン妥当性** — `error_rate ∈ [0,1]`、`p95 ≥ p50`、`replicas ≥ 1` 等の数値レンジまで (推奨、Schema を信頼できる状態に)

C) A + B + **業務的妥当性** — 例えば `database` kind のサービスは outgoing calls を持つべきでない、`cache` kind は GET 系の operation のみ、等

X) Other (please describe after [Answer]: tag below)

[Answer]: B

---

### Question 7: ApplyFaults / FaultOverlay の内部表現

`FaultOverlay` は実行時に Journey Engine が「このノード / このエッジに fault があるか」を高速に lookup するインデックス。

A) **`map[*Service]NodeFault` + `map[*Edge]EdgeFault` + `map[*Operation]OperationFault`** — ポインタをキーにした O(1) lookup (推奨、`Parse` で確定したポインタを再利用)

B) **ServiceID / 文字列キー** — `map[ServiceID]NodeFault` + `map[edgeID]EdgeFault` (`edgeID` は `<from>-><to>` の文字列など) — シリアライズ容易だが lookup ごとに ID 構築コストあり

C) **線形リスト** — `[]FaultSpec` のまま、実行時に毎回スキャン (小規模なら問題ない)

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 8: カスケード障害の事前計算

Application Design Q8 では「カスケードは pre-compute せず実行時に Edge.OnFailure の枯渇で判定」と確定済み。FaultOverlay 構築時に何を pre-compute するか?

A) **何も pre-compute しない** — Overlay は単純な lookup 辞書のみ。Journey Engine が実行時にすべての cascade 判定 (推奨、Application Design 方針と整合)

B) **「絶対に失敗する」エッジ集合だけ pre-compute** — `severity.probability=1.0` の disconnect/crash を持つ edge だけは静的に判定可能、それを Overlay に持つ

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 9: JSON Schema エクスポートの仕様

`Schema.ExportJSONSchema() ([]byte, error)` の出力形式:

A) **JSON Schema Draft 2020-12** — 最新標準、IDE エディタ補完サポートが広い (推奨)

B) **JSON Schema Draft 07** — 旧版だがエディタサポートも広い、互換性重視

C) **両方** — ファイル分割して両方提供

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 10: ParseFile / Parse のストリーミング

`Parse(r io.Reader)` は io.Reader を受けます。大きな YAML (例: 数 MB) を扱うシナリオは?

A) **想定しない** — トポロジー YAML はせいぜい数十 KB、`io.ReadAll` してから処理 (推奨、シンプル)

B) **ストリーミング decoder** — `yaml.NewDecoder(r).Decode(&raw)` を使ってチャンクごとに処理 (現実的メリット低い、yaml.v3 はストリーミングと言ってもまず全部読む)

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 11: パフォーマンス目標

`Parse` 関数の性能目標は?

A) **典型的 YAML (10 services, 30 operations, 50 edges) を 10 ms 以下** で完了 (推奨、k6 init time に影響しない)

B) **大規模 YAML (100 services, 500 operations) でも 100 ms 以下** — 余裕を持つ

C) **目標値なし** — 動けば良い

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 12: テスタブル特性 (PBT-01) — 追加候補

Application Design `application-design.md` §6 で C1 の testable properties を 5 件挙げました。本 FD で追加すべき特性 (PBT-02〜PBT-04 の適用範囲) はどこまで?

A) **Application Design 提示の 5 件をそのまま採用**
- Round-trip PBT-02: `Equal(Parse(Marshal(s)), s)` for ValidSchema
- Parse 後の全 *Service / *Edge ポインタは non-nil
- `Schema.Services[svc.Name] == svc` の整合性
- DAG 性 (Validate 後)
- ApplyFaults の overlay と元 Schema の整合性

B) A に加えて以下を追加:
- Idempotency PBT-04: `Validate(s)` を 2 回呼んで同じ結果
- Idempotency PBT-04: `ApplyFaults(s)` を 2 回呼んで同じ Overlay
- Round-trip: JSON Schema export → ファイルとして書き出し → JSON Schema validator (外部) で valid を確認 (integration 寄り)

C) B に加えて invariant 性質:
- `Validate` が success を返す Schema を任意に変更しても、ある invariant は保たれる (例: services のあるサービス名を変更しても、参照を更新すれば再び valid に)
- Path independence: A と B が同じ FaultSpec 集合を持つなら `ApplyFaults` 結果も同じ

X) Other (please describe after [Answer]: tag below)

[Answer]: B

---

### Question 13: U7 への generator 追加リクエスト

U1 の Functional Design で必要となる U7 ジェネレータ (Q8 of U7 FD の incremental 追加プロセス) はどこまで?

A) **U1 で実際に使うものだけ** — `ValidOperation`, `ValidEdge`, `ValidCallNode`, `ValidRecoveryPolicy`, `ValidJourney`, `ValidStep`, `ValidFaultSpec`, `ValidFaultTarget`, `ValidFaultOverlay` の 9 件 + 対応する Any 系 (合計 ~18 件) (推奨)

B) A に加えて U1 のエラー型ジェネレータも — `ValidParseError`, `ValidValidationError` 等

C) 後で必要になったら追加 — U1 FD では generator 追加要求はせず、Code Generation 段階で発覚次第追加

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 3 つの設計アーティファクトを生成して承認ゲートへ進みます。
