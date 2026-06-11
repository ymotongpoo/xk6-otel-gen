// SPDX-License-Identifier: Apache-2.0

package k6output

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"go.k6.io/k6/metrics"
	"go.k6.io/k6/output"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
)

const (
	instrumentationName = "github.com/ymotongpoo/xk6-otel-gen/k6output"
	runnerServiceName   = "xk6-otel-gen-runner"
	buildVersion        = ""
	maxFlushBatchSize   = 1024
	flushStopTimeout    = 5 * time.Second
	shutdownTimeout     = 30 * time.Second
)

type sharedPipeline interface {
	MetricExporter() sdkmetric.Exporter
	Shutdown(context.Context) error
}

// Output implements go.k6.io/k6/output.Output for the otel-gen extension.
type Output struct {
	params         Params
	runnerResource *resource.Resource

	startOnce sync.Once
	startErr  error
	stopOnce  sync.Once

	pipeline      sharedPipeline
	meterProvider *sdkmetric.MeterProvider
	instruments   instrumentMap
	setCache      *tagSetCache

	queue     chan metrics.SampleContainer
	ctx       context.Context
	cancelFn  context.CancelFunc
	flushDone chan struct{}
	drops     atomic.Uint64

	logger     func(format string, args ...any)
	infoLogger func(format string, args ...any)
}

func init() {
	output.RegisterExtension("otel-gen", New)
}

// New constructs an otel-gen k6 output instance from k6 output parameters.
func New(params output.Params) (output.Output, error) {
	parsed, err := parseOutArgs(params.ConfigArgument)
	if err != nil {
		return nil, fmt.Errorf("k6output: invalid --out args: %w", err)
	}
	if params.ScriptPath != nil {
		parsed.ScriptPath = params.ScriptPath.Path
	}

	warnLogger := log.Printf
	infoLogger := log.Printf
	if params.Logger != nil {
		warnLogger = params.Logger.Warnf
		infoLogger = params.Logger.Infof
	}

	return &Output{
		params:         parsed,
		runnerResource: buildRunnerResource(params),
		setCache:       &tagSetCache{},
		logger:         warnLogger,
		infoLogger:     infoLogger,
	}, nil
}

// Description returns the human-readable output description shown by k6.
func (o *Output) Description() string {
	if o == nil {
		return "k6 native metrics to OTLP/Metrics via xk6-otel-gen"
	}
	return fmt.Sprintf("k6 native metrics to OTLP/Metrics via xk6-otel-gen (endpoint=%s)", o.params.Endpoint)
}

// Start initializes the shared pipeline, runner MeterProvider, instruments,
// and asynchronous flush goroutine.
func (o *Output) Start() error {
	o.startOnce.Do(func() {
		pipeline, err := exporter.GetShared(func() (*exporter.Pipeline, error) {
			return exporter.New(buildPipelineConfig(o.params))
		})
		if err != nil {
			o.startErr = fmt.Errorf("k6output: pipeline init: %w", err)
			return
		}
		o.pipeline = pipeline

		reader := sdkmetric.NewPeriodicReader(
			pipeline.MetricExporter(),
			sdkmetric.WithInterval(o.params.FlushInterval),
		)
		o.meterProvider = sdkmetric.NewMeterProvider(
			sdkmetric.WithResource(o.runnerResource),
			sdkmetric.WithReader(reader),
		)
		if err := o.buildKnownInstruments(); err != nil {
			o.startErr = fmt.Errorf("k6output: instruments: %w", err)
			return
		}

		ctx, cancel := context.WithCancel(context.Background())
		o.ctx = ctx
		o.cancelFn = cancel
		o.queue = make(chan metrics.SampleContainer, o.params.QueueSize)
		o.flushDone = make(chan struct{})
		go o.flushLoop()
	})
	return o.startErr
}

// AddMetricSamples accepts k6 metric sample containers without blocking k6's
// engine hot path.
func (o *Output) AddMetricSamples(samples []metrics.SampleContainer) {
	if o == nil || o.queue == nil || o.ctx == nil {
		return
	}
	select {
	case <-o.ctx.Done():
		return
	default:
	}
	for _, sample := range samples {
		if !o.tryPush(sample) {
			o.drops.Add(1)
		}
	}
}

// Stop drains queued samples, flushes runner metrics, and shuts down the
// shared pipeline. It always returns nil to protect the k6 lifecycle.
func (o *Output) Stop() error {
	if o == nil {
		return nil
	}
	o.stopOnce.Do(func() {
		if o.cancelFn != nil {
			o.cancelFn()
		}
		if o.flushDone != nil {
			select {
			case <-o.flushDone:
			case <-time.After(flushStopTimeout):
				o.warn("k6output: flush loop did not stop within %s", flushStopTimeout)
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if o.meterProvider != nil {
			if err := o.meterProvider.ForceFlush(ctx); err != nil {
				o.warn("k6output: runner metric flush: %v", err)
			}
		}
		if o.pipeline != nil {
			if err := o.pipeline.Shutdown(ctx); err != nil {
				o.warn("k6output: Shutdown: %v", err)
			}
		}
		o.info("k6output: final stats: queue_drops=%d", o.drops.Load())
	})
	return nil
}

func buildRunnerResource(params output.Params) *resource.Resource {
	attrs := []attribute.KeyValue{
		semconv.ServiceName(runnerServiceName),
		semconv.TelemetrySDKName("opentelemetry"),
		semconv.TelemetrySDKLanguageGo,
		semconv.TelemetrySDKVersion(otel.Version()),
		semconv.ProcessRuntimeName("go"),
		semconv.ProcessRuntimeVersion(runtime.Version()),
	}
	if buildVersion != "" {
		attrs = append(attrs, semconv.ServiceVersion(buildVersion))
	}
	if params.ScriptPath != nil && params.ScriptPath.Path != "" {
		attrs = append(attrs, attribute.String("k6.test.name", filepath.Base(params.ScriptPath.Path)))
	}
	if host, err := os.Hostname(); err == nil && host != "" {
		attrs = append(attrs, attribute.String("host.name", host))
	}
	return resource.NewSchemaless(attrs...)
}

func buildPipelineConfig(params Params) exporter.Config {
	cfg := exporter.ConfigFromEnv()
	out := params.exporterConfig()
	if params.wasProvided("endpoint") {
		cfg.Endpoint = out.Endpoint
	}
	if params.wasProvided("protocol") {
		cfg.Protocol = out.Protocol
	}
	if params.wasProvided("insecure") {
		cfg.Insecure = out.Insecure
		cfg.InsecureSet = true
	}
	if params.wasProvided("caCert") {
		cfg.Certificate = out.Certificate
	}
	if params.wasProvided("clientCert") {
		cfg.ClientCertificate = out.ClientCertificate
	}
	if params.wasProvided("clientKey") {
		cfg.ClientKey = out.ClientKey
	}
	if params.wasProvided("headers") {
		cfg.Headers = out.Headers
	}
	if params.wasProvided("compression") {
		cfg.Compression = out.Compression
	}
	if params.wasProvided("timeout") {
		cfg.Timeout = out.Timeout
	}
	if params.wasProvided("batchSize") {
		cfg.BatchSize = out.BatchSize
	}
	if params.wasProvided("batchTimeout") {
		cfg.BatchTimeout = out.BatchTimeout
	}
	if params.wasProvided("maxQueueSize") {
		cfg.MaxQueueSize = out.MaxQueueSize
	}
	return cfg
}

func (o *Output) tryPush(s metrics.SampleContainer) bool {
	select {
	case o.queue <- s:
		return true
	default:
		select {
		case <-o.queue:
			o.drops.Add(1)
			o.warn("k6output: metric sample queue full, dropping oldest sample")
		default:
		}
		select {
		case o.queue <- s:
			return true
		default:
			return false
		}
	}
}

func (o *Output) flushLoop() {
	defer close(o.flushDone)
	ticker := time.NewTicker(o.params.FlushInterval)
	defer ticker.Stop()

	batch := make([]metrics.SampleContainer, 0, maxFlushBatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		for _, container := range batch {
			o.emitContainer(container)
		}
		batch = batch[:0]
	}

	for {
		select {
		case <-o.ctx.Done():
			for {
				select {
				case sample := <-o.queue:
					batch = append(batch, sample)
					if len(batch) >= maxFlushBatchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		case sample := <-o.queue:
			batch = append(batch, sample)
			if len(batch) >= maxFlushBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (o *Output) buildKnownInstruments() error {
	meter := o.meterProvider.Meter(instrumentationName)
	for _, spec := range knownK6Metrics {
		if _, err := o.buildInstrument(meter, spec.k6Name, spec.otelName, spec.instType, spec.unit); err != nil {
			return fmt.Errorf("build %s: %w", spec.otelName, err)
		}
	}
	return nil
}

func (o *Output) lookupOrBuildInstrument(name string, k6Type metrics.MetricType, unit string) any {
	if name == "" || o == nil || o.meterProvider == nil {
		return nil
	}
	if spec, ok := knownMetricSpec(name); ok {
		unit = spec.unit
		switch spec.instType {
		case tInstCounter:
			if v, ok := o.instruments.counters.Load(name); ok {
				return v
			}
			return o.buildLazyInstrument(name, spec.otelName, spec.instType, unit)
		case tInstHistogram:
			if v, ok := o.instruments.histograms.Load(name); ok {
				return v
			}
			return o.buildLazyInstrument(name, spec.otelName, spec.instType, unit)
		case tInstGauge:
			if v, ok := o.instruments.gauges.Load(name); ok {
				return v
			}
			return o.buildLazyInstrument(name, spec.otelName, spec.instType, unit)
		}
	}

	otelName := "k6." + dotted(name)
	switch k6Type {
	case metrics.Counter, metrics.Rate:
		if v, ok := o.instruments.counters.Load(name); ok {
			return v
		}
		if k6Type == metrics.Rate || !stringsHasTotalSuffix(otelName) {
			otelName += ".total"
		}
		return o.buildLazyInstrument(name, otelName, tInstCounter, unit)
	case metrics.Trend:
		if v, ok := o.instruments.histograms.Load(name); ok {
			return v
		}
		return o.buildLazyInstrument(name, otelName, tInstHistogram, unit)
	case metrics.Gauge:
		if v, ok := o.instruments.gauges.Load(name); ok {
			return v
		}
		return o.buildLazyInstrument(name, otelName, tInstGauge, unit)
	default:
		o.warn("k6output: unsupported k6 metric type %s for %q", k6Type, name)
		return nil
	}
}

func (o *Output) buildLazyInstrument(k6Name, otelName string, instType instrumentType, unit string) any {
	inst, err := o.buildInstrument(o.meterProvider.Meter(instrumentationName), k6Name, otelName, instType, unit)
	if err != nil {
		o.warn("k6output: lazy build %s: %v", otelName, err)
		return nil
	}
	return inst
}

func (o *Output) buildInstrument(meter metric.Meter, k6Name, otelName string, instType instrumentType, unit string) (any, error) {
	switch instType {
	case tInstCounter:
		c, err := meter.Float64Counter(otelName, counterUnitOption(unit)...)
		if err != nil {
			return nil, err
		}
		actual, _ := o.instruments.counters.LoadOrStore(k6Name, c)
		return actual, nil
	case tInstHistogram:
		h, err := meter.Float64Histogram(otelName, histogramUnitOption(unit)...)
		if err != nil {
			return nil, err
		}
		actual, _ := o.instruments.histograms.LoadOrStore(k6Name, h)
		return actual, nil
	case tInstGauge:
		g, err := meter.Float64Gauge(otelName, gaugeUnitOption(unit)...)
		if err != nil {
			return nil, err
		}
		actual, _ := o.instruments.gauges.LoadOrStore(k6Name, g)
		return actual, nil
	default:
		return nil, fmt.Errorf("unknown instrument type %d", instType)
	}
}

func (o *Output) emitContainer(container metrics.SampleContainer) {
	if container == nil {
		return
	}
	for _, sample := range container.GetSamples() {
		o.emitSample(sample)
	}
}

func (o *Output) emitSample(sample metrics.Sample) {
	if sample.Metric == nil {
		return
	}
	name := sample.Metric.Name
	k6Type := sample.Metric.Type
	inst := o.lookupOrBuildInstrument(name, k6Type, k6UnitHint(name))
	if inst == nil {
		return
	}

	value := sample.Value
	if k6Type == metrics.Rate {
		if value == 0 {
			return
		}
		value = 1
	}
	if k6Type == metrics.Counter && value < 0 {
		return
	}

	attrSet := o.setCache.get(sampleTagMap(sample))
	ctx := context.Background()
	switch v := inst.(type) {
	case metric.Float64Counter:
		v.Add(ctx, value, metric.WithAttributeSet(attrSet))
	case metric.Float64Histogram:
		v.Record(ctx, value, metric.WithAttributeSet(attrSet))
	case metric.Float64Gauge:
		v.Record(ctx, value, metric.WithAttributeSet(attrSet))
	}
}

func sampleTagMap(sample metrics.Sample) map[string]string {
	if sample.Tags == nil || sample.Tags.IsEmpty() {
		return nil
	}
	return sample.Tags.Map()
}

func counterUnitOption(unit string) []metric.Float64CounterOption {
	if unit == "" {
		return nil
	}
	return []metric.Float64CounterOption{metric.WithUnit(unit)}
}

func histogramUnitOption(unit string) []metric.Float64HistogramOption {
	if unit == "" {
		return nil
	}
	return []metric.Float64HistogramOption{metric.WithUnit(unit)}
}

func gaugeUnitOption(unit string) []metric.Float64GaugeOption {
	if unit == "" {
		return nil
	}
	return []metric.Float64GaugeOption{metric.WithUnit(unit)}
}

func stringsHasTotalSuffix(name string) bool {
	return len(name) >= len(".total") && name[len(name)-len(".total"):] == ".total"
}

func (o *Output) warn(format string, args ...any) {
	if o != nil && o.logger != nil {
		o.logger(format, args...)
		return
	}
	log.Printf(format, args...)
}

func (o *Output) info(format string, args ...any) {
	if o != nil && o.infoLogger != nil {
		o.infoLogger(format, args...)
		return
	}
	log.Printf(format, args...)
}
