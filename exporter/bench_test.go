// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package exporter

import (
	"context"
	"net"
	"testing"
	"time"

	logcollectorpb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	metriccollectorpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	tracecollectorpb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
)

func benchmarkConfig() Config {
	return Config{
		Protocol:     ProtocolGRPC,
		Endpoint:     "localhost:4317",
		Insecure:     true,
		Timeout:      5 * time.Second,
		BatchSize:    512,
		BatchTimeout: time.Second,
		MaxQueueSize: 2048,
	}
}

func BenchmarkNew(b *testing.B) {
	cfg := benchmarkConfig()
	cleanup := startBenchmarkCollector(b, cfg.Endpoint)
	defer cleanup()

	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p, err := New(cfg)
		if err != nil {
			b.Fatal(err)
		}
		if err := p.Shutdown(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}

func startBenchmarkCollector(b *testing.B, endpoint string) func() {
	b.Helper()

	listener, err := net.Listen("tcp", endpoint)
	if err != nil {
		b.Fatalf("listen benchmark OTLP collector: %v", err)
	}
	server := grpc.NewServer()
	tracecollectorpb.RegisterTraceServiceServer(server, benchmarkTraceCollector{})
	metriccollectorpb.RegisterMetricsServiceServer(server, benchmarkMetricCollector{})
	logcollectorpb.RegisterLogsServiceServer(server, benchmarkLogCollector{})

	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = server.Serve(listener)
	}()

	return func() {
		server.Stop()
		<-done
	}
}

type benchmarkTraceCollector struct {
	tracecollectorpb.UnimplementedTraceServiceServer
}

func (benchmarkTraceCollector) Export(context.Context, *tracecollectorpb.ExportTraceServiceRequest) (*tracecollectorpb.ExportTraceServiceResponse, error) {
	return &tracecollectorpb.ExportTraceServiceResponse{}, nil
}

type benchmarkMetricCollector struct {
	metriccollectorpb.UnimplementedMetricsServiceServer
}

func (benchmarkMetricCollector) Export(context.Context, *metriccollectorpb.ExportMetricsServiceRequest) (*metriccollectorpb.ExportMetricsServiceResponse, error) {
	return &metriccollectorpb.ExportMetricsServiceResponse{}, nil
}

type benchmarkLogCollector struct {
	logcollectorpb.UnimplementedLogsServiceServer
}

func (benchmarkLogCollector) Export(context.Context, *logcollectorpb.ExportLogsServiceRequest) (*logcollectorpb.ExportLogsServiceResponse, error) {
	return &logcollectorpb.ExportLogsServiceResponse{}, nil
}
