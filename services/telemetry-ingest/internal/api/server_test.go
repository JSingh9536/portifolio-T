package api

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/jsingh9536/portifolio-t/services/telemetry-ingest/internal/store"
	"github.com/jsingh9536/portifolio-t/services/telemetry-ingest/internal/stream"
)

var now = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func newTestServer(t *testing.T) (*httptest.Server, *Server) {
	t.Helper()
	hub := stream.NewHub()
	reg := prometheus.NewRegistry()
	s := &Server{
		Store: store.NewMemory(), Hub: hub,
		Metrics:  NewMetrics(reg, func() float64 { return float64(hub.Subscribers()) }),
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now:      func() time.Time { return now },
		Gatherer: reg,
	}
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return ts, s
}

const goodRow = `{"robot_id":"tn-01","site_id":"s1","ts":"2026-10-01T11:59:00Z","state":"injecting",
  "x_m":1,"y_m":2,"depth_m":15,"injection_pressure_kpa":700,"flow_rate_lpm":90,"surface_uplift_mm":4,"battery_pct":80}`

func post(t *testing.T, url, body string) (*http.Response, IngestResponse) {
	t.Helper()
	res, err := http.Post(url+"/v1/telemetry", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out IngestResponse
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res, out
}

func TestIngestPartialAcceptance(t *testing.T) {
	ts, _ := newTestServer(t)
	bad := strings.Replace(goodRow, `"battery_pct":80`, `"battery_pct":180`, 1)
	res, out := post(t, ts.URL, `{"samples":[`+goodRow+`,`+bad+`]}`)
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("status %d", res.StatusCode)
	}
	if out.Accepted != 1 || len(out.Rejected) != 1 || out.Rejected[0].Index != 1 {
		t.Fatalf("unexpected response %+v", out)
	}

	res, err := http.Get(ts.URL + "/v1/sites/s1/latest")
	if err != nil {
		t.Fatal(err)
	}
	var latest []map[string]any
	_ = json.NewDecoder(res.Body).Decode(&latest)
	if len(latest) != 1 || latest[0]["robot_id"] != "tn-01" {
		t.Fatalf("latest = %v", latest)
	}
}

func TestIngestRejections(t *testing.T) {
	ts, _ := newTestServer(t)
	for name, tc := range map[string]struct {
		body string
		code int
	}{
		"malformed":     {`{"samples":`, http.StatusBadRequest},
		"unknown field": {`{"samples":[],"extra":1}`, http.StatusBadRequest},
		"empty":         {`{"samples":[]}`, http.StatusBadRequest},
		"all invalid":   {`{"samples":[{"robot_id":"x"}]}`, http.StatusUnprocessableEntity},
	} {
		t.Run(name, func(t *testing.T) {
			if res, _ := post(t, ts.URL, tc.body); res.StatusCode != tc.code {
				t.Fatalf("got %d want %d", res.StatusCode, tc.code)
			}
		})
	}
}

func TestSeriesParamValidation(t *testing.T) {
	ts, _ := newTestServer(t)
	for q, code := range map[string]int{
		"":                                     http.StatusOK,
		"?bucket=5m":                           http.StatusOK,
		"?bucket=1ms":                          http.StatusBadRequest,
		"?bucket=banana":                       http.StatusBadRequest,
		"?from=2026-10-01T12:00:00Z":           http.StatusBadRequest, // from == to
		"?from=2026-01-01T00:00:00Z":           http.StatusBadRequest, // > 31 days
		"?from=2026-09-30T12:00:00Z&bucket=1s": http.StatusBadRequest, // too many buckets
	} {
		res, err := http.Get(ts.URL + "/v1/robots/tn-01/series" + q)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != code {
			t.Errorf("%q: got %d want %d", q, res.StatusCode, code)
		}
	}
}

func TestStreamDeliversIngestedSamples(t *testing.T) {
	ts, srv := newTestServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v1/stream?site=s1", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	r := bufio.NewReader(res.Body)
	if line, _ := r.ReadString('\n'); !strings.HasPrefix(line, ": connected") {
		t.Fatalf("first line %q", line)
	}
	for srv.Hub.Subscribers() == 0 {
		time.Sleep(time.Millisecond)
	}
	post(t, ts.URL, `{"samples":[`+goodRow+`]}`)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(line, "data: ") {
			if !strings.Contains(line, `"robot_id":"tn-01"`) {
				t.Fatalf("unexpected data %q", line)
			}
			return
		}
	}
}

func TestMetricsExposed(t *testing.T) {
	ts, _ := newTestServer(t)
	post(t, ts.URL, `{"samples":[`+goodRow+`]}`)
	res, err := http.Get(ts.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	for _, want := range []string{"telemetry_samples_accepted_total 1", `robot_surface_uplift_mm{robot="tn-01",site="s1"} 4`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("metrics missing %q", want)
		}
	}
}
