// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package k6otelgen

import (
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
