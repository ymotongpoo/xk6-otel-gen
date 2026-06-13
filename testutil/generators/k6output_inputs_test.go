// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package generators

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
	"github.com/ymotongpoo/xk6-otel-gen/k6output"
	"go.k6.io/k6/metrics"
	"pgregory.net/rapid"
)

func TestValidK6Sample_Invariants_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		sample := ValidK6Sample().Draw(t, "sample")
		if !validK6SampleShape(sample) {
			t.Fatalf("ValidK6Sample produced invalid sample: %#v", sample)
		}
	})
}

func TestAnyK6Sample_SometimesInvalid_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		invalid := false
		for i := 0; i < 100; i++ {
			sample := AnyK6Sample().Draw(t, fmt.Sprintf("sample_%d", i))
			if !validK6SampleShape(sample) {
				invalid = true
				break
			}
		}
		if !invalid {
			t.Fatal("AnyK6Sample produced no invalid values in 100 draws")
		}
	})
}

func TestValidOutputParams_Invariants_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		params := ValidOutputParams().Draw(t, "params")
		if !validOutputParamsShape(params) {
			t.Fatalf("ValidOutputParams produced invalid params: %#v", params)
		}
	})
}

func TestAnyOutputParams_SometimesInvalid_Property(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		invalid := false
		for i := 0; i < 100; i++ {
			params := AnyOutputParams().Draw(t, fmt.Sprintf("params_%d", i))
			if !validOutputParamsShape(params) {
				invalid = true
				break
			}
		}
		if !invalid {
			t.Fatal("AnyOutputParams produced no invalid values in 100 draws")
		}
	})
}

func validK6SampleShape(sample metrics.Sample) bool {
	if sample.Metric == nil || sample.Metric.Name == "" {
		return false
	}
	if !validK6MetricType(sample.Metric.Type) {
		return false
	}
	if sample.Value <= 0 {
		return false
	}
	if sample.Tags == nil {
		return false
	}
	tags := sample.Tags.Map()
	if len(tags) > 5 {
		return false
	}
	for key := range tags {
		if key == "" || strings.ContainsAny(key, ". /") {
			return false
		}
	}
	return true
}

func validK6MetricType(typ metrics.MetricType) bool {
	for _, valid := range validK6MetricTypes() {
		if typ == valid {
			return true
		}
	}
	return false
}

func validOutputParamsShape(params k6output.Params) bool {
	return validOutputEndpoint(params.Endpoint) &&
		(params.Protocol == exporter.ProtocolGRPC || params.Protocol == exporter.ProtocolHTTP) &&
		(params.Compression == "" || params.Compression == "gzip") &&
		params.Timeout > 0 &&
		params.BatchSize > 0 &&
		params.BatchTimeout > 0 &&
		params.MaxQueueSize >= params.BatchSize &&
		params.QueueSize >= 10 &&
		params.QueueSize <= 10_000 &&
		params.FlushInterval > 0
}

func validOutputEndpoint(endpoint string) bool {
	if endpoint == "" {
		return false
	}
	if strings.Contains(endpoint, "://") {
		u, err := url.Parse(endpoint)
		return err == nil && u.Scheme != "" && u.Host != ""
	}
	host, port, err := net.SplitHostPort(endpoint)
	return err == nil && host != "" && port != ""
}
