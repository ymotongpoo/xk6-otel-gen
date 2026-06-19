---
title: CLI ツール
weight: 0
---

トポロジ YAML ファイルと連携する補助 CLI ツールです。いずれも `go build` でビルドでき、
実行時に外部依存はありません。

## xk6-otel-gen-viz

トポロジ DAG の自己完結型インタラクティブ HTML 可視化を生成します。
出力ファイルはオフラインで動作します。JavaScript ライブラリ（Cytoscape.js、dagre）は
すべてインラインで埋め込まれています。

### 使い方

```bash
# ツールのビルド
go build ./cmd/xk6-otel-gen-viz/...

# HTML をファイルに出力（推奨 — 出力は約 700 KB）
go run ./cmd/xk6-otel-gen-viz -input topology.yaml -output topology.html

# 標準出力に書き出す場合
go run ./cmd/xk6-otel-gen-viz -input topology.yaml > topology.html
```

### フラグ

| フラグ | デフォルト | 説明 |
|--------|-----------|------|
| `-input` | *（必須）* | トポロジ YAML ファイルのパス |
| `-output` | 標準出力 | 出力先 HTML ファイルのパス |

### 可視化の内容

生成された HTML ページには以下が表示されます。

- **サービス DAG** — ノードが階層的に配置されます（dagre レイアウト、上から下）。
  ノードの形状と色はサービスの種類を示します。

  | 種類 | 色 | 形状 |
  |------|-----|------|
  | `application` | 青 | 角丸長方形 |
  | `database` | 琥珀 | 樽型 |
  | `cache` | 緑 | ダイヤモンド |
  | `queue` | 紫 | 五角形 |
  | `external_api` | 赤 | 八角形 |

- **エッジのスタイル** — 線の種類がプロトコルを示します。

  | プロトコル | 線種 |
  |-----------|------|
  | `http` | 実線 |
  | `grpc` | 破線 |
  | `messaging` | 点線 |

### インタラクティブ機能

**ジャーニートグル**（左サイドバー）— ジャーニー名をクリックすると、そのジャーニーが
到達するサービスとエッジがハイライトされます。到達しない要素は薄く表示されます。
トラフィック配分がパーセンテージで表示されます。「All」をクリックすると全体表示に
戻ります。

**障害オーバーレイ**（右サイドバー）— 障害をチェックすると、ターゲットのノードまたは
エッジが赤くマークされます。スパークラインで障害の強度スケジュールが表示されます。
複数の障害を同時に有効にできます。

**ツールチップ** — ノードにホバーすると、サービスの種類・言語・フレームワーク・
バージョン・レプリカ数・オペレーション一覧が表示されます。エッジにホバーすると、
呼び出し元→呼び出し先のオペレーション・プロトコル・レイテンシ（p50 / p95）・
エラーレート・リトライ回数が表示されます。

**検索** — 検索ボックスに入力すると、名前がマッチするノードがハイライトされます。

**ズーム / パン** — スクロールでズーム、ドラッグでパン（Cytoscape.js 組み込み機能）。

### 使用例

```bash
# astroshop の例を可視化（23 サービス、5 ジャーニー、4 障害）
go run ./cmd/xk6-otel-gen-viz \
  -input examples/astroshop/topology.yaml \
  -output astroshop.html
```

ブラウザで `astroshop.html` を開きます。「place-order」ジャーニーを選択すると、
チェックアウトの完全なパス（frontend → checkout → payment / shipping / email / …）が
表示されます。次に「error\_rate\_override」障害を有効にすると、影響を受けるノードが
確認できます。

---

## xk6-otel-gen-schema

エディタ補完と CI バリデーション用にトポロジの JSON Schema を出力します。

### 使い方

```bash
# 標準出力に書き出す
go run ./cmd/xk6-otel-gen-schema > topology.schema.json

# ファイルに書き出す
go run ./cmd/xk6-otel-gen-schema -output topology.schema.json
```

### フラグ

| フラグ | デフォルト | 説明 |
|--------|-----------|------|
| `-output` | 標準出力 | 出力先ファイルのパス |

生成した Schema をエディタに設定すると、トポロジファイルの YAML 自動補完と
インライン検証が利用できます。
