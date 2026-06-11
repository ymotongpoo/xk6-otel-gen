// SPDX-License-Identifier: Apache-2.0

package exporter_test

import (
	"errors"
	"testing"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
)

func TestGetShared_CachesSuccess(t *testing.T) {
	freshShared(t)

	want := &exporter.Pipeline{}
	calls := 0
	got, err := exporter.GetShared(func() (*exporter.Pipeline, error) {
		calls++
		return want, nil
	})
	if err != nil {
		t.Fatalf("GetShared() error = %v, want nil", err)
	}
	again, err := exporter.GetShared(func() (*exporter.Pipeline, error) {
		calls++
		return &exporter.Pipeline{}, nil
	})
	if err != nil {
		t.Fatalf("second GetShared() error = %v, want nil", err)
	}
	if got != want || again != want {
		t.Fatalf("shared pointers = %p and %p, want %p", got, again, want)
	}
	if calls != 1 {
		t.Fatalf("factory calls = %d, want 1", calls)
	}
}

func TestGetShared_CachesError(t *testing.T) {
	freshShared(t)

	sentinel := errors.New("factory failed")
	calls := 0
	got, err := exporter.GetShared(func() (*exporter.Pipeline, error) {
		calls++
		return nil, sentinel
	})
	if got != nil {
		t.Fatalf("GetShared() pipeline = %v, want nil", got)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("GetShared() error = %v, want %v", err, sentinel)
	}
	_, again := exporter.GetShared(func() (*exporter.Pipeline, error) {
		calls++
		return &exporter.Pipeline{}, nil
	})
	if again != err {
		t.Fatalf("second error = %v, want same cached error %v", again, err)
	}
	if calls != 1 {
		t.Fatalf("factory calls = %d, want 1", calls)
	}
}

func TestSetShared_BeforeAnyGet(t *testing.T) {
	freshShared(t)

	want := &exporter.Pipeline{}
	if err := exporter.SetShared(want); err != nil {
		t.Fatalf("SetShared() error = %v, want nil", err)
	}
	got, err := exporter.GetShared(func() (*exporter.Pipeline, error) {
		t.Fatal("factory should not be called after SetShared")
		return nil, nil
	})
	if err != nil {
		t.Fatalf("GetShared() error = %v, want nil", err)
	}
	if got != want {
		t.Fatalf("GetShared() = %p, want %p", got, want)
	}
}

func TestSetShared_AfterGet_Fails(t *testing.T) {
	freshShared(t)

	if _, err := exporter.GetShared(func() (*exporter.Pipeline, error) {
		return &exporter.Pipeline{}, nil
	}); err != nil {
		t.Fatalf("GetShared() error = %v, want nil", err)
	}

	err := exporter.SetShared(&exporter.Pipeline{})
	var sharedErr *exporter.SharedError
	if !errors.As(err, &sharedErr) {
		t.Fatalf("SetShared() error = %T, want *SharedError", err)
	}
	if sharedErr.Reason != "already_initialized" {
		t.Fatalf("SharedError.Reason = %q, want already_initialized", sharedErr.Reason)
	}
}

func TestSetShared_Nil_Fails(t *testing.T) {
	freshShared(t)

	err := exporter.SetShared(nil)
	var sharedErr *exporter.SharedError
	if !errors.As(err, &sharedErr) {
		t.Fatalf("SetShared(nil) error = %T, want *SharedError", err)
	}
	if sharedErr.Reason != "not_set" {
		t.Fatalf("SharedError.Reason = %q, want not_set", sharedErr.Reason)
	}
}

func freshShared(t *testing.T) {
	t.Helper()
	exporter.ResetShared()
	t.Cleanup(exporter.ResetShared)
}
