package journey

import (
	"sync"
	"testing"
)

func TestRandIntN_ZeroOrOne_ReturnsZero(t *testing.T) {
	t.Parallel()

	impl := &engineImpl{rand: newDefaultRand()}
	if got := impl.randIntN(0); got != 0 {
		t.Fatalf("randIntN(0) = %d, want 0", got)
	}
	if got := impl.randIntN(1); got != 0 {
		t.Fatalf("randIntN(1) = %d, want 0", got)
	}
}

func TestRandIntN_Range(t *testing.T) {
	t.Parallel()

	impl := &engineImpl{rand: newDefaultRand()}
	for i := 0; i < 1000; i++ {
		got := impl.randIntN(10)
		if got < 0 || got >= 10 {
			t.Fatalf("randIntN(10) = %d, want [0, 10)", got)
		}
	}
}

func TestRandFloat64_Range(t *testing.T) {
	t.Parallel()

	impl := &engineImpl{rand: newDefaultRand()}
	for i := 0; i < 1000; i++ {
		got := impl.randFloat64()
		if got < 0.0 || got >= 1.0 {
			t.Fatalf("randFloat64() = %f, want [0.0, 1.0)", got)
		}
	}
}

func TestRand_Concurrent_NoRace(t *testing.T) {
	t.Parallel()

	impl := &engineImpl{rand: newDefaultRand()}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				got := impl.randIntN(32)
				if got < 0 || got >= 32 {
					t.Errorf("randIntN(32) = %d, want [0, 32)", got)
				}
			}
		}()
	}
	wg.Wait()
}
