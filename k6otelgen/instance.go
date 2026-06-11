// SPDX-License-Identifier: Apache-2.0

package k6otelgen

import (
	"bytes"
	"context"
	"errors"
	"os"

	"github.com/grafana/sobek"
	"go.k6.io/k6/js/modules"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
	"github.com/ymotongpoo/xk6-otel-gen/journey"
	"github.com/ymotongpoo/xk6-otel-gen/synth"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

// ModuleInstance is constructed once per k6 VU and holds per-VU state.
type ModuleInstance struct {
	root    *RootModule
	vu      modules.VU
	engine  *journey.Engine
	synth   synth.Synthesizer
	handle  *TopologyHandle
	initErr error
}

// Stats is the JS-friendly snapshot returned by stats.
type Stats struct {
	TracesExported  int64 `js:"tracesExported"`
	TracesFailed    int64 `js:"tracesFailed"`
	MetricsExported int64 `js:"metricsExported"`
	MetricsFailed   int64 `js:"metricsFailed"`
	LogsExported    int64 `js:"logsExported"`
	LogsFailed      int64 `js:"logsFailed"`
}

// Exports returns the JS-visible API surface for this VU.
func (i *ModuleInstance) Exports() modules.Exports {
	return modules.Exports{
		Named: map[string]any{
			"configure": i.jsConfigure,
			"load":      i.jsLoad,
			"stats":     i.jsStats,
			"journeys":  i.jsJourneys,
		},
	}
}

// Load loads and validates a topology YAML file once, then returns this VU's handle.
func (i *ModuleInstance) Load(path string) (*TopologyHandle, error) {
	if i.root == nil {
		return nil, &ConfigError{Kind: "not_loaded", Path: path}
	}
	i.root.schemaOnce.Do(func() {
		data, err := os.ReadFile(path)
		if err != nil {
			i.root.schemaErr = &ConfigError{Kind: "file_not_found", Path: path, Inner: err}
			return
		}
		schema, err := topology.Parse(bytes.NewReader(data))
		if err != nil {
			kind := "parse_error"
			var validationErr *topology.ValidationError
			if errors.As(err, &validationErr) {
				kind = "validate_error"
			}
			i.root.schemaErr = &ConfigError{Kind: kind, Path: path, Inner: err}
			return
		}
		i.root.schema = schema
		i.root.overlay = schema.ApplyFaults()
		i.root.loadedPath = path
	})
	if i.root.schemaErr != nil {
		return nil, i.root.schemaErr
	}
	if i.root.loadedPath != path {
		return nil, &ConfigError{Kind: "path_mismatch", Path: path}
	}
	if i.handle == nil {
		if err := i.lateInit(); err != nil {
			return nil, err
		}
	}
	return i.handle, nil
}

// Configure applies JS options over OTLP environment settings once.
func (i *ModuleInstance) Configure(opts map[string]any) error {
	if i.root == nil {
		return &ConfigError{Kind: "not_loaded"}
	}

	i.root.configureMu.Lock()
	defer i.root.configureMu.Unlock()

	if i.root.configured {
		return &ConfigError{Kind: "already_configured"}
	}
	i.root.configureOnce.Do(func() {
		jsCfg, err := optsToConfig(opts)
		if err != nil {
			i.root.configureErr = err
			return
		}
		envCfg := exporter.ConfigFromEnv()
		i.root.config = exporter.Config{}.MergeWith(envCfg).MergeWith(jsCfg)
		i.root.configured = true
	})
	return i.root.configureErr
}

// Stats returns the shared exporter pipeline counters.
func (i *ModuleInstance) Stats() (Stats, error) {
	pipeline, err := i.getOrBuildPipeline()
	if err != nil {
		return Stats{}, &ConfigError{Kind: "pipeline_error", Inner: err}
	}
	stats := pipeline.Stats()
	return Stats{
		TracesExported:  stats.TracesExported,
		TracesFailed:    stats.TracesFailed,
		MetricsExported: stats.MetricsExported,
		MetricsFailed:   stats.MetricsFailed,
		LogsExported:    stats.LogsExported,
		LogsFailed:      stats.LogsFailed,
	}, nil
}

// Journeys returns sorted journey names, or an empty slice before load.
func (i *ModuleInstance) Journeys() []string {
	if i.root == nil || i.root.schema == nil || i.engine == nil {
		return []string{}
	}
	return i.engine.ListJourneys()
}

func (i *ModuleInstance) jsConfigure(call sobek.FunctionCall) sobek.Value {
	var opts map[string]any
	if err := i.vu.Runtime().ExportTo(call.Argument(0), &opts); err != nil {
		throwJSException(i.vu.Runtime(), &ConfigError{Kind: "invalid_opts", Inner: err})
	}
	if opts == nil {
		opts = map[string]any{}
	}
	if err := i.Configure(opts); err != nil {
		throwJSException(i.vu.Runtime(), err)
	}
	return sobek.Undefined()
}

func (i *ModuleInstance) jsLoad(call sobek.FunctionCall) sobek.Value {
	handle, err := i.Load(call.Argument(0).String())
	if err != nil {
		throwJSException(i.vu.Runtime(), err)
	}
	return i.vu.Runtime().ToValue(handle)
}

func (i *ModuleInstance) jsStats(sobek.FunctionCall) sobek.Value {
	stats, err := i.Stats()
	if err != nil {
		throwJSException(i.vu.Runtime(), err)
	}
	return i.vu.Runtime().ToValue(stats)
}

func (i *ModuleInstance) jsJourneys(sobek.FunctionCall) sobek.Value {
	return i.vu.Runtime().ToValue(i.Journeys())
}

func (i *ModuleInstance) getOrBuildPipeline() (*exporter.Pipeline, error) {
	if i.root == nil {
		return nil, &ConfigError{Kind: "not_loaded"}
	}
	return exporter.GetShared(func() (*exporter.Pipeline, error) {
		return exporter.New(i.root.config)
	})
}

func (i *ModuleInstance) lateInit() error {
	if i.root == nil || i.root.schema == nil {
		return &ConfigError{Kind: "not_loaded"}
	}
	pipeline, err := i.getOrBuildPipeline()
	if err != nil {
		return err
	}
	syn := synth.NewDefault(
		pipeline.TracerProvider(),
		pipeline.MeterProvider(),
		pipeline.LoggerProvider(),
	)
	engine := journey.NewEngineWithSeed(i.root.schema, i.root.overlay, syn, seedForVU(i.vu))
	i.synth = syn
	i.engine = engine
	i.handle = &TopologyHandle{
		runtime:  runtimeForVU(i.vu),
		engine:   engine,
		module:   i.root,
		instance: i,
		name:     i.root.loadedPath,
	}
	i.root.handle = i.handle
	return nil
}

func (i *ModuleInstance) vuContext() context.Context {
	if i == nil || i.vu == nil || i.vu.Context() == nil {
		return context.Background()
	}
	return i.vu.Context()
}

func runtimeForVU(vu modules.VU) *sobek.Runtime {
	if vu == nil {
		return nil
	}
	return vu.Runtime()
}
