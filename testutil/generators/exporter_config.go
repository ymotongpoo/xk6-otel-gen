// SPDX-License-Identifier: Apache-2.0

package generators

import (
	"fmt"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
	"pgregory.net/rapid"
)

const (
	minValidConfigTimeout      = time.Second
	maxValidConfigTimeout      = 30 * time.Second
	minValidConfigBatchTimeout = 100 * time.Millisecond
	maxValidConfigBatchTimeout = 30 * time.Second
)

// ConfigOption mutates exporter Config generation parameters.
type ConfigOption interface {
	applyConfigOption(*configOptions)
}

type configOptionFunc func(*configOptions)

func (f configOptionFunc) applyConfigOption(o *configOptions) {
	f(o)
}

type configOptions struct {
	fixedEndpoint      *string
	protocol           *exporter.Protocol
	minTimeout         time.Duration
	perSignalEndpoints bool
}

func defaultConfigOptions() configOptions {
	return configOptions{
		minTimeout:         minValidConfigTimeout,
		perSignalEndpoints: true,
	}
}

func applyConfigOptions(opts []ConfigOption) configOptions {
	o := defaultConfigOptions()
	for _, opt := range opts {
		opt.applyConfigOption(&o)
	}
	if o.minTimeout <= 0 {
		o.minTimeout = time.Millisecond
	}
	if o.minTimeout > maxValidConfigTimeout {
		o.minTimeout = maxValidConfigTimeout
	}
	return o
}

func (o protocolOption) applyConfigOption(configOpts *configOptions) {
	if o.config != nil {
		configOpts.protocol = o.config
	}
}

// WithFixedEndpoint fixes the generated exporter endpoint.
func WithFixedEndpoint(endpoint string) ConfigOption {
	return configOptionFunc(func(o *configOptions) {
		o.fixedEndpoint = &endpoint
	})
}

// WithMinTimeout sets the minimum generated exporter request timeout.
func WithMinTimeout(timeout time.Duration) ConfigOption {
	return configOptionFunc(func(o *configOptions) {
		o.minTimeout = timeout
	})
}

// WithoutPerSignalEndpoints disables generation of per-signal endpoint
// overrides (TracesEndpoint/MetricsEndpoint/LogsEndpoint), leaving them empty.
func WithoutPerSignalEndpoints() ConfigOption {
	return configOptionFunc(func(o *configOptions) {
		o.perSignalEndpoints = false
	})
}

// ValidConfig returns an exporter Config that passes Config.Validate.
func ValidConfig(opts ...ConfigOption) *rapid.Generator[exporter.Config] {
	o := applyConfigOptions(opts)
	return rapid.Custom(func(t *rapid.T) exporter.Config {
		protocol := rapid.SampledFrom([]exporter.Protocol{
			exporter.ProtocolGRPC,
			exporter.ProtocolHTTP,
		}).Draw(t, "protocol")
		if o.protocol != nil {
			protocol = *o.protocol
		}

		endpoint := validExporterEndpoint(t, "endpoint")
		if o.fixedEndpoint != nil {
			endpoint = *o.fixedEndpoint
		}

		batchSize := rapid.IntRange(128, 8192).Draw(t, "batch_size")
		maxQueueSize := rapid.IntRange(batchSize, batchSize*4).Draw(t, "max_queue_size")

		var tracesEndpoint, metricsEndpoint, logsEndpoint string
		if o.perSignalEndpoints {
			tracesEndpoint = optionalExporterEndpoint(t, "traces_endpoint")
			metricsEndpoint = optionalExporterEndpoint(t, "metrics_endpoint")
			logsEndpoint = optionalExporterEndpoint(t, "logs_endpoint")
		}

		return exporter.Config{
			Protocol:          protocol,
			Endpoint:          endpoint,
			TracesEndpoint:    tracesEndpoint,
			MetricsEndpoint:   metricsEndpoint,
			LogsEndpoint:      logsEndpoint,
			Headers:           validHeaderMap(t, "headers"),
			Insecure:          rapid.Bool().Draw(t, "insecure"),
			Compression:       rapid.SampledFrom([]string{"", "gzip"}).Draw(t, "compression"),
			Timeout:           validConfigDuration(t, "timeout", o.minTimeout, maxValidConfigTimeout),
			BatchSize:         batchSize,
			BatchTimeout:      validConfigDuration(t, "batch_timeout", minValidConfigBatchTimeout, maxValidConfigBatchTimeout),
			MaxQueueSize:      maxQueueSize,
			Sampler:           validSampler(t, "sampler"),
			SamplerArg:        rapid.Float64Range(0, 1).Draw(t, "sampler_arg"),
			SamplerArgSet:     true,
			ResourceOverrides: validResourceOverrideMap(t, "resource_overrides"),
		}
	})
}

// AnyConfig returns an exporter Config that may violate Config.Validate rules.
func AnyConfig(opts ...ConfigOption) *rapid.Generator[exporter.Config] {
	return rapid.Custom(func(t *rapid.T) exporter.Config {
		cfg := ValidConfig(opts...).Draw(t, "valid_config")
		switch rapid.IntRange(0, 14).Draw(t, "config_mutation") {
		case 0:
			return cfg
		case 1:
			cfg.Protocol = exporter.Protocol(rapid.IntRange(-10, -1).Draw(t, "invalid_protocol"))
		case 2:
			cfg.Endpoint = rapid.SampledFrom([]string{"", "localhost", "://missing-scheme"}).Draw(t, "invalid_endpoint")
		case 3:
			cfg.Headers = map[string]string{"bad header": "value"}
		case 4:
			cfg.Headers = map[string]string{"X-Test": ""}
		case 5:
			cfg.Compression = rapid.SampledFrom([]string{"zstd", "snappy", "gzip,broken"}).Draw(t, "invalid_compression")
		case 6:
			cfg.Timeout = time.Duration(rapid.IntRange(-10_000, 0).Draw(t, "invalid_timeout_ms")) * time.Millisecond
		case 7:
			cfg.BatchSize = rapid.IntRange(-128, 0).Draw(t, "invalid_batch_size")
		case 8:
			cfg.BatchTimeout = time.Duration(rapid.IntRange(-10_000, 0).Draw(t, "invalid_batch_timeout_ms")) * time.Millisecond
		case 9:
			cfg.MaxQueueSize = rapid.IntRange(1, cfg.BatchSize-1).Draw(t, "invalid_max_queue_size")
		case 10:
			cfg.Sampler = "invalid"
		case 11:
			cfg.Sampler = "traceidratio"
			cfg.SamplerArg = rapid.SampledFrom([]float64{-0.1, 1.1}).Draw(t, "invalid_sampler_arg")
			cfg.SamplerArgSet = true
		case 12:
			cfg.Insecure = true
			cfg.Certificate = "ca.pem"
		case 13:
			cfg.ClientCertificate = "client.pem"
			cfg.ClientKey = ""
		case 14:
			cfg.ClientCertificate = ""
			cfg.ClientKey = "client-key.pem"
		}
		return cfg
	})
}

func validSampler(t *rapid.T, label string) string {
	return rapid.SampledFrom([]string{"always_on", "always_off", "traceidratio"}).Draw(t, label)
}

// optionalExporterEndpoint returns a valid endpoint roughly half the time and
// an empty string ("unset") otherwise, for per-signal override generation.
func optionalExporterEndpoint(t *rapid.T, label string) string {
	if !rapid.Bool().Draw(t, label+"_set") {
		return ""
	}
	return validExporterEndpoint(t, label)
}

func validExporterEndpoint(t *rapid.T, label string) string {
	host := rapid.StringMatching(`^[a-z][a-z0-9-]{2,20}\.example\.com$`).Draw(t, label+"_host")
	port := rapid.IntRange(1, 65535).Draw(t, label+"_port")
	if rapid.Bool().Draw(t, label+"_url") {
		scheme := rapid.SampledFrom([]string{"http", "https"}).Draw(t, label+"_scheme")
		return fmt.Sprintf("%s://%s:%d", scheme, host, port)
	}
	return fmt.Sprintf("%s:%d", host, port)
}

func validConfigDuration(t *rapid.T, label string, min, max time.Duration) time.Duration {
	minMillis := int(min / time.Millisecond)
	maxMillis := int(max / time.Millisecond)
	if minMillis < 1 {
		minMillis = 1
	}
	if maxMillis < minMillis {
		maxMillis = minMillis
	}
	return time.Duration(rapid.IntRange(minMillis, maxMillis).Draw(t, label+"_ms")) * time.Millisecond
}

func validHeaderMap(t *rapid.T, label string) map[string]string {
	count := rapid.IntRange(0, 5).Draw(t, label+"_count")
	if count == 0 && rapid.Bool().Draw(t, label+"_nil") {
		return nil
	}
	keys := rapid.SliceOfNDistinct(
		rapid.StringMatching(`^[A-Za-z][A-Za-z0-9_-]{0,20}$`),
		count,
		count,
		func(key string) string { return key },
	).Draw(t, label+"_keys")
	headers := make(map[string]string, len(keys))
	for i, key := range keys {
		headers[key] = rapid.StringMatching(`^[A-Za-z0-9_.:/ -]{1,40}$`).Draw(t, fmt.Sprintf("%s_value_%d", label, i))
	}
	return headers
}

func validResourceOverrideMap(t *rapid.T, label string) map[string]string {
	count := rapid.IntRange(0, len(validResourceOverrideKeys())).Draw(t, label+"_count")
	keys := rapid.SliceOfNDistinct(
		rapid.SampledFrom(validResourceOverrideKeys()),
		count,
		count,
		func(key string) string { return key },
	).Draw(t, label+"_keys")
	if len(keys) == 0 && rapid.Bool().Draw(t, label+"_nil") {
		return nil
	}
	overrides := make(map[string]string, len(keys))
	for i, key := range keys {
		overrides[key] = rapid.StringMatching(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,40}$`).Draw(t, fmt.Sprintf("%s_value_%d", label, i))
	}
	return overrides
}

func validResourceOverrideKeys() []string {
	return []string{
		"service.name",
		"service.namespace",
		"service.version",
		"deployment.environment",
		"cloud.provider",
		"cloud.region",
		"k8s.cluster.name",
		"k8s.namespace.name",
		"host.name",
		"telemetry.sdk.language",
	}
}
