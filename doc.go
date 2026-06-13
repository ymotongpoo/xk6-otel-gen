// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

// Package xk6otelgen registers the k6/x/otel-gen JavaScript module and the
// otel-gen k6 output extension for xk6 builds.
package xk6otelgen

import (
	_ "github.com/ymotongpoo/xk6-otel-gen/k6otelgen"
	_ "github.com/ymotongpoo/xk6-otel-gen/k6output"
)
