// Package k6output registers the "otel-gen" k6 output extension.
//
// Use it from k6 as an output:
//
//	k6 run --out otel-gen=endpoint=localhost:4317,protocol=grpc,insecure=true script.js
//
// The output has two responsibilities. First, it participates in the shared
// xk6-otel-gen exporter lifecycle and calls Pipeline.Shutdown when k6 stops.
// This flushes telemetry emitted by the JavaScript module path as well as
// native k6 runner metrics. Second, it converts native k6 samples such as
// http_req_duration, iterations, checks, data_sent, and vus into OpenTelemetry
// metrics under the k6.* namespace.
//
// k6 runner metrics use a dedicated Resource with
// service.name="xk6-otel-gen-runner". This keeps real k6 execution metrics
// separate from synthesized service telemetry emitted by other packages.
//
// Supported --out args are comma-separated key=value pairs:
//
//	Key           Value                         Default
//	endpoint      host:port or scheme://host     localhost:4317
//	protocol      grpc or http                   grpc
//	insecure      true or false                  false
//	headers       key1:val1;key2:val2            none
//	compression   gzip or empty                  empty
//	timeout       Go duration, for example 10s   10s
//	batchSize     integer                        512
//	batchTimeout  Go duration                    1s
//	maxQueueSize  integer                        2048
//	queueSize     integer in [10, 10000]         100
//
// Configuration priority, from highest to lowest, is: JavaScript
// otelgen.configure options, --out otel-gen args, OTEL_EXPORTER_OTLP_* env
// vars, then built-in defaults.
//
// k6 tags are emitted as metric attributes with a k6.tag. prefix. High
// cardinality tag values, especially URL-like name tags, can create expensive
// time series in downstream backends; configure k6 tags and Collector
// processors accordingly.
package k6output
