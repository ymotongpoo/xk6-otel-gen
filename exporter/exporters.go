package exporter

import (
	"context"
	"fmt"
	"strings"

	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// buildTraceExporter creates a protocol-specific OTLP trace exporter.
func buildTraceExporter(ctx context.Context, cfg Config, stats *pipelineStats) (sdktrace.SpanExporter, error) {
	var inner sdktrace.SpanExporter
	var err error
	switch cfg.Protocol {
	case ProtocolGRPC:
		opts := []otlptracegrpc.Option{
			otlptracegrpc.WithHeaders(cfg.Headers),
			otlptracegrpc.WithTimeout(cfg.Timeout),
		}
		if endpointIsURL(cfg.Endpoint) {
			opts = append(opts, otlptracegrpc.WithEndpointURL(cfg.Endpoint))
		} else {
			opts = append(opts, otlptracegrpc.WithEndpoint(cfg.Endpoint))
		}
		if cfg.Insecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}
		if cfg.Compression == "gzip" {
			opts = append(opts, otlptracegrpc.WithCompressor("gzip"))
		}
		inner, err = otlptracegrpc.New(ctx, opts...)
	case ProtocolHTTP:
		opts := []otlptracehttp.Option{
			otlptracehttp.WithHeaders(cfg.Headers),
			otlptracehttp.WithTimeout(cfg.Timeout),
		}
		if endpointIsURL(cfg.Endpoint) {
			opts = append(opts, otlptracehttp.WithEndpointURL(cfg.Endpoint))
		} else {
			opts = append(opts, otlptracehttp.WithEndpoint(cfg.Endpoint))
		}
		if cfg.Insecure {
			opts = append(opts, otlptracehttp.WithInsecure())
		}
		if cfg.Compression == "gzip" {
			opts = append(opts, otlptracehttp.WithCompression(otlptracehttp.GzipCompression))
		}
		inner, err = otlptracehttp.New(ctx, opts...)
	default:
		return nil, fmt.Errorf("unknown protocol: %v", cfg.Protocol)
	}
	if err != nil {
		return nil, err
	}
	return &tracingExporter{inner: inner, stats: stats}, nil
}

// buildMetricExporter creates a protocol-specific OTLP metric exporter.
func buildMetricExporter(ctx context.Context, cfg Config, stats *pipelineStats) (sdkmetric.Exporter, error) {
	var inner sdkmetric.Exporter
	var err error
	switch cfg.Protocol {
	case ProtocolGRPC:
		opts := []otlpmetricgrpc.Option{
			otlpmetricgrpc.WithHeaders(cfg.Headers),
			otlpmetricgrpc.WithTimeout(cfg.Timeout),
		}
		if endpointIsURL(cfg.Endpoint) {
			opts = append(opts, otlpmetricgrpc.WithEndpointURL(cfg.Endpoint))
		} else {
			opts = append(opts, otlpmetricgrpc.WithEndpoint(cfg.Endpoint))
		}
		if cfg.Insecure {
			opts = append(opts, otlpmetricgrpc.WithInsecure())
		}
		if cfg.Compression == "gzip" {
			opts = append(opts, otlpmetricgrpc.WithCompressor("gzip"))
		}
		inner, err = otlpmetricgrpc.New(ctx, opts...)
	case ProtocolHTTP:
		opts := []otlpmetrichttp.Option{
			otlpmetrichttp.WithHeaders(cfg.Headers),
			otlpmetrichttp.WithTimeout(cfg.Timeout),
		}
		if endpointIsURL(cfg.Endpoint) {
			opts = append(opts, otlpmetrichttp.WithEndpointURL(cfg.Endpoint))
		} else {
			opts = append(opts, otlpmetrichttp.WithEndpoint(cfg.Endpoint))
		}
		if cfg.Insecure {
			opts = append(opts, otlpmetrichttp.WithInsecure())
		}
		if cfg.Compression == "gzip" {
			opts = append(opts, otlpmetrichttp.WithCompression(otlpmetrichttp.GzipCompression))
		}
		inner, err = otlpmetrichttp.New(ctx, opts...)
	default:
		return nil, fmt.Errorf("unknown protocol: %v", cfg.Protocol)
	}
	if err != nil {
		return nil, err
	}
	return &metricExporter{inner: inner, stats: stats}, nil
}

// buildLogExporter creates a protocol-specific OTLP log exporter.
func buildLogExporter(ctx context.Context, cfg Config, stats *pipelineStats) (sdklog.Exporter, error) {
	var inner sdklog.Exporter
	var err error
	switch cfg.Protocol {
	case ProtocolGRPC:
		opts := []otlploggrpc.Option{
			otlploggrpc.WithHeaders(cfg.Headers),
			otlploggrpc.WithTimeout(cfg.Timeout),
		}
		if endpointIsURL(cfg.Endpoint) {
			opts = append(opts, otlploggrpc.WithEndpointURL(cfg.Endpoint))
		} else {
			opts = append(opts, otlploggrpc.WithEndpoint(cfg.Endpoint))
		}
		if cfg.Insecure {
			opts = append(opts, otlploggrpc.WithInsecure())
		}
		if cfg.Compression == "gzip" {
			opts = append(opts, otlploggrpc.WithCompressor("gzip"))
		}
		inner, err = otlploggrpc.New(ctx, opts...)
	case ProtocolHTTP:
		opts := []otlploghttp.Option{
			otlploghttp.WithHeaders(cfg.Headers),
			otlploghttp.WithTimeout(cfg.Timeout),
		}
		if endpointIsURL(cfg.Endpoint) {
			opts = append(opts, otlploghttp.WithEndpointURL(cfg.Endpoint))
		} else {
			opts = append(opts, otlploghttp.WithEndpoint(cfg.Endpoint))
		}
		if cfg.Insecure {
			opts = append(opts, otlploghttp.WithInsecure())
		}
		if cfg.Compression == "gzip" {
			opts = append(opts, otlploghttp.WithCompression(otlploghttp.GzipCompression))
		}
		inner, err = otlploghttp.New(ctx, opts...)
	default:
		return nil, fmt.Errorf("unknown protocol: %v", cfg.Protocol)
	}
	if err != nil {
		return nil, err
	}
	return &loggingExporter{inner: inner, stats: stats}, nil
}

func endpointIsURL(endpoint string) bool {
	return strings.Contains(endpoint, "://")
}
