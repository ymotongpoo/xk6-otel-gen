// SPDX-FileCopyrightText: Yoshi Yamaguchi <ymotongpoo@gmail.com>
// SPDX-License-Identifier: Apache-2.0

package exporter

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ymotongpoo/xk6-otel-gen/synth"
)

func TestPipeline_ProfileExporter_NilWhenUnset(t *testing.T) {
	t.Parallel()

	p, err := New(Config{Endpoint: "localhost:4317", Insecure: true})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if p.ProfileExporter() != nil {
		t.Fatal("ProfileExporter() should be nil when ProfilesEndpoint unset")
	}
	_ = p.Shutdown(context.Background())
}

func TestPyroscopeClient_PushProfile_SendsExpectedRequest(t *testing.T) {
	t.Parallel()

	var gotMethod, gotQuery, gotAuth string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		gotBody = body
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	stats := &pipelineStats{}
	cfg := validPipelineConfig()
	cfg.ProfilesEndpoint = srv.URL
	cfg.Headers = map[string]string{"Authorization": "Bearer test-token"}
	client, err := newPyroscopeClient(cfg, nil, stats)
	if err != nil {
		t.Fatalf("newPyroscopeClient() error = %v", err)
	}

	err = client.PushProfile(context.Background(), synth.ProfilePush{
		AppName:    "shipping",
		Labels:     map[string]string{"span_id": "abc", "operation": "quote_shipping", "service_name": "shipping"},
		FromNanos:  100,
		UntilNanos: 200,
		SampleRate: 100,
		Pprof:      []byte("pprof-bytes"),
	})
	if err != nil {
		t.Fatalf("PushProfile() error = %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method = %q", gotMethod)
	}
	if gotAuth != "Bearer test-token" {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if string(gotBody) != "pprof-bytes" {
		t.Fatalf("body = %q", gotBody)
	}
	q, err := url.ParseQuery(gotQuery)
	if err != nil {
		t.Fatalf("ParseQuery() error = %v", err)
	}
	if q.Get("format") != "pprof" || q.Get("from") != "100" || q.Get("until") != "200" || q.Get("sampleRate") != "100" {
		t.Fatalf("query = %#v", q)
	}
	name := q.Get("name")
	if !strings.Contains(name, "shipping{") || !strings.Contains(name, "operation=quote_shipping") || !strings.Contains(name, "span_id=abc") {
		t.Fatalf("name = %q", name)
	}
	if stats.profilesExported.Load() != 1 {
		t.Fatalf("profilesExported = %d", stats.profilesExported.Load())
	}
}

func TestPyroscopeClient_PushProfile_FailureIncrementsCounter(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	stats := &pipelineStats{}
	cfg := validPipelineConfig()
	cfg.ProfilesEndpoint = srv.URL
	cfg.Timeout = time.Second
	client, err := newPyroscopeClient(cfg, nil, stats)
	if err != nil {
		t.Fatalf("newPyroscopeClient() error = %v", err)
	}
	if err := client.PushProfile(context.Background(), synth.ProfilePush{AppName: "app", Pprof: []byte("x")}); err == nil {
		t.Fatal("PushProfile() error = nil, want failure")
	}
	if stats.profilesFailed.Load() != 1 {
		t.Fatalf("profilesFailed = %d", stats.profilesFailed.Load())
	}
}

func TestBuildPyroscopeName_DeterministicLabelOrder(t *testing.T) {
	t.Parallel()

	got := buildPyroscopeName("app", map[string]string{"b": "2", "a": "1"})
	want := "app{a=1,b=2}"
	if got != want {
		t.Fatalf("name = %q, want %q", got, want)
	}
}
