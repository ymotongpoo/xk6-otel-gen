// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package exporter

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/ymotongpoo/xk6-otel-gen/synth"
)

type pyroscopeClient struct {
	cfg    Config
	stats  *pipelineStats
	client *http.Client
}

func newPyroscopeClient(cfg Config, tlsConfig *tls.Config, stats *pipelineStats) (*pyroscopeClient, error) {
	if cfg.ProfilesEndpoint == "" {
		return nil, nil
	}
	if !validEndpoint(cfg.ProfilesEndpoint) {
		return nil, &ConfigError{Field: "ProfilesEndpoint", Value: cfg.ProfilesEndpoint, Message: "must be host:port or scheme://host[:port]"}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = tlsConfig
	return &pyroscopeClient{
		cfg:   cfg,
		stats: stats,
		client: &http.Client{
			Timeout:   cfg.Timeout,
			Transport: transport,
		},
	}, nil
}

func (c *pyroscopeClient) PushProfile(ctx context.Context, p synth.ProfilePush) error {
	if c == nil {
		return nil
	}
	endpoint := strings.TrimRight(c.cfg.ProfilesEndpoint, "/")
	u, err := url.Parse(endpoint + "/ingest")
	if err != nil {
		c.stats.profilesFailed.Add(1)
		return fmt.Errorf("pyroscope ingest url: %w", err)
	}

	q := u.Query()
	q.Set("name", buildPyroscopeName(p.AppName, p.Labels))
	q.Set("from", strconv.FormatInt(p.FromNanos, 10))
	q.Set("until", strconv.FormatInt(p.UntilNanos, 10))
	q.Set("format", "pprof")
	q.Set("sampleRate", strconv.Itoa(p.SampleRate))
	u.RawQuery = q.Encode()

	body := p.Pprof
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		c.stats.profilesFailed.Add(1)
		return fmt.Errorf("pyroscope request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	for key, value := range c.cfg.Headers {
		req.Header.Set(key, value)
	}
	if c.cfg.Compression == "gzip" {
		var compressed bytes.Buffer
		gw := gzip.NewWriter(&compressed)
		if _, err := gw.Write(body); err != nil {
			c.stats.profilesFailed.Add(1)
			return fmt.Errorf("pyroscope gzip: %w", err)
		}
		if err := gw.Close(); err != nil {
			c.stats.profilesFailed.Add(1)
			return fmt.Errorf("pyroscope gzip close: %w", err)
		}
		req.Body = io.NopCloser(bytes.NewReader(compressed.Bytes()))
		req.ContentLength = int64(compressed.Len())
		req.Header.Set("Content-Encoding", "gzip")
	}

	resp, err := c.client.Do(req)
	if err != nil {
		c.stats.profilesFailed.Add(1)
		return fmt.Errorf("pyroscope post: %w", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		c.stats.profilesFailed.Add(1)
		return fmt.Errorf("pyroscope post: status %s", resp.Status)
	}
	c.stats.profilesExported.Add(1)
	return nil
}

func buildPyroscopeName(app string, labels map[string]string) string {
	if len(labels) == 0 {
		return app
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(app)
	b.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(labels[key])
	}
	b.WriteByte('}')
	return b.String()
}
