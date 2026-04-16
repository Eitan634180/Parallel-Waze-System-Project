package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"
)

type goBenchmarkReport struct {
	GoOS       string
	GoArch     string
	CPU        string
	Benchmarks map[string]map[string]float64
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

func main() {
	dir := flag.String("dir", "", "Benchmark output directory")
	flag.Parse()

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

	if writeGoBenchmarkSection(&builder, "Single-Query Static Comparison", filepath.Join(reportDir, "compare-static.txt"), "BenchmarkRouterCompareStatic"); err != nil {
		builder.WriteString(fmt.Sprintf("_Static comparison unavailable: %v_\n\n", err))
	}
	if writeServerScaleSection(&builder, filepath.Join(reportDir, "server-scale")); err != nil {
		builder.WriteString(fmt.Sprintf("_Server-scale results unavailable: %v_\n\n", err))
	}
	if writeBuildScaleSection(&builder, filepath.Join(reportDir, "build-scale")); err != nil {
		builder.WriteString(fmt.Sprintf("_Build-scale results unavailable: %v_\n\n", err))
	}
	if writeOverlayScaleSection(&builder, filepath.Join(reportDir, "overlay-scale")); err != nil {
		builder.WriteString(fmt.Sprintf("_Overlay-scale results unavailable: %v_\n\n", err))
	}
	if writeCustomizationScaleSection(&builder, filepath.Join(reportDir, "customization-scale")); err != nil {
		builder.WriteString(fmt.Sprintf("_Customization-scale results unavailable: %v_\n\n", err))
	}

	summaryPath := filepath.Join(reportDir, "summary.md")
	if err := os.WriteFile(summaryPath, []byte(builder.String()), 0o644); err != nil {
		failf("write summary: %v", err)
	}

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
			row.NsPerOp/1_000_000.0,
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
			if len(fields) < 4 {
				continue
			}
			metrics := make(map[string]float64)
			for i := 2; i+1 < len(fields); i += 2 {
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
		OverlayTimeMs: float64(overlay.Microseconds()) / 1000.0,
		TotalTimeMs:   float64(total.Microseconds()) / 1000.0,
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
		if !strings.HasPrefix(line, "overlay-bench: workers=") || !strings.Contains(line, " avg=") {
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
		summary.AvgTimeMs = float64(avg.Microseconds()) / 1000.0

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
		if !strings.HasPrefix(line, "customization-bench: workers=") || !strings.Contains(line, " avg=") {
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
		summary.AvgTimeMs = float64(avg.Microseconds()) / 1000.0

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
		if ch >= '0' && ch <= '9' {
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
	if len(data) >= 2 {
		switch {
		case data[0] == 0xff && data[1] == 0xfe:
			return decodeUTF16(data[2:], true), nil
		case data[0] == 0xfe && data[1] == 0xff:
			return decodeUTF16(data[2:], false), nil
		}
	}
	if bytes.IndexByte(data, 0x00) >= 0 {
		return decodeUTF16(data, true), nil
	}
	return string(data), nil
}

func decodeUTF16(data []byte, littleEndian bool) string {
	if len(data)%2 == 1 {
		data = data[:len(data)-1]
	}
	values := make([]uint16, 0, len(data)/2)
	for i := 0; i+1 < len(data); i += 2 {
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
