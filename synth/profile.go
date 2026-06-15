// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package synth

import (
	"bytes"
	"context"
	"fmt"

	"github.com/google/pprof/profile"
)

func (s *defaultSynthesizer) EmitProfile(ctx context.Context, in ProfileInput) {
	if s.profiles == nil || len(in.Stacks) == 0 || in.Service == nil {
		return
	}
	if in.SampleRateHz <= 0 {
		in.SampleRateHz = 100
	}

	pprofBytes, err := buildPprofProfile(in)
	if err != nil {
		return
	}

	labels := map[string]string{
		"span_id":      in.ProfileID,
		"service_name": string(in.Service.Name),
		"operation":    in.Operation,
	}
	_ = s.profiles.PushProfile(ctx, ProfilePush{
		AppName:    string(in.Service.Name),
		Labels:     labels,
		FromNanos:  in.StartTime.UnixNano(),
		UntilNanos: in.EndTime.UnixNano(),
		SampleRate: in.SampleRateHz,
		Pprof:      pprofBytes,
	})
}

func buildPprofProfile(in ProfileInput) ([]byte, error) {
	period := int64(1_000_000_000 / in.SampleRateHz)
	if period <= 0 {
		period = 10_000_000
	}
	duration := in.EndTime.Sub(in.StartTime).Nanoseconds()
	if duration < 0 {
		duration = 0
	}

	prof := &profile.Profile{
		SampleType:    []*profile.ValueType{{Type: "cpu", Unit: "nanoseconds"}},
		PeriodType:    &profile.ValueType{Type: "cpu", Unit: "nanoseconds"},
		Period:        period,
		TimeNanos:     in.StartTime.UnixNano(),
		DurationNanos: duration,
	}

	locByName := make(map[string]*profile.Location)
	fnByName := make(map[string]*profile.Function)
	var nextFnID, nextLocID uint64 = 1, 1

	locationForFrame := func(name string) *profile.Location {
		if loc, ok := locByName[name]; ok {
			return loc
		}
		fn, ok := fnByName[name]
		if !ok {
			fnID := nextFnID
			nextFnID++
			fn = &profile.Function{ID: fnID, Name: name}
			prof.Function = append(prof.Function, fn)
			fnByName[name] = fn
		}

		locID := nextLocID
		nextLocID++
		loc := &profile.Location{
			ID:   locID,
			Line: []profile.Line{{Function: fn}},
		}
		prof.Location = append(prof.Location, loc)
		locByName[name] = loc
		return loc
	}

	for _, stack := range in.Stacks {
		sample := &profile.Sample{Value: []int64{int64(stack.Weight)}}
		for i := len(stack.Frames) - 1; i >= 0; i-- {
			sample.Location = append(sample.Location, locationForFrame(stack.Frames[i]))
		}
		prof.Sample = append(prof.Sample, sample)
	}

	if err := prof.CheckValid(); err != nil {
		return nil, fmt.Errorf("pprof valid: %w", err)
	}

	var buf bytes.Buffer
	if err := prof.Write(&buf); err != nil {
		return nil, fmt.Errorf("pprof write: %w", err)
	}
	return buf.Bytes(), nil
}
