// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package k6otelgen

import (
	"fmt"

	"github.com/grafana/sobek"

	"github.com/ymotongpoo/xk6-otel-gen/journey"
)

// TopologyHandle is the JS-visible object returned by load.
type TopologyHandle struct {
	runtime  *sobek.Runtime
	engine   *journey.Engine
	module   *RootModule
	instance *ModuleInstance
	name     string
}

// RunJourney executes the named journey against this handle's per-VU engine.
func (h *TopologyHandle) RunJourney(name string) {
	if h.engine == nil || h.instance == nil {
		throwJSException(h.runtime, &ConfigError{Kind: "not_loaded"})
	}
	plan, err := h.engine.BuildPlan(name)
	if err != nil {
		throwJSException(h.runtime, err)
	}
	if err := h.engine.Execute(h.instance.vuContext(), plan); err != nil {
		throwJSException(h.runtime, err)
	}
	h.instance.emitExporterStats()
}

// RunRandomJourney picks a configured journey by weight, executes it, and
// returns the selected journey name.
func (h *TopologyHandle) RunRandomJourney() string {
	if h.engine == nil || h.instance == nil {
		throwJSException(h.runtime, &ConfigError{Kind: "not_loaded"})
	}
	name := h.engine.PickJourney()
	if name == "" {
		throwJSException(h.runtime, &ConfigError{Kind: "not_loaded"})
	}
	h.RunJourney(name)
	return name
}

// Journeys returns the sorted journey names available through this handle.
func (h *TopologyHandle) Journeys() []string {
	if h.engine == nil {
		return []string{}
	}
	return h.engine.ListJourneys()
}

// JourneyWeights returns the configured journey selection weights.
func (h *TopologyHandle) JourneyWeights() map[string]float64 {
	if h.engine == nil {
		return map[string]float64{}
	}
	return h.engine.JourneyWeights()
}

// SetFaultIntensity scales injected-fault probability and error-rate overrides.
// Use setFaultIntensity(x) for the VU's global intensity or
// setFaultIntensity(target, x) to override one topology fault target.
func (h *TopologyHandle) SetFaultIntensity(args ...any) {
	if h.engine == nil {
		throwJSException(h.runtime, &ConfigError{Kind: "not_loaded"})
	}
	switch len(args) {
	case 1:
		x, err := faultIntensityNumber(args[0])
		if err != nil {
			throwJSException(h.runtime, &ConfigError{Kind: "invalid_fault_intensity", Inner: err})
		}
		h.engine.SetFaultIntensity(x)
	case 2:
		target, err := faultIntensityTarget(args[0])
		if err != nil {
			throwJSException(h.runtime, &ConfigError{Kind: "invalid_fault_intensity", Inner: err})
		}
		x, err := faultIntensityNumber(args[1])
		if err != nil {
			throwJSException(h.runtime, &ConfigError{Kind: "invalid_fault_intensity", Inner: err})
		}
		if err := h.engine.SetFaultTargetIntensity(target, x); err != nil {
			throwJSException(h.runtime, err)
		}
	default:
		throwJSException(h.runtime, &ConfigError{Kind: "invalid_fault_intensity", Inner: fmt.Errorf("want setFaultIntensity(x) or setFaultIntensity(target, x), got %d arguments", len(args))})
	}
}

func faultIntensityTarget(v any) (string, error) {
	if value, ok := v.(sobek.Value); ok {
		return faultIntensityTarget(value.Export())
	}
	target, ok := v.(string)
	if !ok || target == "" {
		return "", fmt.Errorf("target must be a non-empty string")
	}
	return target, nil
}

func faultIntensityNumber(v any) (float64, error) {
	if value, ok := v.(sobek.Value); ok {
		return faultIntensityNumber(value.Export())
	}
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int8:
		return float64(n), nil
	case int16:
		return float64(n), nil
	case int32:
		return float64(n), nil
	case int64:
		return float64(n), nil
	case uint:
		return float64(n), nil
	case uint8:
		return float64(n), nil
	case uint16:
		return float64(n), nil
	case uint32:
		return float64(n), nil
	case uint64:
		return float64(n), nil
	default:
		return 0, fmt.Errorf("intensity must be numeric")
	}
}
