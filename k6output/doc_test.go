// SPDX-License-Identifier: Apache-2.0

package k6output_test

import (
	"fmt"
	"log"

	"go.k6.io/k6/output"

	"github.com/ymotongpoo/xk6-otel-gen/k6output"
)

func ExampleNew() {
	// k6 calls New internally when --out otel-gen=... is configured.
	params := output.Params{ConfigArgument: "endpoint=localhost:4317,protocol=grpc"}
	out, err := k6output.New(params)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(out.Description() != "")
	// Output:
	// true
}
