# Exporter Integration Tests

Run with:

```bash
go test -tags=integration ./exporter/integration/...
```

These tests require Docker with Compose support. They are skipped by default because the `integration` build tag is required.
