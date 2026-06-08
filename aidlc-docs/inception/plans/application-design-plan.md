# Application Design Plan — xk6-otel-gen

## 目的

`requirements.md` で定義した機能要件を、6 つの暫定ユニットに対応するアプリケーション設計に落とし込むため、コンポーネント境界・メソッドシグネチャ・サービスオーケストレーション・依存関係の意思決定を確定する。

## アーティファクト (承認後に Claude が生成)

- [ ] `aidlc-docs/inception/application-design/components.md` — 6 ユニットに対応する Go パッケージ単位のコンポーネント定義・責務・公開インターフェース
- [ ] `aidlc-docs/inception/application-design/component-methods.md` — 各コンポーネントの公開メソッドシグネチャ (詳細業務ロジックは Functional Design で扱う)
- [ ] `aidlc-docs/inception/application-design/services.md` — サービスレイヤの定義とオーケストレーション (k6 起動から OTLP 送信までのデータフロー)
- [ ] `aidlc-docs/inception/application-design/component-dependency.md` — 依存マトリクスと通信パターン (`mermaid` 依存図含む)
- [ ] `aidlc-docs/inception/application-design/application-design.md` — 上記 4 ドキュメントを統合したマスターサマリ
- [ ] 設計の整合性検証 (requirements との traceability、PBT-01 で要求される "Testable Properties" 識別の準備)

---

## 設計確定のための質問

以下の `[Answer]:` タグに回答してください。複数選択可の場合はその旨を明記しています。

### Question 1: コンポーネント境界の粒度

Workflow Planning で提示した 6 つの暫定ユニットを、そのままアプリケーション設計上の **Go パッケージ単位のコンポーネント** とする方針で良いですか?

A) そのまま採用 — `internal/topology`, `internal/journey`, `internal/synth`, `internal/exporter`, `pkg/k6otelgen` (JS module), `pkg/k6output` (output module) + `examples/` をユニットとする (推奨)

B) Topology Schema & Parser と Topology Model & Journey Engine を 1 ユニットに統合 — グラフモデルとパーサは密接なため

C) Signal Synthesizer を Trace / Metric / Log の 3 サブコンポーネントに分割 — 信号ごとに責務が異なる

D) もっと粒度を細かく分けたい (詳細は Other に記述)

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 2: YAML スキーマ設計の哲学

トポロジー YAML のスキーマ設計方針はどれが好みですか?

A) シンプル単一ファイル方式 — 1 つの YAML にすべて記述、`!include` などの分割は無し (PoC / 教材としてシンプル)

B) 単一ファイル + 上書きレイヤー — ベースの YAML を読み込んだあと、CLI/JS 側で部分上書き可能 (例: 環境ごとに `error_rate` だけ変える)

C) 分割可能 — `!include` や `extends:` のような機構で複数 YAML を組み合わせ可能 (大規模トポロジーに対応)

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 3: k6 Output モジュール (`--out otel-gen=...`) の役割

要件 FR-1.2 で「JS モジュール + 出力モジュール両方を提供」と決めましたが、**Output モジュールの責務** は具体的にどれですか?

A) **k6 自身の負荷テスト結果** (http_req_duration, vus, iterations 等の k6 ネイティブメトリクス) を OTLP/Metrics として外部に送出する。**合成シグナルとは独立した別系統**

B) **合成シグナルの egress** として機能 — JS モジュールが内部で組み立てた span/metric/log を Output モジュールが受け取って OTLP で送信。JS モジュールは API のみで、ネットワーク I/O は Output モジュールに集約

C) 両方 — Output モジュールは k6 ネイティブメトリクスを送出しつつ、JS モジュール経由で組み立てた合成シグナルの egress も担当 (内部的に統合された送信パイプライン)

X) Other (please describe after [Answer]: tag below)

[Answer]: C - 合成シグナルのegressは本来の目的からして必要です。負荷テスト結果は、テレメトリー生成は長期間（最低でも10分とか）動かし続けないといけないので、k6の動作状況を理解するために必要です。

---

### Question 4: JS API の使用感

要件 FR-8.1 で示した API の最小形:

```javascript
import otelgen from 'k6/x/otel-gen';
const topology = otelgen.load('./topology.yaml');
export default function () {
  topology.runJourney('checkout');
}
```

この方針を維持しますか? それとも以下の派生案が好みですか?

A) 上記そのまま — `runJourney(name)` が iteration ごとに 1 ジャーニーを駆動。最も宣言的 (推奨、Q11 の答えと一致)

B) ジャーニー選択を k6 シナリオ側に任せる — `runJourney('checkout')` のように呼び元で固定。複数ジャーニーは k6 シナリオを複数用意

C) JS から個別 span / metric を直接 emit する低レベル API も併設 — `topology.span('checkout-service', 'PlaceOrder', {...})` 等。研究・実験用途向け

D) すべて — A の高レベル API + C の低レベル API を両方提供

X) Other (please describe after [Answer]: tag below)

[Answer]: A - ただジャーニーの定義をどこで書いておくかがよくわからない

---

### Question 5: OpenTelemetry Go SDK の使用方針

OTLP 送信パイプラインの実装方針はどれが好みですか?

A) **OTel Go SDK をフル活用** — `go.opentelemetry.io/otel/sdk/trace`、`metric/sdk`、`log/sdk` の BatchProcessor / Exporter をそのまま使う。実装労力は最小、互換性も最高 (推奨)

B) **OTel Proto + 自前の薄いエクスポータ** — `go.opentelemetry.io/proto/otlp` の構造体を直接組み立て、自前のバッチ送信ロジック。1k RPS 性能で SDK のオーバーヘッドが懸念される場合の選択肢

C) **ハイブリッド** — Traces は SDK、Metrics/Logs は自前 (または逆)。信号ごとに方針を変える

X) Other (please describe after [Answer]: tag below)

[Answer]: A - 全部 OpenTelemetry Go SDK に依存します。

---

### Question 6: 時間シミュレーションのモデル

トポロジーで宣言されたレイテンシ (例: `p50: 30ms, p95: 200ms`) を **実時間** で待つか、**仮想時間** で span のタイムスタンプを書き換えるかは設計上の大きな分岐です。

A) **実時間 (real-time)** — `runJourney()` 内で実際に `time.Sleep` し、各 span の `EndTime` は wall-clock。k6 シナリオの RPS と実レイテンシは整合する (例: p95=200ms のジャーニーは 1 iteration あたり最低 200ms かかる)

B) **仮想時間 (virtual / synthesized)** — `runJourney()` はほぼ即座に返り、span のタイムスタンプは合成計算。k6 の throughput は実レイテンシに依存しない (1k RPS = 1k journey/s が常に達成可能、backend では時間軸が現実的に見える)

C) **ハイブリッド (selectable)** — YAML またはオプションで切替可能。デフォルトは A

X) Other (please describe after [Answer]: tag below)

[Answer]: A - 大抵の場合受け付けるバックエンド側が過去のテレメトリーの再生機能を持っているわけではないので、まずはMVPとして実時間のみで良い。今後データ分析用の過去のテレメトリーシグナルを生成するような機能を作るなら、仮想時間に対応する必要があるが、それはTODOとして積んでおいて良い。

---

### Question 7: ジャーニー内の並行性モデル

1 つのジャーニー (例: `checkout`) が複数サービスを通過するとき、サービス間の処理は内部的にどう実行しますか?

A) **逐次** — 1 ジャーニーは 1 つの goroutine で順序通りに各サービスを処理 (最もシンプル。span の parent/child 関係も自然)

B) **部分並行** — 兄弟 span (fan-out 呼び出し、例: チェックアウトが在庫サービスと決済サービスを並列に呼ぶ) は別 goroutine で並行実行。トポロジー YAML で `parallel: true` を指定可能にする

C) **常に並行** — すべての依存呼び出しを goroutine で起動し、必要に応じて join

X) Other (please describe after [Answer]: tag below)

[Answer]: X - 基本的に１ジャーニーは1つのgoroutineで実施するほうがユーザーの振る舞いに近いのでそうしたい。ただ、たとえばあるサービス内で並行してリクエストを投げるようなシナリオもあると思うので、そのような状況には対応してほしい。

---

### Question 8: 失敗注入 (Failure Injection) の責務配置

エラー率・タイムアウト・カスケード障害などの失敗注入ロジックは、どのコンポーネントが担当しますか?

A) **Topology Model 内に集約** — YAML 読み込み時に失敗注入ルールがグラフのエッジに組み込まれ、Journey Engine がそれを参照しながらシミュレートする (失敗ロジックの中央集権)

B) **Signal Synthesizer 内に分散** — 各サービスの span を合成するロジックの中に失敗注入が組み込まれる (テレメトリ生成と失敗ロジックを密結合)

C) **独立した Failure Injector コンポーネント** — `internal/failure/` を別パッケージとし、Journey Engine と Signal Synthesizer から呼ばれる (関心分離)

X) Other (please describe after [Answer]: tag below)

[Answer]: A - トポロジーの定義とは別に、障害がトポロジー内のどのノードあるいはエッジでどのような問題が起きるか（ノード内ならレイテンシーの増加、エッジなら切断など）を定義できるようにしたい。また1つの定義でカスケード障害も自動で発生できると嬉しい（たとえばエッジの切断によって、下流へリクエストが流れなくなることによる障害など）。

---

### Question 9: 設定の優先順位

OTLP エンドポイント、ヘッダー、リソース属性などの設定が **JS API オプション**、**環境変数** (`OTEL_EXPORTER_OTLP_*`)、**YAML defaults** の3箇所で指定可能なとき、優先順位はどうしますか?

A) **JS API > 環境変数 > YAML defaults > ハードコードデフォルト** (推奨。OTel SDK の慣例とも一致)

B) **環境変数 > JS API > YAML defaults** — CI/CD での上書きを優先したい場合

C) **YAML > JS API > 環境変数** — トポロジー YAML を完全な真の SSOT にしたい場合

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

### Question 10: 配布モデル

xk6 でビルドした拡張入り k6 バイナリの配布形態はどれですか?

A) **ソース + ビルド手順のみ** — README に `xk6 build` コマンドを記載、利用者が自分でビルド (OSS k6 拡張の最も標準的なスタイル)

B) **GitHub Releases でプリビルドバイナリ提供** — Linux/macOS × amd64/arm64 のバイナリを Release Action で自動公開、利用者は curl/wget で取得可能 (より親切。GoReleaser で実現可能)

C) **両方** — Source + GitHub Releases、加えて Docker image (`ghcr.io/ymotongpoo/xk6-otel-gen:latest`) も提供

X) Other (please describe after [Answer]: tag below)

[Answer]: B - ただし昨今起きているリポジトリのCIを乗っ取ってマルウェア化するインシデントの多さを鑑みて、バイナリはあくまで補助であり、自分でビルドすることを推奨するとREADMEに明記。

---

### Question 11: モジュールパス

Go モジュールパスは何にしますか? (`go.mod` の `module` 行)

A) `github.com/ymotongpoo/xk6-otel-gen` (推奨、ユーザーの GitHub アカウント想定)

B) `github.com/<別のオーガニゼーション>/xk6-otel-gen` (Grafana 等の org に置く場合)

X) Other (please describe after [Answer]: tag below)

[Answer]: A

---

## 回答後の流れ

すべての `[Answer]:` を埋めて「完了しました」「done」とお伝えください。回答を分析し、矛盾・曖昧があれば追加質問、なければ 5 つの設計アーティファクトを生成して承認ゲートへ進みます。
