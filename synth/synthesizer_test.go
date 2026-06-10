package synth

import "testing"

func TestNewDefault_NilProvider_Panics(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, _, _ := newTestProviders(t)
	tests := []struct {
		name string
		run  func()
	}{
		{name: "trace provider", run: func() { NewDefault(nil, mp, lp) }},
		{name: "meter provider", run: func() { NewDefault(tp, nil, lp) }},
		{name: "logger provider", run: func() { NewDefault(tp, mp, nil) }},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			requirePanic(t, tt.run)
		})
	}
}

func TestNewDefault_BuildsAllInstruments(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, _, _ := newTestProviders(t)
	syn := NewDefault(tp, mp, lp)
	s, ok := syn.(*defaultSynthesizer)
	if !ok {
		t.Fatalf("NewDefault returned %T, want *defaultSynthesizer", syn)
	}

	if s.tracer == nil {
		t.Fatal("tracer is nil")
	}
	if s.meter == nil {
		t.Fatal("meter is nil")
	}
	if s.logger == nil {
		t.Fatal("logger is nil")
	}
	if s.httpClientDur == nil {
		t.Fatal("httpClientDur is nil")
	}
	if s.httpServerDur == nil {
		t.Fatal("httpServerDur is nil")
	}
	if s.httpActiveReq == nil {
		t.Fatal("httpActiveReq is nil")
	}
	if s.rpcClientDur == nil {
		t.Fatal("rpcClientDur is nil")
	}
	if s.rpcServerDur == nil {
		t.Fatal("rpcServerDur is nil")
	}
	if s.rpcActiveReq == nil {
		t.Fatal("rpcActiveReq is nil")
	}
	if s.dbClientDur == nil {
		t.Fatal("dbClientDur is nil")
	}
	if s.msgProducerDur == nil {
		t.Fatal("msgProducerDur is nil")
	}
	if s.msgConsumerDur == nil {
		t.Fatal("msgConsumerDur is nil")
	}
	if s.staticSetCache == nil {
		t.Fatal("staticSetCache is nil")
	}
}
