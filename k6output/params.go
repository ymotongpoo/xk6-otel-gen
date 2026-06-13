// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package k6output

import (
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/exporter"
)

const (
	defaultQueueSize     = 100
	defaultFlushInterval = time.Second
	minQueueSize         = 10
	maxQueueSize         = 10_000
)

// Params is the parsed --out args representation used by the otel-gen output.
type Params struct {
	Endpoint        string
	MetricsEndpoint string
	Protocol        exporter.Protocol
	Insecure        bool
	Certificate     string
	ClientCert      string
	ClientKey       string
	Headers         map[string]string
	Compression     string
	Timeout         time.Duration
	BatchSize       int
	BatchTimeout    time.Duration
	MaxQueueSize    int

	QueueSize     int
	FlushInterval time.Duration
	ScriptPath    string

	provided map[string]struct{}
}

func defaultParams() Params {
	return Params{
		Protocol:      exporter.ProtocolGRPC,
		Endpoint:      "localhost:4317",
		Timeout:       10 * time.Second,
		BatchSize:     512,
		BatchTimeout:  time.Second,
		MaxQueueSize:  2048,
		QueueSize:     defaultQueueSize,
		FlushInterval: defaultFlushInterval,
		provided:      map[string]struct{}{},
	}
}

func parseOutArgs(s string) (Params, error) {
	p := defaultParams()
	if strings.TrimSpace(s) == "" {
		return p, nil
	}
	for _, token := range strings.Split(s, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		key, val, ok := strings.Cut(token, "=")
		if !ok {
			return p, &ConfigError{Kind: ConfigErrorKindInvalidArgs, Field: token, Value: "(missing =)"}
		}
		if err := applyKV(&p, strings.TrimSpace(key), strings.TrimSpace(val)); err != nil {
			return p, err
		}
	}
	return p, nil
}

func applyKV(p *Params, key, val string) error {
	switch key {
	case "endpoint":
		if !validEndpointArg(val) {
			return &ConfigError{Kind: ConfigErrorKindInvalidURL, Field: key, Value: val}
		}
		p.Endpoint = val
	case "metricsEndpoint":
		if !validEndpointArg(val) {
			return &ConfigError{Kind: ConfigErrorKindInvalidURL, Field: key, Value: val}
		}
		p.MetricsEndpoint = val
	case "protocol":
		switch strings.ToLower(val) {
		case "grpc":
			p.Protocol = exporter.ProtocolGRPC
		case "http":
			p.Protocol = exporter.ProtocolHTTP
		default:
			return &ConfigError{Kind: ConfigErrorKindInvalidProtocol, Field: key, Value: val}
		}
	case "insecure":
		b, err := strconv.ParseBool(val)
		if err != nil {
			return &ConfigError{Kind: ConfigErrorKindTypeMismatch, Field: key, Value: val, Inner: err}
		}
		p.Insecure = b
	case "caCert":
		p.Certificate = val
	case "clientCert":
		p.ClientCert = val
	case "clientKey":
		p.ClientKey = val
	case "headers":
		headers, err := parseHeaders(val)
		if err != nil {
			return &ConfigError{Kind: ConfigErrorKindTypeMismatch, Field: key, Value: val, Inner: err}
		}
		p.Headers = headers
	case "compression":
		if val != "" && val != "gzip" {
			return &ConfigError{Kind: ConfigErrorKindInvalidArgs, Field: key, Value: val}
		}
		p.Compression = val
	case "timeout":
		d, err := time.ParseDuration(val)
		if err != nil {
			return &ConfigError{Kind: ConfigErrorKindTypeMismatch, Field: key, Value: val, Inner: err}
		}
		p.Timeout = d
	case "batchSize":
		n, err := parseIntField(key, val)
		if err != nil {
			return err
		}
		p.BatchSize = n
	case "batchTimeout":
		d, err := time.ParseDuration(val)
		if err != nil {
			return &ConfigError{Kind: ConfigErrorKindTypeMismatch, Field: key, Value: val, Inner: err}
		}
		p.BatchTimeout = d
	case "maxQueueSize":
		n, err := parseIntField(key, val)
		if err != nil {
			return err
		}
		p.MaxQueueSize = n
	case "queueSize":
		n, err := parseIntField(key, val)
		if err != nil {
			return err
		}
		if n < minQueueSize || n > maxQueueSize {
			return &ConfigError{Kind: ConfigErrorKindInvalidArgs, Field: key, Value: val}
		}
		p.QueueSize = n
	default:
		return nil
	}
	p.markProvided(key)
	return nil
}

func parseHeaders(s string) (map[string]string, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	out := make(map[string]string)
	for _, pair := range strings.Split(s, ";") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		key, val, ok := strings.Cut(pair, ":")
		if !ok {
			return nil, fmt.Errorf("invalid header pair %q (expected key:value)", pair)
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if key == "" || val == "" {
			return nil, fmt.Errorf("invalid header pair %q (key and value must be non-empty)", pair)
		}
		out[key] = val
	}
	return out, nil
}

func parseIntField(key, val string) (int, error) {
	n, err := strconv.Atoi(val)
	if err != nil {
		return 0, &ConfigError{Kind: ConfigErrorKindTypeMismatch, Field: key, Value: val, Inner: err}
	}
	return n, nil
}

func validEndpointArg(endpoint string) bool {
	if endpoint == "" {
		return false
	}
	if strings.Contains(endpoint, "://") {
		u, err := url.Parse(endpoint)
		return err == nil && u.Scheme != "" && u.Host != ""
	}
	host, port, err := net.SplitHostPort(endpoint)
	return err == nil && host != "" && port != ""
}

func (p *Params) markProvided(key string) {
	if p.provided == nil {
		p.provided = map[string]struct{}{}
	}
	p.provided[key] = struct{}{}
}

func (p Params) wasProvided(key string) bool {
	_, ok := p.provided[key]
	return ok
}

func (p Params) exporterConfig() exporter.Config {
	var cfg exporter.Config
	if p.wasProvided("endpoint") {
		cfg.Endpoint = p.Endpoint
	}
	if p.wasProvided("metricsEndpoint") {
		cfg.MetricsEndpoint = p.MetricsEndpoint
	}
	if p.wasProvided("protocol") {
		cfg.Protocol = p.Protocol
	}
	if p.wasProvided("insecure") {
		cfg.Insecure = p.Insecure
		cfg.InsecureSet = true
	}
	if p.wasProvided("caCert") {
		cfg.Certificate = p.Certificate
	}
	if p.wasProvided("clientCert") {
		cfg.ClientCertificate = p.ClientCert
	}
	if p.wasProvided("clientKey") {
		cfg.ClientKey = p.ClientKey
	}
	if p.wasProvided("headers") {
		cfg.Headers = copyStringMap(p.Headers)
	}
	if p.wasProvided("compression") {
		cfg.Compression = p.Compression
	}
	if p.wasProvided("timeout") {
		cfg.Timeout = p.Timeout
	}
	if p.wasProvided("batchSize") {
		cfg.BatchSize = p.BatchSize
	}
	if p.wasProvided("batchTimeout") {
		cfg.BatchTimeout = p.BatchTimeout
	}
	if p.wasProvided("maxQueueSize") {
		cfg.MaxQueueSize = p.MaxQueueSize
	}
	return cfg
}

func copyStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
