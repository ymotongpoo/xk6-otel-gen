package journey

import (
	"math/rand/v2"
	"time"
)

func newDefaultRand() *rand.Rand {
	seed := uint64(time.Now().UnixNano())
	return rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
}

func (e *engineImpl) randIntN(n int) int {
	if n <= 1 {
		return 0
	}
	e.rmu.Lock()
	defer e.rmu.Unlock()
	return e.rand.IntN(n)
}

func (e *engineImpl) randFloat64() float64 {
	e.rmu.Lock()
	defer e.rmu.Unlock()
	return e.rand.Float64()
}
