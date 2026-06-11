// SPDX-License-Identifier: Apache-2.0

package exporter

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel/attribute"
	sdkresource "go.opentelemetry.io/otel/sdk/resource"
)

// buildResource detects host/process/OS attributes and merges Config overrides.
func buildResource(ctx context.Context, cfg Config) (*sdkresource.Resource, error) {
	detected, err := sdkresource.New(ctx,
		sdkresource.WithFromEnv(),
		sdkresource.WithHost(),
		sdkresource.WithProcess(),
		sdkresource.WithOS(),
	)
	if err != nil {
		return nil, fmt.Errorf("resource: auto-detect: %w", err)
	}

	if len(cfg.ResourceOverrides) == 0 {
		return detected, nil
	}

	attrs := make([]attribute.KeyValue, 0, len(cfg.ResourceOverrides))
	for key, value := range cfg.ResourceOverrides {
		attrs = append(attrs, attribute.String(key, value))
	}
	merged, err := sdkresource.Merge(detected, sdkresource.NewSchemaless(attrs...))
	if err != nil {
		return nil, fmt.Errorf("resource: merge: %w", err)
	}
	return merged, nil
}
