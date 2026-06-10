package journey

import (
	"fmt"
	"math/rand/v2"
	"sort"
	"sync"
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
	schema      *topology.Schema
	overlay     *topology.FaultOverlay
	synth       synth.Synthesizer
	plans       map[string]*Plan
	journeyKeys []string
	rand        *rand.Rand
	rmu         sync.Mutex
}

// NewEngine constructs an Engine and eagerly builds all journey plans.
func NewEngine(schema *topology.Schema, overlay *topology.FaultOverlay, syn synth.Synthesizer) *Engine {
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
		rand:    newDefaultRand(),
	}
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

func newDefaultRand() *rand.Rand {
	seed := uint64(time.Now().UnixNano())
	return rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
}
