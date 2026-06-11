// SPDX-License-Identifier: Apache-2.0

package k6otelgen_test

import "github.com/ymotongpoo/xk6-otel-gen/k6otelgen"

func ExampleNew() {
	rm := k6otelgen.New()
	_ = rm
	// Output:
}

func ExampleRootModule_NewModuleInstance() {
	// NewModuleInstance is called by the k6 module system with a modules.VU:
	//   rm := k6otelgen.New()
	//   inst := rm.NewModuleInstance(vu)
}
