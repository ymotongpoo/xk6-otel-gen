// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package synth

import (
	"context"
	"testing"
	"time"

	"github.com/google/pprof/profile"
	"github.com/ymotongpoo/xk6-otel-gen/topology"
)

type fakeProfileExporter struct {
	pushes []ProfilePush
}

func (f *fakeProfileExporter) PushProfile(_ context.Context, p ProfilePush) error {
	f.pushes = append(f.pushes, p)
	return nil
}

func TestEmitProfile_BuildsPprofAndPush(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, _, _ := newTestProviders(t)
	fake := &fakeProfileExporter{}
	syn := NewDefault(singleProviderFactory{tp: tp, lp: lp}, mp, fake)
	start := time.Unix(1_700_000_000, 0)
	end := start.Add(50 * time.Millisecond)
	syn.EmitProfile(context.Background(), ProfileInput{
		Service:      makeSpanService("shipping", topology.KindApplication),
		Operation:    "quote_shipping",
		InstanceIdx:  0,
		SampleRateHz: 100,
		Stacks: []topology.StackSample{
			{Frames: []string{"root", "leaf"}, Weight: 42},
		},
		StartTime: start,
		EndTime:   end,
		ProfileID: "abc123",
	})

	if len(fake.pushes) != 1 {
		t.Fatalf("pushes = %d, want 1", len(fake.pushes))
	}
	push := fake.pushes[0]
	if push.AppName != "shipping" {
		t.Fatalf("AppName = %q", push.AppName)
	}
	if push.Labels["span_id"] != "abc123" || push.Labels["operation"] != "quote_shipping" {
		t.Fatalf("Labels = %#v", push.Labels)
	}
	if push.FromNanos != start.UnixNano() || push.UntilNanos != end.UnixNano() {
		t.Fatalf("window = [%d,%d]", push.FromNanos, push.UntilNanos)
	}

	prof, err := profile.ParseData(push.Pprof)
	if err != nil {
		t.Fatalf("profile.ParseData() error = %v", err)
	}
	if len(prof.Sample) != 1 || prof.Sample[0].Value[0] != 42 {
		t.Fatalf("samples = %+v", prof.Sample)
	}
	if len(prof.Location) != 2 {
		t.Fatalf("locations = %d, want 2", len(prof.Location))
	}
	if prof.Period != 10_000_000 {
		t.Fatalf("Period = %d, want 10ms for 100Hz", prof.Period)
	}
}

func TestEmitProfile_NilExporter_NoOp(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, _, _ := newTestProviders(t)
	syn := NewDefault(singleProviderFactory{tp: tp, lp: lp}, mp, nil)
	syn.EmitProfile(context.Background(), ProfileInput{
		Service:   makeSpanService("shipping", topology.KindApplication),
		Operation: "quote_shipping",
		Stacks:    []topology.StackSample{{Frames: []string{"a"}, Weight: 1}},
	})
}

func TestEmitProfile_EmptyStacks_NoOp(t *testing.T) {
	t.Parallel()

	tp, mp, lp, _, _, _ := newTestProviders(t)
	fake := &fakeProfileExporter{}
	syn := NewDefault(singleProviderFactory{tp: tp, lp: lp}, mp, fake)
	syn.EmitProfile(context.Background(), ProfileInput{
		Service:   makeSpanService("shipping", topology.KindApplication),
		Operation: "quote_shipping",
	})
	if len(fake.pushes) != 0 {
		t.Fatalf("pushes = %d, want 0", len(fake.pushes))
	}
}
