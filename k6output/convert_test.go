// SPDX-License-Identifier: Apache-2.0

package k6output

import "testing"

func TestKnownK6Metrics_TableComplete(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"http_req_duration":  "k6.http.request.duration",
		"http_req_failed":    "k6.http.request.failed.total",
		"http_reqs":          "k6.http.requests.total",
		"iterations":         "k6.iterations.total",
		"iteration_duration": "k6.iteration.duration",
		"vus":                "k6.vus",
		"vus_max":            "k6.vus.max",
		"data_sent":          "k6.data.sent.total",
		"data_received":      "k6.data.received.total",
		"checks":             "k6.checks.total",
		"group_duration":     "k6.group.duration",
	}
	if len(knownK6Metrics) != len(want) {
		t.Fatalf("knownK6Metrics len = %d, want %d", len(knownK6Metrics), len(want))
	}
	for _, spec := range knownK6Metrics {
		got, ok := want[spec.k6Name]
		if !ok {
			t.Fatalf("unexpected k6 metric %q", spec.k6Name)
		}
		if spec.otelName != got {
			t.Fatalf("metric %q otelName = %q, want %q", spec.k6Name, spec.otelName, got)
		}
		if spec.unit == "" {
			t.Fatalf("metric %q unit is empty", spec.k6Name)
		}
	}
}

func TestHashTags_SortedInvariance(t *testing.T) {
	t.Parallel()

	left := map[string]string{"status": "200", "method": "GET", "name": "/api"}
	right := map[string]string{"name": "/api", "method": "GET", "status": "200"}
	if got, want := hashTags(left), hashTags(right); got != want {
		t.Fatalf("hashTags order-sensitive: %q != %q", got, want)
	}
}

func TestHashTags_EmptyMap(t *testing.T) {
	t.Parallel()

	if got := hashTags(map[string]string{}); got != "" {
		t.Fatalf("hashTags(empty) = %q, want empty", got)
	}
	if got := hashTags(nil); got != "" {
		t.Fatalf("hashTags(nil) = %q, want empty", got)
	}
}

func TestHashTags_SingleTag(t *testing.T) {
	t.Parallel()

	if got := hashTags(map[string]string{"method": "GET"}); got != "method=GET;" {
		t.Fatalf("hashTags(single) = %q, want method=GET;", got)
	}
}

func TestTagSetCache_Get_CacheHit(t *testing.T) {
	t.Parallel()

	cache := &tagSetCache{}
	first := cache.get(map[string]string{"method": "GET", "status": "200"})
	second := cache.get(map[string]string{"status": "200", "method": "GET"})
	if !first.Equals(&second) {
		t.Fatalf("tag set cache returned non-equivalent sets: %v vs %v", first, second)
	}
	if first.Len() != 2 {
		t.Fatalf("tag set len = %d, want 2", first.Len())
	}
	if value, ok := first.Value("k6.tag.method"); !ok || value.AsString() != "GET" {
		t.Fatalf("k6.tag.method = %q/%v, want GET/true", value.AsString(), ok)
	}
}

func TestTagSetCache_Get_NilCache(t *testing.T) {
	t.Parallel()

	var cache *tagSetCache
	set := cache.get(map[string]string{"method": "GET"})
	if value, ok := set.Value("k6.tag.method"); !ok || value.AsString() != "GET" {
		t.Fatalf("k6.tag.method = %q/%v, want GET/true", value.AsString(), ok)
	}
}

func TestK6UnitHint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want string
	}{
		{name: "http_req_duration", want: "ms"},
		{name: "data_sent", want: "By"},
		{name: "iterations", want: "{iteration}"},
		{name: "vus", want: "{vu}"},
		{name: "unknown_metric", want: ""},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := k6UnitHint(tt.name); got != tt.want {
				t.Fatalf("k6UnitHint(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestKnownMetricSpec_NotFound(t *testing.T) {
	t.Parallel()

	if spec, ok := knownMetricSpec("unknown"); ok || spec.k6Name != "" {
		t.Fatalf("knownMetricSpec(unknown) = %#v, %v; want zero false", spec, ok)
	}
}

func TestDotted(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{in: "http_req_duration", want: "http.req.duration"},
		{in: "vus", want: "vus"},
		{in: "", want: ""},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.in, func(t *testing.T) {
			t.Parallel()

			if got := dotted(tt.in); got != tt.want {
				t.Fatalf("dotted(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
