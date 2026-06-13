// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package journey

import (
	"math/rand/v2"
	"time"
)

func newDefaultRand() *rand.Rand {
	seed := uint64(time.Now().UnixNano())
	return newRandWithSeed(seed)
}

func newRandWithSeed(seed uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed, seed^0xdeadbeefcafebabe))
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
