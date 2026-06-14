---
title: Building
weight: 2
---

Install xk6 and build a custom k6 binary:

```bash
go install go.k6.io/xk6/cmd/xk6@latest
xk6 build --with github.com/ymotongpoo/xk6-otel-gen
```

For local development, point xk6 at this checkout:

```bash
xk6 build --with github.com/ymotongpoo/xk6-otel-gen=.
./k6 version
```

| Build mode | Command |
|---|---|
| Remote module | `xk6 build --with github.com/ymotongpoo/xk6-otel-gen` |
| Local checkout | `xk6 build --with github.com/ymotongpoo/xk6-otel-gen=.` |
