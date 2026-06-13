// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package generators

import "github.com/ymotongpoo/xk6-otel-gen/topology"

// SchemaOption mutates schema-generation parameters.
type SchemaOption func(*schemaOptions)

// ServiceOption mutates service-generation parameters.
type ServiceOption = SchemaOption

type schemaOptions struct {
	maxServices      int
	maxOpsPerService int
	maxCallsPerOp    int
	maxFaults        int
	biasValid        float64
	fixedKind        *topology.ServiceKind
}

type serviceOptions = schemaOptions

func defaultSchemaOptions() schemaOptions {
	return schemaOptions{
		maxServices:      10,
		maxOpsPerService: 5,
		maxCallsPerOp:    5,
		maxFaults:        3,
		biasValid:        0.5,
	}
}

func defaultServiceOptions() serviceOptions {
	return serviceOptions{
		maxOpsPerService: 5,
	}
}

func applySchemaOptions(opts []SchemaOption) schemaOptions {
	o := defaultSchemaOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

func applyServiceOptions(opts []ServiceOption) serviceOptions {
	o := defaultServiceOptions()
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// MaxServices caps the number of services in the generated schema.
func MaxServices(n int) SchemaOption {
	return func(o *schemaOptions) {
		o.maxServices = clampInt(n, 1, n)
	}
}

// MaxOpsPerService caps the number of operations per generated service.
func MaxOpsPerService(n int) SchemaOption {
	return func(o *schemaOptions) {
		o.maxOpsPerService = clampInt(n, 1, n)
	}
}

// MaxCallsPerOp caps the number of outgoing calls per operation.
func MaxCallsPerOp(n int) SchemaOption {
	return func(o *schemaOptions) {
		o.maxCallsPerOp = clampInt(n, 0, n)
	}
}

// MaxFaults caps the number of generated fault specifications.
func MaxFaults(n int) SchemaOption {
	return func(o *schemaOptions) {
		o.maxFaults = clampInt(n, 0, n)
	}
}

// BiasValid sets the probability that AnySchema returns a valid schema.
func BiasValid(p float64) SchemaOption {
	return func(o *schemaOptions) {
		switch {
		case p < 0:
			o.biasValid = 0
		case p > 1:
			o.biasValid = 1
		default:
			o.biasValid = p
		}
	}
}

// WithKind fixes the ServiceKind used by ValidService and AnyService.
func WithKind(k topology.ServiceKind) ServiceOption {
	return func(o *schemaOptions) {
		o.fixedKind = &k
	}
}

func clampInt(n, min, max int) int {
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}
