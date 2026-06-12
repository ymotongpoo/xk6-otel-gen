---
name: build-go125-buildvcs
description: Go 1.25+ requires -buildvcs=false when building k6 via xk6 in this repo
metadata:
  type: project
---

Go 1.25 以降、このリポジトリで `xk6 build`(内部で `go build`)を実行すると VCS スタンプ処理で失敗する。`-buildvcs=false` を付けてビルドすること。

**How to apply:** xk6 は `go build` を呼ぶので `GOFLAGS=-buildvcs=false` を環境変数で渡すのが簡単:

```
GOFLAGS=-buildvcs=false xk6 build --with github.com/ymotongpoo/xk6-otel-gen=. --output ./k6
```

直接 `go build` する場合も同様に `-buildvcs=false` を付ける。
