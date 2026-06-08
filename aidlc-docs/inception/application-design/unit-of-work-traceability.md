# Unit of Work Traceability — xk6-otel-gen

User Stories はスキップ (Workflow Planning の決定) のため、本書は `requirements.md` の **FR (機能要件) / NFR (非機能要件) とユニットの追跡マッピング** を提供します (Q8=A、Application Design §4 表を起点)。

各 FR/NFR が **どのユニットで実装されるか** を一覧化することで:
- すべての要件が少なくとも 1 つのユニットに割り当てられている (網羅性確認)
- すべてのユニットが少なくとも 1 つの要件を支えている (無駄なユニットがない)
- Construction フェーズで「ユニット完成時にどの要件が満たされたか」をレビューできる

凡例:
- **P** (Primary) — そのユニットが主担当として実装する
- **S** (Supporting) — そのユニットも貢献するが、主担当ではない
- 空欄 — 当該ユニットは関与しない

---

## FR (機能要件) → ユニット

| 要件 ID | 概要 | U1 topology | U2 journey | U3 synth | U4 exporter | U5 k6otelgen | U6 k6output | U7 testutil | U8 dist |
|---|---|---|---|---|---|---|---|---|---|
| FR-1.1 | xk6 build でカスタム k6 バイナリ生成 | | | | | S | S | | **P** |
| FR-1.2 | JS モジュール + 出力モジュール両方を提供 | | | | | **P** | **P** | | |
| FR-1.3 | Apache-2.0 OSS、xk6 ビルド手順を README に明記 | | | | | | | | **P** |
| FR-2.1 | 独自 YAML スキーマで入力 | **P** | | | | | | | |
| FR-2.2 | YAML が表現できる要素 (service kind/replicas, edges, journeys, faults) | **P** | | | | | | | |
| FR-2.3 | YAML 厳格バリデーション | **P** | | | | | | | |
| FR-3.1 | 3 信号 (Metrics/Logs/Traces) すべて生成 | | S (起動側) | **P** | S (egress) | | | | |
| FR-4.1 | OTLP/gRPC + OTLP/HTTP の両プロトコル | | | | **P** | | | | |
| FR-4.2 | 環境変数 + JS API オプションの設定優先順位 | | | | **P** | S (JS 側受口) | S (--out 受口) | | |
| FR-4.3 | OTLP Protobuf 公式スキーマ準拠 | | | | **P** | | | | |
| FR-5.1 | E2E シミュレーション (CUJ を 1 trace で表現) | | **P** | S (span 連鎖) | | | | | |
| FR-5.2 | サーバー側視点の metrics / logs 補完 | | S | **P** | | | | | |
| FR-5.3 | 負荷プロファイルは k6 シナリオで駆動 | | | | | **P** | | | |
| FR-6.1 | Semantic Conventions の主要部分準拠 | | | **P** | S (Resource) | | | | |
| FR-6.2 | 将来の SC 拡張可能な属性マッピング層 | | | **P** | | | | | |
| FR-7.1 | 失敗シナリオ E (HTTP error / timeout / cascade / retry storm) | **P** (faults section) | **P** (実行時 cascade 判定) | S (error 属性) | | | | | |
| FR-7.2 | span に error.type / exception.* / status=ERROR | | S | **P** | | | | | |
| FR-8.1 | 宣言的 JS API (`topology.runJourney`) | | | | | **P** | | | |
| FR-8.2 | `runJourney` が trace_id 生成 + 全 signals 合成 | | **P** | S | S | **P** (起動口) | | | |
| FR-8.3 | configure(), env, YAML defaults の受け口 | | | | **P** (merge logic) | **P** (JS configure) | S (--out 解析) | | |
| FR-9.1 | minimal サンプル (3 サービス) | | | | | | | | **P** |
| FR-9.2 | realistic サンプル (OTel Demo 10+ サービス) | | | | | | | | **P** |
| FR-9.3 | サンプル毎の k6 スクリプト + Collector 起動 README | | | | | | | | **P** |

### FR の網羅検証
- すべての FR に少なくとも 1 つの Primary 割当あり ✓
- 未割当の FR: なし ✓

---

## NFR (非機能要件) → ユニット

| 要件 ID | 概要 | U1 topology | U2 journey | U3 synth | U4 exporter | U5 k6otelgen | U6 k6output | U7 testutil | U8 dist |
|---|---|---|---|---|---|---|---|---|---|
| NFR-1.1 | 1k RPS 持続 (4 vCPU / 8 GB) | | S (sleep 精度) | S (合成効率) | **P** (BatchProcessor チューニング) | S | | | |
| NFR-1.2 | k6 単体比のスループット低下 30% 以内 | | S | S | **P** | S | | | |
| NFR-1.3 | 送信は非同期バッチで iteration をブロックしない | | | | **P** | | | | |
| NFR-1.4 | バックプレッシャー時のドロップ + メモリリーク防止 | | | | **P** | | | | |
| NFR-2.1 | OTLP 未到達でも k6 ラン継続 + 送信失敗カウント | | | | **P** | | S (Stats 露出) | | |
| NFR-2.2 | 設定エラーは k6 起動時に fail fast | **P** (Parse/Validate) | | | S (Config 検証) | S (init で停止) | S | | |
| NFR-3.1 | k6 最新 stable + 直近 2 マイナー対応 | | | | | **P** | **P** | | S (build script) |
| NFR-3.2 | Go 最新 stable + 1 つ前の minor 対応 | | | | | | | | **P** (CI matrix) |
| NFR-3.3 | Linux / macOS 対応 (Windows best-effort) | | | | | | | | **P** (CI matrix) |
| NFR-4.1 | Unit + Integration テスト | S | S | S | S | S | S | S | **P** (integration harness) |
| NFR-4.2 | PBT (Full) を主要ロジックに必須適用 | **P** | **P** | **P** | **P** | S | S | **P** (generators) | |
| NFR-4.3 | CI で PBT のシード値ログ + 再現性 | | | | | | | S (rapid 設定) | **P** (CI ログ) |
| NFR-4.4 | フレームワークは `pgregory.net/rapid` | | | | | | | **P** | |
| NFR-5.1 | 拡張自身の動作ログ (k6 logger 経由) | | | | S | **P** | **P** | | |
| NFR-5.2 | 内部メトリクス (送信成功/失敗、内部キュー長) を k6 メトリクスへ | | | | **P** (Stats) | S | **P** (k6 metrics ブリッジ) | | |
| NFR-6.1 | README + JS API リファレンス + YAML スキーマリファレンス | | | | | | | | **P** |
| NFR-6.2 | JSON Schema 公開 | **P** (ExportJSONSchema) | | | | | | | **P** (`schemas/topology.schema.json`) |

### NFR の網羅検証
- すべての NFR に少なくとも 1 つの Primary 割当あり ✓
- 未割当の NFR: なし ✓

---

## PBT ルール (参考) → ユニット

Q8=A では PBT を含めない選択でしたが、各ユニットの Functional Design (PBT-01 が要求するプロパティ識別) のスタート地点として **どのユニットでどの PBT ルールが該当しそうか** の見立てを参考に記載します (確定は各 Functional Design で):

| PBT ルール | 適用見立て (Functional Design で確定) |
|---|---|
| PBT-01 (Property Identification) | 全ユニット (U7 を除く) の Functional Design で必須 |
| PBT-02 (Round-trip) | **U1** (YAML パース ↔ Marshal)、**U4** (OTLP protobuf marshal/unmarshal) |
| PBT-03 (Invariants) | **U1** (DAG/参照/journey 到達可能性)、**U2** (trace_id 一意性、parent_span_id 連鎖、recovery 不変条件)、**U3** (metric sum 保存則) |
| PBT-04 (Idempotency) | **U1** (Validate, ApplyFaults の冪等性)、**U2** (BuildPlan の冪等性)、**U4** (Config.MergeWith) |
| PBT-05 (Oracle) | N/A 想定 (参照実装が存在しない) |
| PBT-06 (Stateful) | **U4** (Pipeline 内部キュー、Stats カウンタ) のみ — Functional Design で要検討 |
| PBT-07 (Generator Quality) | **U7** が責任を持って集中管理 |
| PBT-08 (Shrinking & Reproducibility) | **U7** + 全テスト (CI ログ): NFR-4.3 と直結 |
| PBT-09 (Framework Selection) | NFR-4.4 で確定済み: `pgregory.net/rapid` |
| PBT-10 (Complementary) | 全ユニット (example-based + PBT を別ファイルに分離) |

---

## 補足: Cross-cutting 要件

以下は単一ユニットでは完結せず、複数ユニット協調で実現される要件です:

| 要件 | 関与ユニット | 協調パターン |
|---|---|---|
| FR-1.2 (デュアル機能 k6 拡張) | U5 + U6 + U4 | U4 内の shared Pipeline holder で連携 |
| NFR-1.1 (1k RPS) | U2 + U3 + U4 | sleep 精度 (U2) × 合成効率 (U3) × バッチ送信 (U4) の総合最適化 |
| NFR-4.2 (PBT Full) | 全ユニット | U7 のジェネレータを再利用しつつ、各ユニットが自分の不変条件を PBT 化 |
| FR-7.1 (cascade / recovery) | U1 + U2 | U1 の Fault Overlay → U2 の実行時条件付きカスケード判定 |
| FR-3.1 (3 信号) | U3 + U4 | U3 が SDK Provider に投入 → U4 BatchProcessor が OTLP 送信 |
