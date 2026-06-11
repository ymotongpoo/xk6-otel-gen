// SPDX-License-Identifier: Apache-2.0

package topology_test

import (
	"fmt"
	"strings"

	"github.com/ymotongpoo/xk6-otel-gen/topology"
	"gopkg.in/yaml.v3"
)

func ExampleParse() {
	s, err := topology.Parse(strings.NewReader(`
services:
  frontend:
    kind: application
    operations:
      - name: GET /
journeys:
  home:
    steps:
      - service: frontend
        operation: GET /
`))
	if err != nil {
		panic(err)
	}
	fmt.Println(s.Services["frontend"].Name)
	// Output: frontend
}

func ExampleSchema_MarshalYAML() {
	s, err := topology.Parse(strings.NewReader(`
services:
  frontend:
    kind: application
    operations:
      - name: GET /
journeys:
  home:
    steps:
      - service: frontend
        operation: GET /
`))
	if err != nil {
		panic(err)
	}
	data, err := yaml.Marshal(s)
	if err != nil {
		panic(err)
	}
	roundTrip, err := topology.Parse(strings.NewReader(string(data)))
	if err != nil {
		panic(err)
	}
	fmt.Println(topology.Equal(s, roundTrip))
	// Output: true
}

func ExampleLint() {
	issues, err := topology.Lint(strings.NewReader(`
services:
  frontend:
    kind: application
    extra: ignored
    operations:
      - name: GET /
journeys:
  home:
    steps:
      - service: frontend
        operation: GET /
`))
	if err != nil {
		panic(err)
	}
	fmt.Println(issues[0].Severity, issues[0].Path)
	// Output: warning <root>.extra
}
