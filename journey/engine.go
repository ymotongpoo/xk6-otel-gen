// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package journey

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/synth"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

// Engine drives immutable journey Plans against a fixed topology and fault
// overlay. An Engine is safe for concurrent use after construction.
type Engine struct {
	impl *engineImpl
}

type engineImpl struct {
	schema         *topology.Schema
	overlay        *topology.FaultOverlay
	synth          synth.Synthesizer
	plans          map[string]*Plan
	journeyKeys    []string
	rand           *rand.Rand
	rmu            sync.Mutex
	faultIntensity atomic.Uint64
}

// NewEngine constructs an Engine and eagerly builds all journey plans.
func NewEngine(schema *topology.Schema, overlay *topology.FaultOverlay, syn synth.Synthesizer) *Engine {
	return NewEngineWithSeed(schema, overlay, syn, uint64(time.Now().UnixNano()))
}

// NewEngineWithSeed constructs an Engine with a deterministic random source
// derived from seed and eagerly builds all journey plans.
func NewEngineWithSeed(schema *topology.Schema, overlay *topology.FaultOverlay, syn synth.Synthesizer, seed uint64) *Engine {
	if schema == nil {
		panic("journey: NewEngine: schema must not be nil")
	}
	if overlay == nil {
		panic("journey: NewEngine: overlay must not be nil")
	}
	if syn == nil {
		panic("journey: NewEngine: synth must not be nil")
	}

	impl := &engineImpl{
		schema:  schema,
		overlay: overlay,
		synth:   syn,
		plans:   make(map[string]*Plan, len(schema.Journeys)),
		rand:    newRandWithSeed(seed),
	}
	impl.faultIntensity.Store(math.Float64bits(1.0))
	for name := range schema.Journeys {
		plan, err := impl.buildPlan(name)
		if err != nil {
			panic(fmt.Sprintf("journey: NewEngine: build %q: %v", name, err))
		}
		impl.plans[name] = plan
		impl.journeyKeys = append(impl.journeyKeys, name)
	}
	sort.Strings(impl.journeyKeys)

	return &Engine{impl: impl}
}

// ListJourneys returns sorted journey names known to the Engine.
func (e *Engine) ListJourneys() []string {
	keys := make([]string, len(e.impl.journeyKeys))
	copy(keys, e.impl.journeyKeys)
	return keys
}

// JourneyWeights returns a copy of the configured journey selection weights.
func (e *Engine) JourneyWeights() map[string]float64 {
	out := make(map[string]float64, len(e.impl.journeyKeys))
	for _, name := range e.impl.journeyKeys {
		if journey := e.impl.schema.Journeys[name]; journey != nil {
			out[name] = journey.Weight
		}
	}
	return out
}

// PickJourney returns one journey name selected according to configured
// weights using the Engine's deterministic random source.
func (e *Engine) PickJourney() string {
	if e == nil || e.impl == nil || len(e.impl.journeyKeys) == 0 {
		return ""
	}
	var total float64
	for _, name := range e.impl.journeyKeys {
		if journey := e.impl.schema.Journeys[name]; journey != nil && journey.Weight > 0 {
			total += journey.Weight
		}
	}
	if total <= 0 {
		return ""
	}
	roll := e.impl.randFloat64() * total
	var cumulative float64
	for _, name := range e.impl.journeyKeys {
		journey := e.impl.schema.Journeys[name]
		if journey == nil || journey.Weight <= 0 {
			continue
		}
		cumulative += journey.Weight
		if roll < cumulative {
			return name
		}
	}
	for idx := len(e.impl.journeyKeys) - 1; idx >= 0; idx-- {
		name := e.impl.journeyKeys[idx]
		if journey := e.impl.schema.Journeys[name]; journey != nil && journey.Weight > 0 {
			return name
		}
	}
	return ""
}

func (e *engineImpl) setFaultIntensity(x float64) {
	if x < 0 {
		x = 0
	}
	e.faultIntensity.Store(math.Float64bits(x))
}

func (e *engineImpl) faultIntensityValue() float64 {
	return math.Float64frombits(e.faultIntensity.Load())
}

// SetFaultIntensity scales injected-fault probability and error-rate overrides
// for this VU's engine. 0 disables injected faults, 1 is full intensity. Drive
// it from k6 stages to script a burn->recover timeline. Safe for concurrent
// use from parallel goroutines.
func (e *Engine) SetFaultIntensity(x float64) {
	e.impl.setFaultIntensity(x)
}
