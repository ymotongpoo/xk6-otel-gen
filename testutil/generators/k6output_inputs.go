// SPDX-License-Identifier: Apache-2.0

package generators

import (
	"fmt"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
	"github.com/ymotongpoo/xk6-otel-gen/k6output"
	"go.k6.io/k6/metrics"
	"pgregory.net/rapid"
)

// SampleOption mutates k6 output sample generation parameters.
type SampleOption interface {
	applySampleOption(*sampleOptions)
}

type sampleOptionFunc func(*sampleOptions)

func (f sampleOptionFunc) applySampleOption(o *sampleOptions) {
	f(o)
}

type sampleOptions struct {
	metricType *metrics.MetricType
	maxTags    int
}

func applySampleOptions(opts []SampleOption) sampleOptions {
	o := sampleOptions{maxTags: 5}
	for _, opt := range opts {
		opt.applySampleOption(&o)
	}
	if o.maxTags < 0 {
		o.maxTags = 0
	}
	return o
}

// WithK6MetricType fixes the metric type used by ValidK6Sample.
func WithK6MetricType(typ metrics.MetricType) SampleOption {
	return sampleOptionFunc(func(o *sampleOptions) {
		o.metricType = &typ
	})
}

// WithMaxK6Tags caps generated k6 sample tags.
func WithMaxK6Tags(n int) SampleOption {
	return sampleOptionFunc(func(o *sampleOptions) {
		o.maxTags = n
	})
}

// ParamsOption mutates k6output Params generation parameters.
type ParamsOption interface {
	applyParamsOption(*paramsOptions)
}

type paramsOptionFunc func(*paramsOptions)

func (f paramsOptionFunc) applyParamsOption(o *paramsOptions) {
	f(o)
}

type paramsOptions struct {
	protocol *exporter.Protocol
}

func applyParamsOptions(opts []ParamsOption) paramsOptions {
	o := paramsOptions{}
	for _, opt := range opts {
		opt.applyParamsOption(&o)
	}
	return o
}

// WithOutputProtocol fixes the generated k6output protocol.
func WithOutputProtocol(protocol exporter.Protocol) ParamsOption {
	return paramsOptionFunc(func(o *paramsOptions) {
		o.protocol = &protocol
	})
}

// ValidK6Sample returns a k6 metric sample accepted by k6output conversion.
func ValidK6Sample(opts ...SampleOption) *rapid.Generator[metrics.Sample] {
	o := applySampleOptions(opts)
	return rapid.Custom(func(t *rapid.T) metrics.Sample {
		typ := rapid.SampledFrom(validK6MetricTypes()).Draw(t, "metric_type")
		if o.metricType != nil {
			typ = *o.metricType
		}
		name := validK6MetricNameForType(typ)
		value := rapid.Float64Range(0.001, 10_000).Draw(t, "value")
		if typ == metrics.Rate {
			value = 1
		}
		return buildK6Sample(t, name, typ, value, validK6TagMap(t, "tags", o.maxTags))
	})
}

// AnyK6Sample returns a k6 metric sample that may violate U6 input invariants.
func AnyK6Sample(opts ...SampleOption) *rapid.Generator[metrics.Sample] {
	o := applySampleOptions(opts)
	return rapid.Custom(func(t *rapid.T) metrics.Sample {
		sample := ValidK6Sample(opts...).Draw(t, "valid_sample")
		switch rapid.IntRange(0, 4).Draw(t, "sample_mutation") {
		case 0:
			return sample
		case 1:
			sample.Metric = nil
		case 2:
			sample.Value = rapid.Float64Range(-10_000, -0.001).Draw(t, "negative_value")
		case 3:
			sample.Tags = metrics.NewRegistry().RootTagSet().WithTagsFromMap(validK6TagMap(t, "large_tags", maxInt(o.maxTags+1, 20)))
		case 4:
			sample.Metric = &metrics.Metric{Name: "", Type: metrics.MetricType(-1)}
		}
		return sample
	})
}

// ValidOutputParams returns k6output Params that satisfy parser/output invariants.
func ValidOutputParams(opts ...ParamsOption) *rapid.Generator[k6output.Params] {
	o := applyParamsOptions(opts)
	return rapid.Custom(func(t *rapid.T) k6output.Params {
		protocol := rapid.SampledFrom([]exporter.Protocol{exporter.ProtocolGRPC, exporter.ProtocolHTTP}).Draw(t, "protocol")
		if o.protocol != nil {
			protocol = *o.protocol
		}
		batchSize := rapid.IntRange(1, 2048).Draw(t, "batch_size")
		return k6output.Params{
			Endpoint:      validExporterEndpoint(t, "endpoint"),
			Protocol:      protocol,
			Insecure:      rapid.Bool().Draw(t, "insecure"),
			Headers:       validHeaderMap(t, "headers"),
			Compression:   rapid.SampledFrom([]string{"", "gzip"}).Draw(t, "compression"),
			Timeout:       validConfigDuration(t, "timeout", time.Millisecond, 30*time.Second),
			BatchSize:     batchSize,
			BatchTimeout:  validConfigDuration(t, "batch_timeout", time.Millisecond, 30*time.Second),
			MaxQueueSize:  rapid.IntRange(batchSize, batchSize*4).Draw(t, "max_queue_size"),
			QueueSize:     rapid.IntRange(10, 10_000).Draw(t, "queue_size"),
			FlushInterval: validConfigDuration(t, "flush_interval", time.Millisecond, 10*time.Second),
			ScriptPath:    "testdata/script.js",
		}
	})
}

// AnyOutputParams returns k6output Params that may violate U6 invariants.
func AnyOutputParams(opts ...ParamsOption) *rapid.Generator[k6output.Params] {
	return rapid.Custom(func(t *rapid.T) k6output.Params {
		params := ValidOutputParams(opts...).Draw(t, "valid_output_params")
		switch rapid.IntRange(0, 6).Draw(t, "params_mutation") {
		case 0:
			return params
		case 1:
			params.Protocol = exporter.Protocol(rapid.IntRange(-10, -1).Draw(t, "bad_protocol"))
		case 2:
			params.Endpoint = rapid.SampledFrom([]string{"", "localhost", "://missing"}).Draw(t, "bad_endpoint")
		case 3:
			params.Timeout = -time.Second
		case 4:
			params.BatchSize = 0
		case 5:
			params.MaxQueueSize = params.BatchSize - 1
		case 6:
			params.QueueSize = rapid.SampledFrom([]int{0, 9, 10_001}).Draw(t, "bad_queue_size")
		}
		return params
	})
}

func validK6MetricTypes() []metrics.MetricType {
	return []metrics.MetricType{metrics.Counter, metrics.Trend, metrics.Gauge, metrics.Rate}
}

func validK6MetricNameForType(typ metrics.MetricType) string {
	switch typ {
	case metrics.Counter:
		return "iterations"
	case metrics.Trend:
		return "http_req_duration"
	case metrics.Gauge:
		return "vus"
	case metrics.Rate:
		return "http_req_failed"
	default:
		return "custom_metric"
	}
}

func buildK6Sample(t *rapid.T, name string, typ metrics.MetricType, value float64, tags map[string]string) metrics.Sample {
	registry := metrics.NewRegistry()
	metric, err := registry.NewMetric(name, typ)
	if err != nil {
		t.Fatalf("NewMetric(%q, %s) error = %v", name, typ, err)
	}
	return metrics.Sample{
		TimeSeries: metrics.TimeSeries{
			Metric: metric,
			Tags:   registry.RootTagSet().WithTagsFromMap(tags),
		},
		Time:  time.Now(),
		Value: value,
	}
}

func validK6TagMap(t *rapid.T, label string, maxTags int) map[string]string {
	count := rapid.IntRange(0, maxTags).Draw(t, label+"_count")
	keys := rapid.SliceOfNDistinct(
		rapid.StringMatching(`^[A-Za-z][A-Za-z0-9_-]{0,12}$`),
		count,
		count,
		func(key string) string { return key },
	).Draw(t, label+"_keys")
	tags := make(map[string]string, len(keys))
	for i, key := range keys {
		tags[key] = rapid.StringMatching(`^[A-Za-z0-9_.:/ -]{0,32}$`).Draw(t, fmt.Sprintf("%s_value_%d", label, i))
	}
	return tags
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
