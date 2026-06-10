# Journey Integration Tests

These tests require Docker with Compose support. They start an OpenTelemetry
Collector from `journey/testdata/docker-compose.yaml`, run the real exporter
pipeline and synthesizer, then read Collector file-exporter output.

Run:

```bash
go test -tags=integration ./journey/integration/...
```
