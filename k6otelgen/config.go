// SPDX-License-Identifier: Apache-2.0

package k6otelgen

import (
	"fmt"
	"math"
	"strconv"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
)

func optsToConfig(opts map[string]any) (exporter.Config, error) {
	var cfg exporter.Config
	for key, value := range opts {
		switch key {
		case "endpoint":
			s, ok := value.(string)
			if !ok {
				return cfg, typeMismatch(key, value, "string")
			}
			cfg.Endpoint = s
		case "protocol":
			s, ok := value.(string)
			if !ok {
				return cfg, typeMismatch(key, value, "string")
			}
			switch s {
			case "grpc":
				cfg.Protocol = exporter.ProtocolGRPC
			case "http":
				cfg.Protocol = exporter.ProtocolHTTP
			default:
				return cfg, &ConfigError{Kind: "invalid_protocol", Path: s}
			}
		case "insecure":
			b, ok := value.(bool)
			if !ok {
				return cfg, typeMismatch(key, value, "bool")
			}
			cfg.Insecure = b
			cfg.InsecureSet = true
		case "caCert":
			s, ok := value.(string)
			if !ok {
				return cfg, typeMismatch(key, value, "string")
			}
			cfg.Certificate = s
		case "clientCert":
			s, ok := value.(string)
			if !ok {
				return cfg, typeMismatch(key, value, "string")
			}
			cfg.ClientCertificate = s
		case "clientKey":
			s, ok := value.(string)
			if !ok {
				return cfg, typeMismatch(key, value, "string")
			}
			cfg.ClientKey = s
		case "headers":
			headers, err := toStringMap(value)
			if err != nil {
				return cfg, &ConfigError{Kind: "type_mismatch", Path: key, Inner: err}
			}
			cfg.Headers = headers
		case "compression":
			s, ok := value.(string)
			if !ok {
				return cfg, typeMismatch(key, value, "string")
			}
			cfg.Compression = s
		case "timeout":
			timeout, err := toDuration(value)
			if err != nil {
				return cfg, &ConfigError{Kind: "type_mismatch", Path: key, Inner: err}
			}
			cfg.Timeout = timeout
		case "batchSize":
			size, err := toInt(value)
			if err != nil {
				return cfg, &ConfigError{Kind: "type_mismatch", Path: key, Inner: err}
			}
			cfg.BatchSize = size
		case "batchTimeout":
			timeout, err := toDuration(value)
			if err != nil {
				return cfg, &ConfigError{Kind: "type_mismatch", Path: key, Inner: err}
			}
			cfg.BatchTimeout = timeout
		case "maxQueueSize":
			size, err := toInt(value)
			if err != nil {
				return cfg, &ConfigError{Kind: "type_mismatch", Path: key, Inner: err}
			}
			cfg.MaxQueueSize = size
		case "resourceOverrides":
			overrides, err := toStringMap(value)
			if err != nil {
				return cfg, &ConfigError{Kind: "type_mismatch", Path: key, Inner: err}
			}
			cfg.ResourceOverrides = overrides
		case "sampler":
			s, ok := value.(string)
			if !ok {
				return cfg, typeMismatch(key, value, "string")
			}
			switch s {
			case "always_on", "always_off", "traceidratio":
				cfg.Sampler = s
			default:
				return cfg, &ConfigError{Kind: "invalid_sampler", Path: s}
			}
		case "samplerArg":
			arg, err := toFloat64(value)
			if err != nil {
				return cfg, &ConfigError{Kind: "type_mismatch", Path: key, Inner: err}
			}
			cfg.SamplerArg = arg
			cfg.SamplerArgSet = true
		default:
			// Unknown keys are ignored for forward-compatible JS opts.
		}
	}
	if cfg.SamplerArgSet && (cfg.SamplerArg < 0 || cfg.SamplerArg > 1) {
		return cfg, &ConfigError{Kind: "invalid_sampler_arg", Path: strconv.FormatFloat(cfg.SamplerArg, 'f', -1, 64)}
	}
	return cfg, nil
}

func toFloat64(v any) (float64, error) {
	switch x := v.(type) {
	case int:
		return float64(x), nil
	case int64:
		return float64(x), nil
	case float64:
		return x, nil
	default:
		return 0, fmt.Errorf("expected number, got %T", v)
	}
}

func toDuration(v any) (time.Duration, error) {
	switch x := v.(type) {
	case int:
		return time.Duration(x) * time.Millisecond, nil
	case int64:
		return time.Duration(x) * time.Millisecond, nil
	case float64:
		return time.Duration(x * float64(time.Millisecond)), nil
	case string:
		return time.ParseDuration(x)
	default:
		return 0, fmt.Errorf("expected number or duration string, got %T", v)
	}
}

func toStringMap(v any) (map[string]string, error) {
	raw, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected object, got %T", v)
	}
	out := make(map[string]string, len(raw))
	for key, value := range raw {
		switch x := value.(type) {
		case string:
			out[key] = x
		case int:
			out[key] = strconv.Itoa(x)
		case int64:
			out[key] = strconv.FormatInt(x, 10)
		case float64:
			out[key] = strconv.FormatFloat(x, 'f', -1, 64)
		default:
			return nil, fmt.Errorf("value for %q is %T, not string-coercible", key, value)
		}
	}
	return out, nil
}

func toInt(v any) (int, error) {
	maxInt := int(^uint(0) >> 1)
	minInt := -maxInt - 1

	switch x := v.(type) {
	case int:
		return x, nil
	case int64:
		if x > int64(maxInt) || x < int64(minInt) {
			return 0, fmt.Errorf("integer %d overflows int", x)
		}
		return int(x), nil
	case float64:
		if math.Trunc(x) != x {
			return 0, fmt.Errorf("number %v is not an integer", x)
		}
		if x > float64(maxInt) || x < float64(minInt) {
			return 0, fmt.Errorf("number %v overflows int", x)
		}
		return int(x), nil
	default:
		return 0, fmt.Errorf("expected integer number, got %T", v)
	}
}

func typeMismatch(path string, value any, want string) *ConfigError {
	return &ConfigError{
		Kind:  "type_mismatch",
		Path:  path,
		Inner: fmt.Errorf("expected %s, got %T", want, value),
	}
}
