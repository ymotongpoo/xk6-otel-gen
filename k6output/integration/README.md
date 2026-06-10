# k6output Integration Test

This integration test builds a local k6 binary with xk6, starts an OpenTelemetry
Collector with Docker Compose, runs a k6 script using `--out otel-gen=...`, and
asserts that native k6 metrics are exported with
`service.name=xk6-otel-gen-runner`.

Requirements:

- Docker with Compose support
- `xk6` available on `PATH`

Run:

```bash
go test -tags=integration ./k6output/integration/...
```
