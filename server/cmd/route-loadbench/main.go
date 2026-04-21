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
	"strconv"
	"strings"
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

type simulationWarmupRequest struct {
	Count int `json:"count"`
}

type summary struct {
	Server        string  `json:"server"`
	RoutingMode   string  `json:"routing_mode"`
	Corpus        string  `json:"corpus"`
	Region        string  `json:"region"`
	QueryCount    int     `json:"query_count"`
	Concurrency   int     `json:"concurrency"`
	Requests      int     `json:"requests"`
	Warmup        int     `json:"warmup"`
	GOMAXPROCS    int     `json:"gomaxprocs"`
	GoVersion     string  `json:"go_version"`
	CommitHash    string  `json:"commit_hash,omitempty"`
	TotalSec      float64 `json:"total_sec"`
	ThroughputRPS float64 `json:"throughput_rps"`
	P50Ms         float64 `json:"p50_ms"`
	P95Ms         float64 `json:"p95_ms"`
	P99Ms         float64 `json:"p99_ms"`
	ErrorCount    int64   `json:"error_count"`
}

func main() {
	serverURL := flag.String("server", "", "Base server URL")
	casesName := flag.String("cases", "", "Benchmark corpus file name under server/test/testdata/benchmark-cases")
	concurrency := flag.Int("concurrency", 0, "Concurrent request workers")
	requests := flag.Int("requests", 0, "Measured request count")
	warmup := flag.Int("warmup", -1, "Warmup request count")
	outPrefix := flag.String("out", "", "Output file prefix (writes <prefix>.json)")
	routingMode := flag.String("routing-mode", "", "Routing mode label for metadata")
	targetGOMAXPROCS := flag.Int("target-gomaxprocs", 0, "Target server GOMAXPROCS for benchmark metadata")
	flag.Parse()

	if strings.TrimSpace(*serverURL) == "" {
		fail("server must be provided")
	}
	if *concurrency <= 0 {
		fail("concurrency must be positive")
	}
	if *requests <= 0 {
		fail("requests must be positive")
	}
	if *warmup < 0 {
		fail("warmup must be non-negative")
	}
	if strings.TrimSpace(*casesName) == "" {
		fail("cases must be provided")
	}
	if strings.TrimSpace(*routingMode) == "" {
		fail("routing-mode must be provided")
	}

	fixture, err := benchmark_test.LoadFixture(*casesName)
	if err != nil {
		failf("LoadFixture: %v", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	if *warmup > 0 {
		if err := runWarmup(client, *serverURL, *warmup); err != nil {
			failf("warmup failed: %v", err)
		}
		defer func() {
			if err := clearWarmup(client, *serverURL); err != nil {
				failf("cleanup failed: %v", err)
			}
		}()
	}

	start := time.Now()
	latencies, errorCount, err := runMeasured(client, *serverURL, fixture.Corpus, *requests, *concurrency)
	if err != nil {
		failf("runMeasured: %v", err)
	}
	total := time.Since(start)

	report := summary{
		Server:        *serverURL,
		RoutingMode:   *routingMode,
		Corpus:        *casesName,
		Region:        fixture.Region,
		QueryCount:    len(fixture.Corpus),
		Concurrency:   *concurrency,
		Requests:      *requests,
		Warmup:        *warmup,
		GOMAXPROCS:    benchmarkGOMAXPROCS(*targetGOMAXPROCS),
		GoVersion:     runtime.Version(),
		CommitHash:    readCommitHash(),
		TotalSec:      total.Seconds(),
		ThroughputRPS: float64(*requests) / total.Seconds(),
		P50Ms:         percentileMs(latencies, 50),
		P95Ms:         percentileMs(latencies, 95),
		P99Ms:         percentileMs(latencies, 99),
		ErrorCount:    errorCount,
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

func benchmarkGOMAXPROCS(target int) int {
	if target > 0 {
		return target
	}
	return runtime.GOMAXPROCS(0)
}

func runWarmup(client *http.Client, serverURL string, warmup int) error {
	body, err := json.Marshal(simulationWarmupRequest{Count: warmup})
	if err != nil {
		return err
	}

	resp, err := client.Post(serverURL+"/simulation/random", "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}

	time.Sleep(warmupWaitDuration())
	return nil
}

func clearWarmup(client *http.Client, serverURL string) error {
	req, err := http.NewRequest(http.MethodDelete, serverURL+"/simulation", nil)
	if err != nil {
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

func warmupWaitDuration() time.Duration {
	const defaultWaitSec = 5

	raw := strings.TrimSpace(os.Getenv("TEST_BENCH_WARMUP_WAIT_SEC"))
	if raw == "" {
		return defaultWaitSec * time.Second
	}

	waitSec, err := strconv.Atoi(raw)
	if err != nil || waitSec < 0 {
		return defaultWaitSec * time.Second
	}
	return time.Duration(waitSec) * time.Second
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
