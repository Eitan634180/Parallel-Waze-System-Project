package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	benchmarkfixture "nav-system/test/benchmark"
	graphbuilder "nav-system/src/graph/builder"
	"nav-system/src/graph/model"
	graphstore "nav-system/src/graph/store"
	trafficcustomization "nav-system/src/traffic/customization"
	trafficstore "nav-system/src/traffic/store"
	"nav-system/src/utilities"
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
	"unicode/utf16"
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

type loadbenchSummary struct {
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

type goBenchmarkReport struct {
	GoOS       string
	GoArch     string
	CPU        string
	Benchmarks map[string]map[string]float64
}

type buildSummary struct {
	GOMAXPROCS    int
	OverlayTimeMs float64
	TotalTimeMs   float64
}

type overlaySummary struct {
	GOMAXPROCS int
	AvgTimeMs  float64
	Runs       int
}

type customizationSummary struct {
	GOMAXPROCS int
	AvgTimeMs  float64
	Runs       int
}

type compareRow struct {
	Mode         string
	NsPerOp      float64
	VisitedNodes float64
}

const (
	routeLoadCommand            = "route-load"
	overlayBuildCommand         = "overlay-build"
	overlayCustomizationCommand = "overlay-customization"
	reportCommand               = "report"

	overlayBenchLogPrefix       = "overlay-bench:"
	customizationBenchLogPrefix = "customization-bench:"

	loadbenchHTTPTimeout       = 30 * time.Second
	loadbenchDefaultWaitSec    = 5
	loadbenchP50               = 50
	loadbenchP95               = 95
	loadbenchP99               = 99
	loadbenchPercentDivisor    = 100
	microsecondsPerMillisecond = 1000.0
	nanosecondsPerMillisecond  = 1_000_000.0

	customizationObservedRatio = float32(2.0)

	goBenchmarkMinimumFields    = 4
	goBenchmarkMetricStartIndex = 2
	goBenchmarkMetricFieldStep  = 2
	digitZero                   = '0'
	digitNine                   = '9'
	utf16BOMSize                = 2
	utf16CodeUnitSize           = 2

	reportFilePerm = 0o644
	reportDirPerm  = 0o755
)

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(1)
	}

	switch os.Args[1] {
	case routeLoadCommand:
		runRouteLoad(os.Args[2:])
	case overlayBuildCommand:
		runOverlayBuild(os.Args[2:])
	case overlayCustomizationCommand:
		runOverlayCustomization(os.Args[2:])
	case reportCommand:
		runReport(os.Args[2:])
	case "help", "-h", "--help":
		printUsage(os.Stdout)
	default:
		failf("unknown subcommand %q", os.Args[1])
	}
}

func runRouteLoad(args []string) {
	flags := flag.NewFlagSet(routeLoadCommand, flag.ExitOnError)
	serverURL := flags.String("server", "", "Base server URL")
	casesName := flags.String("cases", "", "Benchmark corpus file name under server/test/testdata/benchmark-cases")
	concurrency := flags.Int("concurrency", 0, "Concurrent request workers")
	requests := flags.Int("requests", 0, "Measured request count")
	warmup := flags.Int("warmup", -1, "Warmup request count")
	outPrefix := flags.String("out", "", "Output file prefix (writes <prefix>.json)")
	routingMode := flags.String("routing-mode", "", "Routing mode label for metadata")
	targetGOMAXPROCS := flags.Int("target-gomaxprocs", 0, "Target server GOMAXPROCS for benchmark metadata")
	flags.Parse(args)

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

	fixture, err := benchmarkfixture.LoadFixture(*casesName)
	if err != nil {
		failf("LoadFixture: %v", err)
	}

	client := &http.Client{Timeout: loadbenchHTTPTimeout}
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

	report := loadbenchSummary{
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
		P50Ms:         percentileMs(latencies, loadbenchP50),
		P95Ms:         percentileMs(latencies, loadbenchP95),
		P99Ms:         percentileMs(latencies, loadbenchP99),
		ErrorCount:    errorCount,
	}

	if *outPrefix != "" {
		if err := writeLoadbenchOutputs(*outPrefix, report); err != nil {
			failf("write outputs: %v", err)
		}
	}

	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		failf("marshal summary: %v", err)
	}
	fmt.Println(string(encoded))
}

func runOverlayBuild(args []string) {
	flags := flag.NewFlagSet(overlayBuildCommand, flag.ExitOnError)
	dataDir := flags.String("data", "", "Path to saved graph directory")
	workersFlag := flags.String("workers", "1", "Comma-separated worker counts")
	runs := flags.Int("runs", 5, "Timed runs per worker count")
	flags.Parse(args)

	if *runs <= 0 {
		log.Fatalf("runs must be > 0")
	}
	if *dataDir == "" {
		*dataDir = utilities.RequireEnv("NAV_MAP_ROOT")
	}

	workerCounts := parseWorkerCounts(*workersFlag)
	originalGOMAXPROCS := runtime.GOMAXPROCS(0)
	defer runtime.GOMAXPROCS(originalGOMAXPROCS)

	log.Printf("%s loading graph from %s", overlayBenchLogPrefix, *dataDir)
	g, err := graphstore.LoadGraph(*dataDir)
	if err != nil {
		log.Fatalf("LoadGraph: %v", err)
	}
	log.Printf("%s graph ready (%d cells, %d boundary nodes)", overlayBenchLogPrefix, len(g.Cells), len(g.BoundaryBaseIdxs))

	for _, workers := range workerCounts {
		var total time.Duration
		for run := 1; run <= *runs; run++ {
			runtime.GOMAXPROCS(workers)
			elapsed := graphbuilder.BuildOverlayGraph(g, workers)
			total += elapsed
			log.Printf("%s workers=%d run=%d duration=%s", overlayBenchLogPrefix, workers, run, elapsed.Round(time.Millisecond))
		}

		avg := total / time.Duration(*runs)
		fmt.Printf("%s workers=%d avg=%s runs=%d\n", overlayBenchLogPrefix, workers, avg.Round(time.Millisecond), *runs)
	}
}

func runOverlayCustomization(args []string) {
	flags := flag.NewFlagSet(overlayCustomizationCommand, flag.ExitOnError)
	dataDir := flags.String("data", "", "Path to saved graph directory")
	workersFlag := flags.String("workers", "1", "Comma-separated worker counts")
	runs := flags.Int("runs", 5, "Timed runs per worker count")
	flags.Parse(args)

	if *runs <= 0 {
		log.Fatalf("runs must be > 0")
	}
	if *dataDir == "" {
		*dataDir = utilities.RequireEnv("NAV_MAP_ROOT")
	}

	workerCounts := parseWorkerCounts(*workersFlag)
	originalGOMAXPROCS := runtime.GOMAXPROCS(0)
	defer runtime.GOMAXPROCS(originalGOMAXPROCS)

	log.Printf("%s loading graph from %s", customizationBenchLogPrefix, *dataDir)
	g, err := graphstore.LoadGraph(*dataDir)
	if err != nil {
		log.Fatalf("LoadGraph: %v", err)
	}
	log.Printf("%s graph ready (%d cells, %d boundary nodes, %d edges)", customizationBenchLogPrefix, len(g.Cells), len(g.BoundaryBaseIdxs), len(g.Edges))

	store := trafficstore.NewStore()
	dirtyEdges := seedDirtyStore(g, store)
	log.Printf("%s store ready (%d dirty edges)", customizationBenchLogPrefix, dirtyEdges)

	trafficcustomization.CustomizeOverlayWeights(g, store)
	log.Printf("%s finished initial customization", customizationBenchLogPrefix)

	for _, workers := range workerCounts {
		runtime.GOMAXPROCS(workers)

		var total time.Duration
		for run := 1; run <= *runs; run++ {
			store.RefillPendingForBenchmarks()
			start := time.Now()
			trafficcustomization.CustomizeOverlayWeights(g, store)
			elapsed := time.Since(start)
			total += elapsed
			log.Printf("%s workers=%d run=%d duration=%s", customizationBenchLogPrefix, workers, run, elapsed.Round(time.Millisecond))
		}

		avg := total / time.Duration(*runs)
		fmt.Printf("%s workers=%d avg=%s runs=%d\n", customizationBenchLogPrefix, workers, avg.Round(time.Millisecond), *runs)
	}
}

func runReport(args []string) {
	flags := flag.NewFlagSet(reportCommand, flag.ExitOnError)
	dir := flags.String("dir", "", "Benchmark output directory")
	flags.Parse(args)

	if *dir == "" {
		fail("missing --dir")
	}

	reportDir, err := filepath.Abs(*dir)
	if err != nil {
		failf("resolve report dir: %v", err)
	}

	var builder strings.Builder
	builder.WriteString("# Benchmark Report\n\n")
	builder.WriteString(fmt.Sprintf("Generated: `%s`\n\n", time.Now().Format(time.RFC3339)))

	if err := writeGoBenchmarkSection(&builder, "Single-Query Static Comparison", filepath.Join(reportDir, "compare-static.txt"), "BenchmarkRouterCompareStatic"); err != nil {
		builder.WriteString(fmt.Sprintf("_Static comparison unavailable: %v_\n\n", err))
	}
	if err := writeServerScaleSection(&builder, filepath.Join(reportDir, "server-scale")); err != nil {
		builder.WriteString(fmt.Sprintf("_Server-scale results unavailable: %v_\n\n", err))
	}
	if err := writeBuildScaleSection(&builder, filepath.Join(reportDir, "build-scale")); err != nil {
		builder.WriteString(fmt.Sprintf("_Build-scale results unavailable: %v_\n\n", err))
	}
	if err := writeOverlayScaleSection(&builder, filepath.Join(reportDir, "overlay-scale")); err != nil {
		builder.WriteString(fmt.Sprintf("_Overlay-scale results unavailable: %v_\n\n", err))
	}
	if err := writeCustomizationScaleSection(&builder, filepath.Join(reportDir, "customization-scale")); err != nil {
		builder.WriteString(fmt.Sprintf("_Customization-scale results unavailable: %v_\n\n", err))
	}

	summaryPath := filepath.Join(reportDir, "summary.md")
	if err := os.WriteFile(summaryPath, []byte(builder.String()), reportFilePerm); err != nil {
		failf("write summary: %v", err)
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  benchmark <subcommand> [flags]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Subcommands:")
	fmt.Fprintf(w, "  %-22s Run HTTP route-load benchmark against a server\n", routeLoadCommand)
	fmt.Fprintf(w, "  %-22s Benchmark overlay graph build throughput\n", overlayBuildCommand)
	fmt.Fprintf(w, "  %-22s Benchmark overlay customization throughput\n", overlayCustomizationCommand)
	fmt.Fprintf(w, "  %-22s Generate a markdown report from benchmark outputs\n", reportCommand)
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
	raw := strings.TrimSpace(os.Getenv("TEST_BENCH_WARMUP_WAIT_SEC"))
	if raw == "" {
		return loadbenchDefaultWaitSec * time.Second
	}

	waitSec, err := strconv.Atoi(raw)
	if err != nil || waitSec < 0 {
		return loadbenchDefaultWaitSec * time.Second
	}
	return time.Duration(waitSec) * time.Second
}

func runMeasured(client *http.Client, serverURL string, corpus []benchmarkfixture.CorpusCase, requests, concurrency int) ([]float64, int64, error) {
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
				latencies[idx] = float64(time.Since(start).Microseconds()) / microsecondsPerMillisecond
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

func postRoute(client *http.Client, serverURL string, query benchmarkfixture.CorpusCase) error {
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
	index := (len(copyValues) - 1) * pct / loadbenchPercentDivisor
	return copyValues[index]
}

func writeLoadbenchOutputs(prefix string, report loadbenchSummary) error {
	dir := filepath.Dir(prefix)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, reportDirPerm); err != nil {
			return err
		}
	}

	jsonPath := prefix + ".json"
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(jsonPath, encoded, reportFilePerm); err != nil {
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

func parseWorkerCounts(raw string) []int {
	parts := strings.Split(raw, ",")
	workers := make([]int, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil || value <= 0 {
			log.Fatalf("invalid worker count %q", part)
		}
		workers = append(workers, value)
	}
	return workers
}

func seedDirtyStore(g *model.Graph, store *trafficstore.Store) int {
	dirtyEdges := 0
	for _, edge := range g.Edges {
		if edge.Weight <= 0 {
			continue
		}
		store.RecordObservation(edge.ID, edge.Weight*customizationObservedRatio, edge.Weight)
		dirtyEdges++
	}
	return dirtyEdges
}

func writeGoBenchmarkSection(builder *strings.Builder, title, path, prefix string) error {
	report, err := parseGoBenchmarkFile(path)
	if err != nil {
		return err
	}

	builder.WriteString("## " + title + "\n\n")
	if report.CPU != "" || report.GoOS != "" || report.GoArch != "" {
		builder.WriteString(fmt.Sprintf("- Environment: `%s / %s / %s`\n\n", report.GoOS, report.GoArch, report.CPU))
	}

	rows := make([]compareRow, 0, len(report.Benchmarks))
	for name, metrics := range report.Benchmarks {
		if !strings.HasPrefix(name, prefix+"/") {
			continue
		}
		mode := strings.TrimPrefix(name, prefix+"/")
		mode = trimGOMAXPROCSSuffix(mode)
		rows = append(rows, compareRow{
			Mode:         mode,
			NsPerOp:      metrics["ns/op"],
			VisitedNodes: metrics["visited_nodes/op"],
		})
	}
	if len(rows) == 0 {
		return fmt.Errorf("no benchmarks with prefix %q", prefix)
	}

	sort.Slice(rows, func(i, j int) bool { return rows[i].NsPerOp < rows[j].NsPerOp })

	astarNs := metricForMode(rows, "base-astar")
	dijkstraNs := metricForMode(rows, "base-dijkstra")

	builder.WriteString("| Mode | ms/op | Visited Nodes/op | vs A* | vs Dijkstra |\n")
	builder.WriteString("| --- | ---: | ---: | ---: | ---: |\n")
	for _, row := range rows {
		builder.WriteString(fmt.Sprintf(
			"| `%s` | %.3f | %.0f | %s | %s |\n",
			row.Mode,
			row.NsPerOp/nanosecondsPerMillisecond,
			row.VisitedNodes,
			speedupString(astarNs, row.NsPerOp),
			speedupString(dijkstraNs, row.NsPerOp),
		))
	}
	builder.WriteString("\n")
	return nil
}

func writeServerScaleSection(builder *strings.Builder, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	var results []loadbenchSummary
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		summary, err := loadLoadbenchSummary(filepath.Join(dir, entry.Name()))
		if err != nil {
			return err
		}
		results = append(results, summary)
	}
	if len(results) == 0 {
		return fmt.Errorf("no load-bench json summaries found")
	}

	sort.Slice(results, func(i, j int) bool {
		if results[i].RoutingMode == results[j].RoutingMode {
			return results[i].GOMAXPROCS < results[j].GOMAXPROCS
		}
		return results[i].RoutingMode < results[j].RoutingMode
	})

	baseline := make(map[string]float64)
	for _, result := range results {
		if _, ok := baseline[result.RoutingMode]; !ok || result.GOMAXPROCS == 1 {
			baseline[result.RoutingMode] = result.ThroughputRPS
		}
	}

	builder.WriteString("## Server Throughput Scaling\n\n")
	builder.WriteString("| Mode | GOMAXPROCS | Throughput (req/s) | p50 (ms) | p95 (ms) | p99 (ms) | Errors | Speedup |\n")
	builder.WriteString("| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: |\n")
	for _, result := range results {
		builder.WriteString(fmt.Sprintf(
			"| `%s` | %d | %.2f | %.2f | %.2f | %.2f | %d | %s |\n",
			result.RoutingMode,
			result.GOMAXPROCS,
			result.ThroughputRPS,
			result.P50Ms,
			result.P95Ms,
			result.P99Ms,
			result.ErrorCount,
			ratioString(result.ThroughputRPS, baseline[result.RoutingMode]),
		))
	}
	builder.WriteString("\n")
	return nil
}

func writeBuildScaleSection(builder *strings.Builder, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	var results []buildSummary
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".log" {
			continue
		}
		summary, err := parseBuildLog(filepath.Join(dir, entry.Name()))
		if err != nil {
			return err
		}
		results = append(results, summary)
	}
	if len(results) == 0 {
		return fmt.Errorf("no build logs found")
	}

	sort.Slice(results, func(i, j int) bool { return results[i].GOMAXPROCS < results[j].GOMAXPROCS })
	totalBaseline := results[0].TotalTimeMs
	overlayBaseline := results[0].OverlayTimeMs

	builder.WriteString("## Build Scaling\n\n")
	builder.WriteString("| GOMAXPROCS | Overlay (ms) | Total (ms) | Overlay Speedup | Total Speedup |\n")
	builder.WriteString("| --- | ---: | ---: | ---: | ---: |\n")
	for _, result := range results {
		builder.WriteString(fmt.Sprintf(
			"| %d | %.2f | %.2f | %s | %s |\n",
			result.GOMAXPROCS,
			result.OverlayTimeMs,
			result.TotalTimeMs,
			ratioString(overlayBaseline, result.OverlayTimeMs),
			ratioString(totalBaseline, result.TotalTimeMs),
		))
	}
	builder.WriteString("\n")
	return nil
}

func writeOverlayScaleSection(builder *strings.Builder, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	var results []overlaySummary
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".log" {
			continue
		}
		summaries, err := parseOverlayLog(filepath.Join(dir, entry.Name()))
		if err != nil {
			return err
		}
		results = append(results, summaries...)
	}
	if len(results) == 0 {
		return fmt.Errorf("no overlay-scale logs found")
	}

	sort.Slice(results, func(i, j int) bool { return results[i].GOMAXPROCS < results[j].GOMAXPROCS })
	baseline := results[0].AvgTimeMs

	builder.WriteString("## Overlay Build Scaling\n\n")
	builder.WriteString("| GOMAXPROCS | Avg Overlay (ms) | Runs | Speedup |\n")
	builder.WriteString("| --- | ---: | ---: | ---: |\n")
	for _, result := range results {
		builder.WriteString(fmt.Sprintf(
			"| %d | %.2f | %d | %s |\n",
			result.GOMAXPROCS,
			result.AvgTimeMs,
			result.Runs,
			ratioString(baseline, result.AvgTimeMs),
		))
	}
	builder.WriteString("\n")
	return nil
}

func writeCustomizationScaleSection(builder *strings.Builder, dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}

	var results []customizationSummary
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".log" {
			continue
		}
		summaries, err := parseCustomizationLog(filepath.Join(dir, entry.Name()))
		if err != nil {
			return err
		}
		results = append(results, summaries...)
	}
	if len(results) == 0 {
		return fmt.Errorf("no customization-scale logs found")
	}

	sort.Slice(results, func(i, j int) bool { return results[i].GOMAXPROCS < results[j].GOMAXPROCS })
	baseline := results[0].AvgTimeMs

	builder.WriteString("## Overlay Customization Scaling\n\n")
	builder.WriteString("| GOMAXPROCS | Avg Customization (ms) | Runs | Speedup |\n")
	builder.WriteString("| --- | ---: | ---: | ---: |\n")
	for _, result := range results {
		builder.WriteString(fmt.Sprintf(
			"| %d | %.2f | %d | %s |\n",
			result.GOMAXPROCS,
			result.AvgTimeMs,
			result.Runs,
			ratioString(baseline, result.AvgTimeMs),
		))
	}
	builder.WriteString("\n")
	return nil
}

func parseGoBenchmarkFile(path string) (*goBenchmarkReport, error) {
	text, err := readTextFile(path)
	if err != nil {
		return nil, err
	}

	report := &goBenchmarkReport{Benchmarks: make(map[string]map[string]float64)}
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		switch {
		case strings.HasPrefix(line, "goos:"):
			report.GoOS = strings.TrimSpace(strings.TrimPrefix(line, "goos:"))
		case strings.HasPrefix(line, "goarch:"):
			report.GoArch = strings.TrimSpace(strings.TrimPrefix(line, "goarch:"))
		case strings.HasPrefix(line, "cpu:"):
			report.CPU = strings.TrimSpace(strings.TrimPrefix(line, "cpu:"))
		case strings.HasPrefix(line, "Benchmark"):
			fields := strings.Fields(line)
			if len(fields) < goBenchmarkMinimumFields {
				continue
			}
			metrics := make(map[string]float64)
			for i := goBenchmarkMetricStartIndex; i+1 < len(fields); i += goBenchmarkMetricFieldStep {
				value, err := strconv.ParseFloat(fields[i], 64)
				if err != nil {
					continue
				}
				metrics[fields[i+1]] = value
			}
			report.Benchmarks[fields[0]] = metrics
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return report, nil
}

func loadLoadbenchSummary(path string) (loadbenchSummary, error) {
	var summary loadbenchSummary
	data, err := os.ReadFile(path)
	if err != nil {
		return summary, err
	}
	if err := json.Unmarshal(data, &summary); err != nil {
		return summary, err
	}
	return summary, nil
}

func parseBuildLog(path string) (buildSummary, error) {
	text, err := readTextFile(path)
	if err != nil {
		return buildSummary{}, err
	}

	gomaxprocs := gomaxprocsFromPath(path)
	overlay, err := parseDurationAfter(text, "overlay ready in ")
	if err != nil {
		return buildSummary{}, err
	}
	total, err := parseDurationAfter(text, "build complete in ")
	if err != nil {
		return buildSummary{}, err
	}
	return buildSummary{
		GOMAXPROCS:    gomaxprocs,
		OverlayTimeMs: float64(overlay.Microseconds()) / microsecondsPerMillisecond,
		TotalTimeMs:   float64(total.Microseconds()) / microsecondsPerMillisecond,
	}, nil
}

func parseOverlayLog(path string) ([]overlaySummary, error) {
	text, err := readTextFile(path)
	if err != nil {
		return nil, err
	}

	summaries := make([]overlaySummary, 0)
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, overlayBenchLogPrefix+" workers=") || !strings.Contains(line, " avg=") {
			continue
		}

		summary := overlaySummary{GOMAXPROCS: gomaxprocsFromPath(path)}
		if workersText, ok := fieldValue(line, "workers="); ok {
			workers, err := strconv.Atoi(workersText)
			if err != nil {
				return nil, err
			}
			summary.GOMAXPROCS = workers
		}

		avgText, ok := fieldValue(line, "avg=")
		if !ok {
			continue
		}
		avg, err := time.ParseDuration(avgText)
		if err != nil {
			return nil, err
		}
		summary.AvgTimeMs = float64(avg.Microseconds()) / microsecondsPerMillisecond

		if runsText, ok := fieldValue(line, "runs="); ok {
			runs, err := strconv.Atoi(runsText)
			if err != nil {
				return nil, err
			}
			summary.Runs = runs
		}
		summaries = append(summaries, summary)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(summaries) == 0 {
		return nil, fmt.Errorf("missing overlay summary in %s", path)
	}
	return summaries, nil
}

func parseCustomizationLog(path string) ([]customizationSummary, error) {
	text, err := readTextFile(path)
	if err != nil {
		return nil, err
	}

	summaries := make([]customizationSummary, 0)
	scanner := bufio.NewScanner(strings.NewReader(text))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, customizationBenchLogPrefix+" workers=") || !strings.Contains(line, " avg=") {
			continue
		}

		summary := customizationSummary{GOMAXPROCS: gomaxprocsFromPath(path)}
		if workersText, ok := fieldValue(line, "workers="); ok {
			workers, err := strconv.Atoi(workersText)
			if err != nil {
				return nil, err
			}
			summary.GOMAXPROCS = workers
		}

		avgText, ok := fieldValue(line, "avg=")
		if !ok {
			continue
		}
		avg, err := time.ParseDuration(avgText)
		if err != nil {
			return nil, err
		}
		summary.AvgTimeMs = float64(avg.Microseconds()) / microsecondsPerMillisecond

		if runsText, ok := fieldValue(line, "runs="); ok {
			runs, err := strconv.Atoi(runsText)
			if err != nil {
				return nil, err
			}
			summary.Runs = runs
		}
		summaries = append(summaries, summary)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(summaries) == 0 {
		return nil, fmt.Errorf("missing customization summary in %s", path)
	}
	return summaries, nil
}

func parseDurationAfter(text, marker string) (time.Duration, error) {
	idx := strings.Index(text, marker)
	if idx < 0 {
		return 0, fmt.Errorf("missing marker %q", marker)
	}
	start := idx + len(marker)
	end := strings.IndexAny(text[start:], " \r\n")
	if end < 0 {
		end = len(text[start:])
	}
	return time.ParseDuration(strings.TrimSpace(text[start : start+end]))
}

func gomaxprocsFromPath(path string) int {
	base := filepath.Base(path)
	digits := ""
	for _, ch := range base {
		if ch >= digitZero && ch <= digitNine {
			digits += string(ch)
		}
	}
	value, _ := strconv.Atoi(digits)
	return value
}

func fieldValue(line, prefix string) (string, bool) {
	for _, field := range strings.Fields(line) {
		if strings.HasPrefix(field, prefix) {
			return strings.TrimPrefix(field, prefix), true
		}
	}
	return "", false
}

func trimGOMAXPROCSSuffix(name string) string {
	if idx := strings.LastIndex(name, "-"); idx >= 0 {
		return name[:idx]
	}
	return name
}

func metricForMode(rows []compareRow, mode string) float64 {
	for _, row := range rows {
		if row.Mode == mode {
			return row.NsPerOp
		}
	}
	return 0
}

func ratioString(numerator, denominator float64) string {
	if numerator <= 0 || denominator <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.2fx", numerator/denominator)
}

func speedupString(baselineNs, currentNs float64) string {
	if baselineNs <= 0 || currentNs <= 0 {
		return "-"
	}
	return fmt.Sprintf("%.2fx", baselineNs/currentNs)
}

func readTextFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) >= utf16BOMSize {
		switch {
		case data[0] == 0xff && data[1] == 0xfe:
			return decodeUTF16(data[utf16BOMSize:], true), nil
		case data[0] == 0xfe && data[1] == 0xff:
			return decodeUTF16(data[utf16BOMSize:], false), nil
		}
	}
	if bytes.IndexByte(data, 0x00) >= 0 {
		return decodeUTF16(data, true), nil
	}
	return string(data), nil
}

func decodeUTF16(data []byte, littleEndian bool) string {
	if len(data)%utf16CodeUnitSize == 1 {
		data = data[:len(data)-1]
	}
	values := make([]uint16, 0, len(data)/utf16CodeUnitSize)
	for i := 0; i+1 < len(data); i += utf16CodeUnitSize {
		if littleEndian {
			values = append(values, uint16(data[i])|uint16(data[i+1])<<8)
		} else {
			values = append(values, uint16(data[i])<<8|uint16(data[i+1]))
		}
	}
	return string(utf16.Decode(values))
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, "error:", message)
	os.Exit(1)
}

func failf(format string, args ...any) {
	fail(fmt.Sprintf(format, args...))
}
