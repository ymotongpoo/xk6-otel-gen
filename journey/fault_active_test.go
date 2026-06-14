// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package journey

import (
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

func TestFaultActiveForKind(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ff   foldedFault
		kind topology.FaultKind
		want bool
	}{
		{
			name: "latency inflation active",
			ff:   foldedFault{latencyInflate: time.Millisecond},
			kind: topology.FaultLatencyInflation,
			want: true,
		},
		{
			name: "latency inflation inactive",
			ff:   foldedFault{},
			kind: topology.FaultLatencyInflation,
			want: false,
		},
		{
			name: "crash active",
			ff:   foldedFault{crashed: true},
			kind: topology.FaultCrash,
			want: true,
		},
		{
			name: "disconnect active",
			ff:   foldedFault{disconnected: true},
			kind: topology.FaultDisconnect,
			want: true,
		},
		{
			name: "error rate active",
			ff:   foldedFault{errorRate: 0.5},
			kind: topology.FaultErrorRateOverride,
			want: true,
		},
		{
			name: "unknown kind",
			ff:   foldedFault{crashed: true},
			kind: topology.FaultKind(-1),
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := faultActiveForKind(tt.ff, tt.kind); got != tt.want {
				t.Fatalf("faultActiveForKind() = %v, want %v", got, tt.want)
			}
		})
	}
}
