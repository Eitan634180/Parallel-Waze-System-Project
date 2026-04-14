package main

import (
	"flag"
	"fmt"
	"log"
	"runtime"
	"strconv"
	"strings"
	"time"

	"nav-system/src/graph/builder"
)

const overlayBenchLogPrefix = "overlay-bench:"

func main() {
	dataDir := flag.String("data", "./data/map", "Path to saved graph directory")
	workersFlag := flag.String("workers", "1", "Comma-separated worker counts")
	runs := flag.Int("runs", 5, "Timed runs per worker count")
	flag.Parse()

	if *runs <= 0 {
		log.Fatalf("runs must be > 0")
	}

	workerCounts := parseWorkerCounts(*workersFlag)
	originalGOMAXPROCS := runtime.GOMAXPROCS(0)
	defer runtime.GOMAXPROCS(originalGOMAXPROCS)

	log.Printf("%s loading graph from %s", overlayBenchLogPrefix, *dataDir)
	g, err := builder.LoadGraph(*dataDir)
	if err != nil {
		log.Fatalf("LoadGraph: %v", err)
	}
	log.Printf("%s graph ready (%d cells, %d boundary nodes)", overlayBenchLogPrefix, len(g.Cells), len(g.BoundaryNodes))

	for _, workers := range workerCounts {
		var total time.Duration
		for run := 1; run <= *runs; run++ {
			runtime.GOMAXPROCS(workers)
			elapsed := builder.BuildOverlayGraph(g, workers)
			total += elapsed
			log.Printf("%s workers=%d run=%d duration=%s", overlayBenchLogPrefix, workers, run, elapsed.Round(time.Millisecond))
		}

		avg := total / time.Duration(*runs)
		fmt.Printf("%s workers=%d avg=%s runs=%d\n", overlayBenchLogPrefix, workers, avg.Round(time.Millisecond), *runs)
	}
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
