// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package generators

import (
	"fmt"
	"strings"

	"pgregory.net/rapid"
)

// ConfigureOptsOption mutates k6otelgen configure option generation.
type ConfigureOptsOption interface {
	applyConfigureOptsOption(*configureOptsOptions)
}

type configureOptsOptions struct{}

func applyConfigureOptsOptions(opts []ConfigureOptsOption) configureOptsOptions {
	o := configureOptsOptions{}
	for _, opt := range opts {
		opt.applyConfigureOptsOption(&o)
	}
	return o
}

// LoadPathOption mutates k6otelgen load path generation.
type LoadPathOption interface {
	applyLoadPathOption(*loadPathOptions)
}

type loadPathOptions struct{}

func applyLoadPathOptions(opts []LoadPathOption) loadPathOptions {
	o := loadPathOptions{}
	for _, opt := range opts {
		opt.applyLoadPathOption(&o)
	}
	return o
}

// ValidConfigureOpts returns JS-friendly configure options accepted by U5.
func ValidConfigureOpts(opts ...ConfigureOptsOption) *rapid.Generator[map[string]any] {
	return rapid.Custom(func(t *rapid.T) map[string]any {
		_ = applyConfigureOptsOptions(opts)

		batchSize := rapid.IntRange(1, 1024).Draw(t, "batch_size")
		maxQueueSize := rapid.IntRange(batchSize, batchSize*4).Draw(t, "max_queue_size")
		timeoutMS := rapid.IntRange(1, 30_000).Draw(t, "timeout_ms")
		batchTimeoutMS := rapid.IntRange(1, 30_000).Draw(t, "batch_timeout_ms")

		timeout := any(timeoutMS)
		if rapid.Bool().Draw(t, "timeout_as_string") {
			timeout = fmt.Sprintf("%dms", timeoutMS)
		}

		return map[string]any{
			"endpoint":          validExporterEndpoint(t, "endpoint"),
			"protocol":          rapid.SampledFrom([]string{"grpc", "http"}).Draw(t, "protocol"),
			"insecure":          rapid.Bool().Draw(t, "insecure"),
			"headers":           stringAnyMap(validHeaderMap(t, "headers")),
			"compression":       rapid.SampledFrom([]string{"", "gzip"}).Draw(t, "compression"),
			"timeout":           timeout,
			"batchSize":         batchSize,
			"batchTimeout":      batchTimeoutMS,
			"maxQueueSize":      maxQueueSize,
			"resourceOverrides": stringAnyMap(validResourceOverrideMap(t, "resource_overrides")),
		}
	})
}

// AnyConfigureOpts returns configure options that may violate U5 decode rules.
func AnyConfigureOpts(opts ...ConfigureOptsOption) *rapid.Generator[map[string]any] {
	return rapid.Custom(func(t *rapid.T) map[string]any {
		_ = applyConfigureOptsOptions(opts)
		cfg := ValidConfigureOpts(opts...).Draw(t, "valid_configure_opts")
		switch rapid.IntRange(0, 9).Draw(t, "configure_mutation") {
		case 0:
			return cfg
		case 1:
			cfg["protocol"] = rapid.SampledFrom([]string{"http/protobuf", "udp", "grpc-web"}).Draw(t, "bad_protocol")
		case 2:
			cfg["endpoint"] = rapid.IntRange(-100, 100).Draw(t, "bad_endpoint")
		case 3:
			cfg["timeout"] = rapid.SampledFrom([]any{-1, "soon", false}).Draw(t, "bad_timeout")
		case 4:
			cfg["headers"] = rapid.SampledFrom([]any{"A=B", map[string]any{"bad": []string{"x"}}}).Draw(t, "bad_headers")
		case 5:
			cfg["batchSize"] = rapid.SampledFrom([]any{-1, 1.25, "64"}).Draw(t, "bad_batch_size")
		case 6:
			cfg["batchTimeout"] = rapid.SampledFrom([]any{-1, "later", []int{1}}).Draw(t, "bad_batch_timeout")
		case 7:
			cfg["maxQueueSize"] = rapid.SampledFrom([]any{-1, 1.5, "128"}).Draw(t, "bad_queue")
		case 8:
			cfg["resourceOverrides"] = map[string]any{"service.name": []string{"api"}}
		case 9:
			cfg["unknownFutureField"] = map[string]any{"ignored": true}
		}
		return cfg
	})
}

// ValidLoadPath returns a safe relative YAML path for otelgen.load tests.
func ValidLoadPath(opts ...LoadPathOption) *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		_ = applyLoadPathOptions(opts)
		name := rapid.StringMatching(`^[a-z][a-z0-9-]{0,20}$`).Draw(t, "load_path_name")
		return "topologies/" + name + ".yaml"
	})
}

// AnyLoadPath returns load paths that may include traversal or unusual input.
func AnyLoadPath(opts ...LoadPathOption) *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		_ = applyLoadPathOptions(opts)
		switch rapid.IntRange(0, 5).Draw(t, "load_path_kind") {
		case 0:
			return ValidLoadPath(opts...).Draw(t, "valid_load_path")
		case 1:
			return "../" + ValidLoadPath(opts...).Draw(t, "traversal_path")
		case 2:
			return "/tmp/" + ValidLoadPath(opts...).Draw(t, "absolute_path")
		case 3:
			return "topologies/ユニコード.yaml"
		case 4:
			return strings.Repeat("a", rapid.IntRange(256, 1024).Draw(t, "long_path_len")) + ".yaml"
		default:
			return ""
		}
	})
}

func stringAnyMap(in map[string]string) map[string]any {
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
