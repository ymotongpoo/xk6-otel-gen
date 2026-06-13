// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package exporter

import "sync"

var (
	sharedMu       sync.Mutex
	sharedOnce     sync.Once
	sharedPipeline *Pipeline
	sharedInitErr  error
)

// GetShared returns the process-wide shared Pipeline, initializing it once.
func GetShared(factory func() (*Pipeline, error)) (*Pipeline, error) {
	sharedMu.Lock()
	defer sharedMu.Unlock()

	sharedOnce.Do(func() {
		if factory == nil {
			sharedInitErr = &SharedError{Reason: "not_set"}
			return
		}
		pipeline, err := factory()
		if err != nil {
			sharedInitErr = &SharedError{Reason: "init_failed", Inner: err}
			return
		}
		if pipeline == nil {
			sharedInitErr = &SharedError{Reason: "not_set"}
			return
		}
		sharedPipeline = pipeline
	})
	return sharedPipeline, sharedInitErr
}

// SetShared installs p as the process-wide shared Pipeline before initialization.
func SetShared(p *Pipeline) error {
	if p == nil {
		return &SharedError{Reason: "not_set"}
	}

	sharedMu.Lock()
	defer sharedMu.Unlock()

	var set bool
	sharedOnce.Do(func() {
		sharedPipeline = p
		set = true
	})
	if !set {
		return &SharedError{Reason: "already_initialized", Inner: sharedInitErr}
	}
	return nil
}

// ResetShared resets the shared Pipeline holder. Intended for tests only.
//
// Goroutines holding the previous Pipeline remain responsible for its Shutdown.
func ResetShared() {
	sharedMu.Lock()
	defer sharedMu.Unlock()

	sharedOnce = sync.Once{}
	sharedPipeline = nil
	sharedInitErr = nil
}
