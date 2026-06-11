// SPDX-License-Identifier: Apache-2.0

package exporter

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Protocol identifies the OTLP transport protocol used by a Pipeline.
type Protocol int

const (
	// ProtocolGRPC sends OTLP over gRPC.
	ProtocolGRPC Protocol = iota
	// ProtocolHTTP sends OTLP over HTTP/protobuf.
	ProtocolHTTP
)

const (
	defaultEndpoint     = "localhost:4317"
	defaultTimeout      = 10 * time.Second
	defaultBatchSize    = 512
	defaultBatchTimeout = time.Second
	defaultMaxQueueSize = 2048
)

const headerKeyPattern = `^[A-Za-z0-9_-]+$`

var defaultConfig = Config{
	Protocol:      ProtocolGRPC,
	Endpoint:      defaultEndpoint,
	Timeout:       defaultTimeout,
	BatchSize:     defaultBatchSize,
	BatchTimeout:  defaultBatchTimeout,
	MaxQueueSize:  defaultMaxQueueSize,
	Sampler:       "always_on",
	SamplerArg:    1,
	SamplerArgSet: true,
}

// String returns the OTLP protocol name.
func (p Protocol) String() string {
	switch p {
	case ProtocolGRPC:
		return "grpc"
	case ProtocolHTTP:
		return "http"
	default:
		return "unknown"
	}
}

// Config contains all settings needed to build an OTLP Pipeline.
type Config struct {
	Protocol          Protocol
	Endpoint          string
	Headers           map[string]string
	Insecure          bool
	InsecureSet       bool
	Certificate       string
	ClientCertificate string
	ClientKey         string
	Compression       string
	Timeout           time.Duration
	BatchSize         int
	BatchTimeout      time.Duration
	MaxQueueSize      int
	ResourceOverrides map[string]string
	Sampler           string
	SamplerArg        float64
	SamplerArgSet     bool
}

func (c Config) fillDefaults() Config {
	if c.Endpoint == "" {
		c.Endpoint = defaultConfig.Endpoint
	}
	if c.Timeout == 0 {
		c.Timeout = defaultConfig.Timeout
	}
	if c.BatchSize == 0 {
		c.BatchSize = defaultConfig.BatchSize
	}
	if c.BatchTimeout == 0 {
		c.BatchTimeout = defaultConfig.BatchTimeout
	}
	if c.MaxQueueSize == 0 {
		c.MaxQueueSize = defaultConfig.MaxQueueSize
	}
	if c.Sampler == "" {
		c.Sampler = defaultConfig.Sampler
	}
	if !c.SamplerArgSet {
		c.SamplerArg = defaultConfig.SamplerArg
		c.SamplerArgSet = true
	}
	return c
}

// Validate returns nil when c satisfies the exporter configuration invariants.
func (c Config) Validate() error {
	var errs []error
	if c.Protocol != ProtocolGRPC && c.Protocol != ProtocolHTTP {
		errs = append(errs, &ConfigError{Field: "Protocol", Value: c.Protocol, Message: "must be ProtocolGRPC or ProtocolHTTP"})
	}
	if c.Endpoint == "" {
		errs = append(errs, &ConfigError{Field: "Endpoint", Value: c.Endpoint, Message: "must not be empty"})
	} else if !validEndpoint(c.Endpoint) {
		errs = append(errs, &ConfigError{Field: "Endpoint", Value: c.Endpoint, Message: "must be host:port or scheme://host[:port]"})
	}
	if c.Timeout <= 0 {
		errs = append(errs, &ConfigError{Field: "Timeout", Value: c.Timeout, Message: "must be > 0"})
	}
	if c.BatchSize <= 0 {
		errs = append(errs, &ConfigError{Field: "BatchSize", Value: c.BatchSize, Message: "must be > 0"})
	}
	if c.BatchTimeout <= 0 {
		errs = append(errs, &ConfigError{Field: "BatchTimeout", Value: c.BatchTimeout, Message: "must be > 0"})
	}
	if c.MaxQueueSize <= 0 {
		errs = append(errs, &ConfigError{Field: "MaxQueueSize", Value: c.MaxQueueSize, Message: "must be > 0"})
	} else if c.BatchSize > 0 && c.MaxQueueSize < c.BatchSize {
		errs = append(errs, &ConfigError{Field: "MaxQueueSize", Value: c.MaxQueueSize, Message: "must be >= BatchSize"})
	}
	if c.Compression != "" && c.Compression != "gzip" {
		errs = append(errs, &ConfigError{Field: "Compression", Value: c.Compression, Message: `must be "" or "gzip"`})
	}
	if !validSampler(c.Sampler) {
		errs = append(errs, &ConfigError{Field: "Sampler", Value: c.Sampler, Message: `must be "always_on", "always_off", or "traceidratio"`})
	}
	if c.Sampler == "traceidratio" && (c.SamplerArg < 0 || c.SamplerArg > 1) {
		errs = append(errs, &ConfigError{Field: "SamplerArg", Value: c.SamplerArg, Message: "must be in [0,1]"})
	}
	validateStringMap(&errs, "Headers", c.Headers, true)
	validateStringMap(&errs, "ResourceOverrides", c.ResourceOverrides, false)
	if err := validateTLSConfig(c); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// MergeWith returns a new Config where non-zero override fields take precedence.
func (c Config) MergeWith(override Config) Config {
	if override.Protocol != ProtocolGRPC {
		c.Protocol = override.Protocol
	}
	if override.Endpoint != "" {
		c.Endpoint = override.Endpoint
	}
	if override.Headers != nil {
		c.Headers = override.Headers
	}
	if override.Insecure || override.InsecureSet {
		c.Insecure = override.Insecure
		c.InsecureSet = override.InsecureSet
	}
	if override.Certificate != "" {
		c.Certificate = override.Certificate
	}
	if override.ClientCertificate != "" {
		c.ClientCertificate = override.ClientCertificate
	}
	if override.ClientKey != "" {
		c.ClientKey = override.ClientKey
	}
	if override.Compression != "" {
		c.Compression = override.Compression
	}
	if override.Timeout != 0 {
		c.Timeout = override.Timeout
	}
	if override.BatchSize > 0 {
		c.BatchSize = override.BatchSize
	}
	if override.BatchTimeout > 0 {
		c.BatchTimeout = override.BatchTimeout
	}
	if override.MaxQueueSize > 0 {
		c.MaxQueueSize = override.MaxQueueSize
	}
	if override.ResourceOverrides != nil {
		c.ResourceOverrides = override.ResourceOverrides
	}
	if override.Sampler != "" {
		c.Sampler = override.Sampler
	}
	if override.SamplerArgSet {
		c.SamplerArg = override.SamplerArg
		c.SamplerArgSet = true
	}
	return c
}

// ConfigFromEnv reads OTEL_EXPORTER_OTLP_* environment variables into a Config.
func ConfigFromEnv() Config {
	var c Config
	if value, ok := lookupSignalEnv("ENDPOINT"); ok {
		c.Endpoint = value
	}
	if value, ok := lookupSignalEnv("HEADERS"); ok {
		c.Headers = parseHeaders(value)
	}
	if value, ok := lookupSignalEnv("PROTOCOL"); ok {
		c.Protocol = parseProtocol(value)
	}
	if value, ok := lookupSignalEnv("COMPRESSION"); ok {
		c.Compression = value
	}
	if value, ok := lookupSignalEnv("TIMEOUT"); ok {
		c.Timeout = parseTimeoutMillis(value)
	}
	if value, ok := lookupSignalEnv("INSECURE"); ok {
		c.Insecure = parseBool(value)
		c.InsecureSet = true
	}
	if value, ok := lookupSignalEnv("CERTIFICATE"); ok {
		c.Certificate = value
	}
	if value, ok := lookupSignalEnv("CLIENT_CERTIFICATE"); ok {
		c.ClientCertificate = value
	}
	if value, ok := lookupSignalEnv("CLIENT_KEY"); ok {
		c.ClientKey = value
	}
	if value, ok := os.LookupEnv("OTEL_TRACES_SAMPLER"); ok {
		c.Sampler = parseSampler(value)
	}
	if value, ok := os.LookupEnv("OTEL_TRACES_SAMPLER_ARG"); ok {
		arg, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		if err != nil {
			c.SamplerArg = -1
		} else {
			c.SamplerArg = arg
		}
		c.SamplerArgSet = true
	}
	return c
}

func validEndpoint(endpoint string) bool {
	if strings.Contains(endpoint, "://") {
		u, err := url.Parse(endpoint)
		return err == nil && u.Scheme != "" && u.Host != ""
	}
	host, port, err := net.SplitHostPort(endpoint)
	return err == nil && host != "" && port != ""
}

func validateStringMap(errs *[]error, field string, values map[string]string, headerKeys bool) {
	for key, value := range values {
		if key == "" {
			*errs = append(*errs, &ConfigError{Field: field, Value: values, Message: "keys must not be empty"})
			continue
		}
		if headerKeys && !validHeaderKey(key) {
			*errs = append(*errs, &ConfigError{Field: field, Value: key, Message: "keys must match [A-Za-z0-9_-]+"})
		}
		if value == "" {
			*errs = append(*errs, &ConfigError{Field: field, Value: values, Message: "values must not be empty"})
		}
	}
}

func validHeaderKey(key string) bool {
	ok, err := regexp.MatchString(headerKeyPattern, key)
	return err == nil && ok
}

func validSampler(value string) bool {
	switch value {
	case "", "always_on", "always_off", "traceidratio":
		return true
	default:
		return false
	}
}

func parseSampler(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "always_on":
		return "always_on"
	case "always_off":
		return "always_off"
	case "traceidratio":
		return "traceidratio"
	default:
		return fmt.Sprintf("invalid:%s", value)
	}
}

func lookupSignalEnv(suffix string) (string, bool) {
	for _, name := range []string{
		"OTEL_EXPORTER_OTLP_TRACES_" + suffix,
		"OTEL_EXPORTER_OTLP_METRICS_" + suffix,
		"OTEL_EXPORTER_OTLP_LOGS_" + suffix,
		"OTEL_EXPORTER_OTLP_" + suffix,
	} {
		if value, ok := os.LookupEnv(name); ok {
			return value, true
		}
	}
	return "", false
}

func parseProtocol(value string) Protocol {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "grpc":
		return ProtocolGRPC
	case "http/protobuf", "http":
		return ProtocolHTTP
	default:
		return Protocol(-1)
	}
}

func parseHeaders(value string) map[string]string {
	headers := map[string]string{}
	if value == "" {
		return headers
	}
	for _, part := range strings.Split(value, ",") {
		key, rawValue, ok := strings.Cut(part, "=")
		if !ok {
			headers[strings.TrimSpace(part)] = ""
			continue
		}
		decoded, err := url.QueryUnescape(rawValue)
		if err != nil {
			decoded = rawValue
		}
		headers[strings.TrimSpace(key)] = decoded
	}
	return headers
}

func parseTimeoutMillis(value string) time.Duration {
	ms, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	if err != nil {
		return -1
	}
	return time.Duration(ms) * time.Millisecond
}

func parseBool(value string) bool {
	parsed, err := strconv.ParseBool(strings.TrimSpace(value))
	return err == nil && parsed
}
