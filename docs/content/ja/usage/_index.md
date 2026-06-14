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

完全なスクリプトは
[minimal](https://github.com/ymotongpoo/xk6-otel-gen/tree/main/examples/minimal) と
[astroshop](https://github.com/ymotongpoo/xk6-otel-gen/tree/main/examples/astroshop)
の例を参照してください。
