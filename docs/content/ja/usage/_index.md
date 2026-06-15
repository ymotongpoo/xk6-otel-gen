---
title: 使い方
weight: 2
---

JS モジュールをインポートし、OTLP を設定し、トポロジを読み込んでジャーニーを実行します。

```javascript
import otelgen from "k6/x/otel-gen";

export function setup() {
  otelgen.configure({
    endpoint: "localhost:4317",
    protocol: "grpc",
    insecure: true,
  });
}

export default function () {
  const topology = otelgen.load("./topology.yaml");
  topology.runRandomJourney();
}

export function teardown() {
  otelgen.flush();
}
```

`load()` は `setup()` ではなく `default()` の中で呼び出してください。k6 は `setup()` の
戻り値を JSON シリアライズするため、ハンドルのメソッドが失われてしまいます。`load()` は
テスト実行ごとに 1 度だけ YAML をパース・検証し、以降の呼び出しではキャッシュした
ハンドルを返すため、イテレーションごとに呼んでもオーバーヘッドはありません。

`otelgen.flush()` は `teardown()` で呼び出してください。各トレースのルートスパンは
すべての子スパンの後に終了するため、バッチキューに最後に入ります。最後のフラッシュが
ないとプロセス終了時に破棄され、バックエンドは「root span not yet received」と報告
します。`flush()` はトレース・メトリクス・ログの送信を `otel-gen` 出力の有効・無効に
依存しない形で行います。エクスポーターを閉じずにバッチプロセッサーを強制フラッシュする
ため、`--out otel-gen=...` の有無にかかわらず安全に呼び出せます(出力が有効な場合は、
その `Stop` フックが最終的なパイプラインのシャットダウンを実施します)。

| API | 用途 |
|---|---|
| `otelgen.configure(opts)` | OTLP エンドポイント、プロトコル、TLS、ヘッダー、バッチを設定 |
| `otelgen.load(path)` | 1 つのトポロジ YAML ファイルをパース・検証 |
| `handle.runJourney(name)` | 名前付きジャーニーを実行 |
| `handle.runRandomJourney()` | YAML の weight に従ってジャーニーを選んで実行し、その名前を返す |
| `handle.journeyWeights()` | カスタム JS 選択用に `{ name: weight }` を返す |
| `otelgen.flush()` | キュー済みテレメトリを強制フラッシュ(`teardown()` で呼びルートスパンを確実に送信) |
| `otelgen.stats()` | エクスポーターの成功/失敗カウンタを返す |
| `otelgen.journeys()` | 読み込み後にジャーニー名の一覧を返す |
| `handle.journeys()` | ハンドルからジャーニー名の一覧を返す |

## シグナルと機能

ジャーニーの実行ごとに、トレースコンテキストを共有する相関した OpenTelemetry シグナルが
生成されます。

- **トレース** — ジャーニーごとに 1 本、オペレーションと呼び出しごとにスパン。`messaging`
  エッジはさらに PRODUCER（publish）と CONSUMER（receive）のスパンを出し、スパンリンクで
  連結します。
- **メトリクス** — 組み込みのリクエスト/所要時間インストルメントに加え、オペレーション単位の
  カスタムメトリクス（counter / gauge / histogram）。ヒストグラムにはエグゼンプラー
  （`trace_id` / `span_id`）が付き、メトリクス→トレースのドリルダウンができます。
- **ログ** — オペレーション単位のログに加え、`event.name` を持つ宣言的な構造化ログイベント。
- **プロファイル** — [`profilesEndpoint`]({{< relref "/reference/configuration" >}}) を設定すると
  合成 pprof フレームグラフを Pyroscope へ送ります。

これらはすべてトポロジから駆動されます。各オペレーションは次を宣言できます。

| フィールド | 出力 |
|---|---|
| `log_events` | 構造化ログ（name、severity、condition、body、attributes） |
| `metrics` | カスタム counter / gauge / histogram（任意で fault 連動） |
| `profile` | diff プロファイリング用の baseline / incident フレームグラフ（fault 連動） |

完全な構文は [トポロジ YAML リファレンス]({{< relref "/reference/topology" >}}) を参照してください。
カスタムメトリクスとプロファイルは active な fault に反応できるため、インシデント時に出力値や
スタックが決定的に変化します。

完全なスクリプトは
[minimal](https://github.com/ymotongpoo/xk6-otel-gen/tree/main/examples/minimal) と
[astroshop](https://github.com/ymotongpoo/xk6-otel-gen/tree/main/examples/astroshop)
の例を参照してください。
