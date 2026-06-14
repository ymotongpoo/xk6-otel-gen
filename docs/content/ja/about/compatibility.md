---
title: 互換性
weight: 3
---

| ツール | 最小バージョン | 用途 |
|---|---:|---|
| Go | 1.25+ | モジュールのビルドとテスト |
| xk6 | latest | カスタム k6 バイナリのビルド |
| k6 | xk6 でビルド | 負荷試験のランタイム |
| kubectl | 1.27+ | マニフェストの適用と確認 |
| kind | 0.20+ | ローカル Kubernetes クラスター |
| Docker | 最新の安定版 | kind ノードのランタイム |

ローカルのバージョンを確認します。

```bash
go version
xk6 version
kubectl version --client
kind version
docker version
```

## ライセンス

`xk6-otel-gen` は Apache-2.0 でライセンスされています。

```text
SPDX-License-Identifier: Apache-2.0
```

| ファイル | 用途 |
|---|---|
| [LICENSE](https://github.com/ymotongpoo/xk6-otel-gen/blob/main/LICENSE) | Apache License 2.0 の全文 |
| `.go` ファイル | lint で強制される SPDX ヘッダー |
