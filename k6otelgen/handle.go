package k6otelgen

import (
	"context"

	"github.com/grafana/sobek"

	"github.com/ymotongpoo/xk6-otel-gen/journey"
)

type vuContextProvider interface {
	vuContext() context.Context
}

// TopologyHandle is the JS-visible object returned by load.
type TopologyHandle struct {
	runtime  *sobek.Runtime
	engine   *journey.Engine
	module   any
	instance vuContextProvider
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
}

// Journeys returns the sorted journey names available through this handle.
func (h *TopologyHandle) Journeys() []string {
	if h.engine == nil {
		return []string{}
	}
	return h.engine.ListJourneys()
}
