// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package synth

import (
	"context"
	"fmt"
	"math"
	"sync"
	"sync/atomic"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	semconv "go.opentelemetry.io/otel/semconv/v1.27.0"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

// ObservableState stores process-wide accumulator values consumed by
// service-scoped observable metrics.
type ObservableState struct {
	values sync.Map
}

type observableValue struct {
	bits atomic.Uint64
}

// NewObservableState returns an empty accumulator state.
func NewObservableState() *ObservableState {
	return &ObservableState{}
}

// Add increments key by delta. Empty keys are ignored.
func (s *ObservableState) Add(key string, delta float64) {
	if s == nil || key == "" {
		return
	}
	cell := s.valueFor(key)
	for {
		oldBits := cell.bits.Load()
		oldValue := math.Float64frombits(oldBits)
		newBits := math.Float64bits(oldValue + delta)
		if cell.bits.CompareAndSwap(oldBits, newBits) {
			return
		}
	}
}

// Load returns the current value for key, or 0 when key has not been updated.
func (s *ObservableState) Load(key string) float64 {
	if s == nil || key == "" {
		return 0
	}
	value, ok := s.values.Load(key)
	if !ok {
		return 0
	}
	cell, ok := value.(*observableValue)
	if !ok || cell == nil {
		return 0
	}
	return math.Float64frombits(cell.bits.Load())
}

func (s *ObservableState) valueFor(key string) *observableValue {
	cell := &observableValue{}
	actual, _ := s.values.LoadOrStore(key, cell)
	if existing, ok := actual.(*observableValue); ok && existing != nil {
		return existing
	}
	return cell
}

// ServiceMetricRegistration unregisters observable service metric callbacks.
type ServiceMetricRegistration interface {
	Unregister() error
}

type noopServiceMetricRegistration struct{}

func (noopServiceMetricRegistration) Unregister() error { return nil }

type serviceMetricRegistration struct {
	reg metric.Registration
}

func (r serviceMetricRegistration) Unregister() error {
	if r.reg == nil {
		return nil
	}
	return r.reg.Unregister()
}

type observableMetricBinding struct {
	service    *topology.Service
	spec       topology.ObservableMetricSpec
	attrs      attribute.Set
	instrument metric.Float64Observable
}

// RegisterServiceMetrics registers all service-scoped observable custom
// metrics declared by schema against mp. The returned registration is
// idempotent and should be unregistered when the owning MeterProvider is no
// longer used.
func RegisterServiceMetrics(mp metric.MeterProvider, schema *topology.Schema, overlay *topology.FaultOverlay, state *ObservableState) (ServiceMetricRegistration, error) {
	if mp == nil {
		return nil, fmt.Errorf("synth: register service metrics: meter provider is nil")
	}
	if schema == nil {
		return nil, fmt.Errorf("synth: register service metrics: schema is nil")
	}
	if state == nil {
		return nil, fmt.Errorf("synth: register service metrics: observable state is nil")
	}

	meter := mp.Meter(instrumentationName)
	bindings := make([]observableMetricBinding, 0)
	instruments := make([]metric.Observable, 0)
	for _, svc := range schema.Services {
		if svc == nil {
			continue
		}
		for _, spec := range svc.Metrics {
			inst, err := observableInstrument(meter, spec)
			if err != nil {
				return nil, fmt.Errorf("synth: register service metric %q: %w", spec.Name, err)
			}
			bindings = append(bindings, observableMetricBinding{
				service:    svc,
				spec:       spec,
				attrs:      observableMetricAttrs(svc, spec),
				instrument: inst,
			})
			instruments = append(instruments, inst)
		}
	}
	if len(instruments) == 0 {
		return noopServiceMetricRegistration{}, nil
	}

	reg, err := meter.RegisterCallback(func(ctx context.Context, obs metric.Observer) error {
		for _, binding := range bindings {
			value := observableMetricValue(binding.service, binding.spec, overlay, state)
			obs.ObserveFloat64(binding.instrument, value, metric.WithAttributeSet(binding.attrs))
		}
		return nil
	}, instruments...)
	if err != nil {
		return nil, fmt.Errorf("synth: register service metric callback: %w", err)
	}
	return serviceMetricRegistration{reg: reg}, nil
}

func observableInstrument(meter metric.Meter, spec topology.ObservableMetricSpec) (metric.Float64Observable, error) {
	switch spec.Type {
	case topology.MetricObservableGauge:
		return meter.Float64ObservableGauge(spec.Name, metric.WithUnit(spec.Unit))
	case topology.MetricObservableCounter:
		return meter.Float64ObservableCounter(spec.Name, metric.WithUnit(spec.Unit))
	default:
		return nil, fmt.Errorf("unsupported observable metric type %d", spec.Type)
	}
}

func observableMetricAttrs(svc *topology.Service, spec topology.ObservableMetricSpec) attribute.Set {
	attrs := make([]attribute.KeyValue, 0, len(spec.Attributes)+1)
	if svc != nil {
		attrs = append(attrs, semconv.ServiceName(string(svc.Name)))
	}
	for key, value := range spec.Attributes {
		attrs = append(attrs, attribute.Key(key).String(toAttributeString(value)))
	}
	return attribute.NewSet(attrs...)
}

func observableMetricValue(svc *topology.Service, spec topology.ObservableMetricSpec, overlay *topology.FaultOverlay, state *ObservableState) float64 {
	value := spec.Baseline
	if spec.Source != nil {
		value += state.Load(spec.Source.Accumulator)
		if spec.Source.Minus != "" {
			value -= state.Load(spec.Source.Minus)
		}
	}
	if spec.WhenFault != nil && deterministicServiceFaultActive(overlay, svc, spec.WhenFault.Kind) {
		if spec.WhenFault.HasValue {
			return spec.WhenFault.Value
		}
		return value + spec.WhenFault.Delta
	}
	return value
}

func deterministicServiceFaultActive(overlay *topology.FaultOverlay, svc *topology.Service, kind topology.FaultKind) bool {
	for _, spec := range overlay.NodeFaults(svc) {
		if spec.Kind == kind && spec.Severity.Probability >= 1 {
			return true
		}
	}
	return false
}
