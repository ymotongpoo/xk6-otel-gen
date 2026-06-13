// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package k6otelgen

import "testing"

func TestNew_ReturnsZeroState(t *testing.T) {
	t.Parallel()

	root := New()
	if root == nil {
		t.Fatal("New() = nil")
	}
	if root.schema != nil || root.overlay != nil || root.loadedPath != "" || root.configured || root.handle != nil {
		t.Fatalf("New() returned non-zero root: %#v", root)
	}
}

func TestNewModuleInstance_BeforeLoad_PartialInstance(t *testing.T) {
	t.Parallel()

	root := newTestRootModule(t)
	vu := newFakeVU(t, 1)
	instance, ok := root.NewModuleInstance(vu).(*ModuleInstance)
	if !ok {
		t.Fatalf("NewModuleInstance() type = %T, want *ModuleInstance", root.NewModuleInstance(vu))
	}
	if instance.root != root || instance.vu != vu {
		t.Fatalf("NewModuleInstance() root/vu not wired: %#v", instance)
	}
	if instance.engine != nil || instance.synth != nil || instance.handle != nil || instance.initErr != nil {
		t.Fatalf("partial instance initialized early: %#v", instance)
	}
}

func TestNewModuleInstance_AfterLoad_BuildsEngine(t *testing.T) {
	t.Parallel()

	root := newTestRootModule(t)
	root.schema = testModuleSchema()
	root.overlay = root.schema.ApplyFaults()
	root.loadedPath = "topology.yaml"

	instance := root.NewModuleInstance(newFakeVU(t, 7)).(*ModuleInstance)
	if instance.initErr != nil {
		t.Fatalf("initErr = %v, want nil", instance.initErr)
	}
	if instance.engine == nil || instance.synth == nil || instance.handle == nil {
		t.Fatalf("NewModuleInstance() did not initialize per-VU state: %#v", instance)
	}
	if instance.handle.instance != instance || instance.handle.module != root || instance.handle.name != "topology.yaml" {
		t.Fatalf("handle not wired to instance/root/path: %#v", instance.handle)
	}
}
