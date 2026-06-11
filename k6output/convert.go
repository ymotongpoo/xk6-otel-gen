// SPDX-License-Identifier: Apache-2.0

package k6output

import (
	"sort"
	"strings"
	"sync"

	"go.opentelemetry.io/otel/attribute"
)

type k6MetricSpec struct {
	k6Name   string
	otelName string
	unit     string
	instType instrumentType
}

type instrumentType int

const (
	tInstCounter instrumentType = iota
	tInstHistogram
	tInstGauge
)

var knownK6Metrics = []k6MetricSpec{
	{k6Name: "http_req_duration", otelName: "k6.http.request.duration", unit: "ms", instType: tInstHistogram},
	{k6Name: "http_req_failed", otelName: "k6.http.request.failed.total", unit: "{request}", instType: tInstCounter},
	{k6Name: "http_reqs", otelName: "k6.http.requests.total", unit: "{request}", instType: tInstCounter},
	{k6Name: "iterations", otelName: "k6.iterations.total", unit: "{iteration}", instType: tInstCounter},
	{k6Name: "iteration_duration", otelName: "k6.iteration.duration", unit: "ms", instType: tInstHistogram},
	{k6Name: "vus", otelName: "k6.vus", unit: "{vu}", instType: tInstGauge},
	{k6Name: "vus_max", otelName: "k6.vus.max", unit: "{vu}", instType: tInstGauge},
	{k6Name: "data_sent", otelName: "k6.data.sent.total", unit: "By", instType: tInstCounter},
	{k6Name: "data_received", otelName: "k6.data.received.total", unit: "By", instType: tInstCounter},
	{k6Name: "checks", otelName: "k6.checks.total", unit: "{check}", instType: tInstCounter},
	{k6Name: "group_duration", otelName: "k6.group.duration", unit: "ms", instType: tInstHistogram},
}

type instrumentMap struct {
	counters   sync.Map
	histograms sync.Map
	gauges     sync.Map
}

type tagSetCache struct {
	sets sync.Map
}

func hashTags(tags map[string]string) string {
	if len(tags) == 0 {
		return ""
	}
	keys := make([]string, 0, len(tags))
	for key := range tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, key := range keys {
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(tags[key])
		b.WriteByte(';')
	}
	return b.String()
}

func (c *tagSetCache) get(tags map[string]string) attribute.Set {
	if c == nil {
		return attribute.NewSet(tagsToAttributes(tags)...)
	}
	key := hashTags(tags)
	if v, ok := c.sets.Load(key); ok {
		return v.(attribute.Set)
	}
	set := attribute.NewSet(tagsToAttributes(tags)...)
	actual, _ := c.sets.LoadOrStore(key, set)
	return actual.(attribute.Set)
}

func tagsToAttributes(tags map[string]string) []attribute.KeyValue {
	if len(tags) == 0 {
		return nil
	}
	keys := make([]string, 0, len(tags))
	for key := range tags {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	kvs := make([]attribute.KeyValue, 0, len(keys))
	for _, key := range keys {
		kvs = append(kvs, attribute.String("k6.tag."+key, tags[key]))
	}
	return kvs
}

func k6UnitHint(name string) string {
	for _, spec := range knownK6Metrics {
		if spec.k6Name == name {
			return spec.unit
		}
	}
	return ""
}

func dotted(s string) string {
	return strings.ReplaceAll(s, "_", ".")
}

func knownMetricSpec(name string) (k6MetricSpec, bool) {
	for _, spec := range knownK6Metrics {
		if spec.k6Name == name {
			return spec, true
		}
	}
	return k6MetricSpec{}, false
}
