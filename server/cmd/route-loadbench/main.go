package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	benchmark_test "nav-system/test/benchmark"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type routeRequest struct {
	SrcLat       float64 `json:"src_lat"`
	SrcLon       float64 `json:"src_lon"`
	DstLat       float64 `json:"dst_lat"`
	DstLon       float64 `json:"dst_lon"`
	Alternatives int     `json:"alternatives"`
}

type summary struct {
	Server         string  `json:"server"`
	RoutingMode    string  `json:"routing_mode"`
	Corpus         string  `json:"corpus"`
	Region         string  `json:"region"`
	QueryCount     int     `json:"query_count"`
	Concurrency    int     `json:"concurrency"`
	Requests       int     `json:"requests"`
	Warmup         int     `json:"warmup"`
	GOMAXPROCS     int     `json:"gomaxprocs"`
	GoVersion      string  `json:"go_version"`
	CommitHash     string  `json:"commit_hash,omitempty"`
	TotalSec       float64 `json:"total_sec"`
	ThroughputRPS  float64 `json:"throughput_rps"`
	P50Ms          float64 `json:"p50_ms"`
	P95Ms          float64 `json:"p95_ms"`
	P99Ms          float64 `json:"p99_ms"`
	ErrorCount     int64   `json:"error_count"`
}

func main() {
	serverURL := flag.String("server", "http://127.0.0.1:8080", "Base server URL")
	casesName := flag.String("cases", "israel-and-palestine-bench.json", "Benchmark corpus file name under server/test/testdata/benchmark-cases")
	concurrency := flag.Int("concurrency", 32, "Concurrent request workers")
	requests := flag.Int("requests", 256, "Measured request count")
	warmup := flag.Int("warmup", 64, "Warmup request count")
	outPrefix := flag.String("out", "", "Output file prefix (writes <prefix>.json)")
	routingMode := flag.String("routing-mode", "hierarchical", "Routing mode label for metadata")
	flag.Parse()

	if *concurrency <= 0 {
		fail("concurrency must be positive")
	}
	if *requests <= 0 {
		fail("requests must be positive")
	}
	if *warmup < 0 {
		fail("warmup must be non-negative")
	}

	fixture, err := benchmark_test.LoadFixture(*casesName)
	if err != nil {
		failf("LoadFixture: %v", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	if *warmup > 0 {
		if err := runWarmup(client, *serverURL, fixture.Corpus, *warmup); err != nil {
			failf("warmup failed: %v", err)
		}
	}

	start := time.Now()
	latencies, errorCount, err := runMeasured(client, *serverURL, fixture.Corpus, *requests, *concurrency)
	if err != nil {
		failf("runMeasured: %v", err)
	}
	total := time.Since(start)

	report := summary{
		Server:         *serverURL,
		RoutingMode:    *routingMode,
		Corpus:         *casesName,
		Region:         fixture.Region,
		QueryCount:     len(fixture.Corpus),
		Concurrency:    *concurrency,
		Requests:       *requests,
		Warmup:         *warmup,
		GOMAXPROCS:     runtime.GOMAXPROCS(0),
		GoVersion:      runtime.Version(),
		CommitHash:     readCommitHash(),
		TotalSec:       total.Seconds(),
		ThroughputRPS:  float64(*requests) / total.Seconds(),
		P50Ms:          percentileMs(latencies, 50),
		P95Ms:          percentileMs(latencies, 95),
		P99Ms:          percentileMs(latencies, 99),
		ErrorCount:     errorCount,
	}

	if *outPrefix != "" {
		if err := writeOutputs(*outPrefix, report); err != nil {
			failf("write outputs: %v", err)
		}
	}

	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		failf("marshal summary: %v", err)
	}
	fmt.Println(string(encoded))
}

func runWarmup(client *http.Client, serverURL string, corpus []benchmark_test.CorpusCase, warmup int) error {
	for i := 0; i < warmup; i++ {
		if err := postRoute(client, serverURL, corpus[i%len(corpus)]); err != nil {
			return err
		}
	}
	return nil
}

func runMeasured(client *http.Client, serverURL string, corpus []benchmark_test.CorpusCase, requests, concurrency int) ([]float64, int64, error) {
	latencies := make([]float64, requests)
	jobs := make(chan int, requests)
	var errorCount atomic.Int64

	var wg sync.WaitGroup
	for worker := 0; worker < concurrency; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				start := time.Now()
				if err := postRoute(client, serverURL, corpus[idx%len(corpus)]); err != nil {
					errorCount.Add(1)
				}
				latencies[idx] = float64(time.Since(start).Microseconds()) / 1000.0
			}
		}()
	}

	for i := 0; i < requests; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return latencies, errorCount.Load(), nil
}

func postRoute(client *http.Client, serverURL string, query benchmark_test.CorpusCase) error {
	body, err := json.Marshal(routeRequest{
		SrcLat:       query.SrcLat,
		SrcLon:       query.SrcLon,
		DstLat:       query.DstLat,
		DstLon:       query.DstLon,
		Alternatives: 0,
	})
	if err != nil {
		return err
	}

	resp, err := client.Post(serverURL+"/route", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func percentileMs(values []float64, pct int) float64 {
	if len(values) == 0 {
		return 0
	}
	copyValues := append([]float64(nil), values...)
	sort.Float64s(copyValues)
	index := (len(copyValues) - 1) * pct / 100
	return copyValues[index]
}

func writeOutputs(prefix string, report summary) error {
	dir := filepath.Dir(prefix)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	jsonPath := prefix + ".json"

	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, encoded, 0o644); err != nil {
		return err
	}
	return nil
}

func readCommitHash() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			return setting.Value
		}
	}
	return ""
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, "error:", message)
	os.Exit(1)
}

func failf(format string, args ...any) {
	fail(fmt.Sprintf(format, args...))
}
