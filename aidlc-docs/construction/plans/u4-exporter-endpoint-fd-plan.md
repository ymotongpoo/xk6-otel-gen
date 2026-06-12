# U4 Exporter — Functional Design Plan: Per-Signal Endpoint Support

**Change Request**: Per-Signal Endpoint Support(2026-06-12)
**Requirements**: `aidlc-docs/inception/requirements/endpoint-config-requirements.md`
**Scope**: U4 `exporter/` のエンドポイント解決ロジックのみ(U5/U6/U8 は Code Generation で対応)

## Plan Steps

- [x] Step 1: ドメインモデル更新 — `exporter.Config` へのシグナル別エンドポイントフィールド追加を
      既存 `domain-entities.md` に追記(§9)
- [x] Step 2: ビジネスロジックモデル — OTLP 仕様準拠のエンドポイント解決アルゴリズム
      (ベースパス補完 / per-signal as-is / 優先順位)を `business-logic-model.md` に追記(§9)
- [x] Step 3: ビジネスルール — バリデーション・優先順位・環境変数適用規則・テスト可能プロパティ
      (PBT)を `business-rules.md` に追記(§11、TP-U4-5〜7)
- [x] Step 4: 完了メッセージ提示 & 承認待ち

## Design Questions

以下の質問に `[Answer]:` タグで回答してください(要件で未確定の設計詳細のみ)。

### Question 1: Config 内部表現
`exporter.Config` でのシグナル別エンドポイントの持ち方はどうしますか?
(JS 側はフラットキーで確定済み。これは Go 構造体の内部設計です)

A) フラットフィールド: `TracesEndpoint` / `MetricsEndpoint` / `LogsEndpoint` string を追加
   (既存 Config のフラット構造・MergeWith / fillDefaults の単純な拡張で済む)

B) ネスト構造体: `SignalEndpoints struct { Traces, Metrics, Logs string }` フィールドを追加

C) Other (please describe after [Answer]: tag below)

[Answer]: A

### Question 2: gRPC でのシグナル別エンドポイント形式
gRPC プロトコル時のシグナル別エンドポイントはどの形式を受け付けますか?

A) ベース `endpoint` と同じ両形式: `host:port` および `scheme://host[:port]`
   (既存バリデーション validEndpoint を再利用。URL 形式は WithEndpointURL、host:port は WithEndpoint)

B) `host:port` のみ(gRPC にパス概念がないため URL 形式は拒否)

C) Other (please describe after [Answer]: tag below)

[Answer]: A

### Question 3: ベース URL に query string がある場合の扱い
OTLP 仕様はベースパスへの `v1/{signal}` 追記のみ規定しています。
`https://host/otlp?key=value` のような query 付きベース URL はどう扱いますか?

A) query / fragment を保持し、パス部分のみに `v1/{signal}` を追記する
   (OTel Go SDK の環境変数処理と同等の挙動)

B) query 付きベース URL はバリデーションエラーとして拒否する

C) Other (please describe after [Answer]: tag below)

[Answer]: A

### Question 4: 解決後エンドポイントのログ出力(NFR-4 の具体化)
起動時ログへの反映方法はどうしますか?

A) 既存の `exporter configured` INFO ログに解決後の 3 シグナル分の送信先 URL を
   フィールド(traces= / metrics= / logs=)として追加する(1 行で完結)

B) シグナルごとに 1 行ずつ INFO ログを出す(3 行)

C) Other (please describe after [Answer]: tag below)

[Answer]: A
