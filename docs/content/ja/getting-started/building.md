---
title: ビルド
weight: 2
---

xk6 をインストールし、カスタム k6 バイナリをビルドします。

```bash
go install go.k6.io/xk6/cmd/xk6@latest
xk6 build --with github.com/ymotongpoo/xk6-otel-gen
```

ローカル開発では、xk6 にこのチェックアウトを指定します。

```bash
xk6 build --with github.com/ymotongpoo/xk6-otel-gen=.
./k6 version
```

| ビルド方法 | コマンド |
|---|---|
| リモートモジュール | `xk6 build --with github.com/ymotongpoo/xk6-otel-gen` |
| ローカルチェックアウト | `xk6 build --with github.com/ymotongpoo/xk6-otel-gen=.` |
