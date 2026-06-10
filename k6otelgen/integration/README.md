# k6otelgen Integration Test

This package verifies the k6 JS module with a custom xk6-built binary and an
OpenTelemetry Collector.

Requirements:

- Docker with `docker compose`
- `xk6` on `PATH`
- U6 `k6output/` implementation for `--out otel-gen=...`

Run:

```bash
go test -tags=integration ./k6otelgen/integration/...
```

The test starts `otel/opentelemetry-collector-contrib`, builds a temporary k6
binary with this module, runs `testdata/script.js`, and checks the Collector
file exporter output under `testdata/otel-logs/`.
