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
	"nav-system/src/traffic"
)

const customizationBenchLogPrefix = "customization-bench:"
const customizationObservedRatio = float32(2.0)

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

	log.Printf("%s loading graph from %s", customizationBenchLogPrefix, *dataDir)
	g, err := builder.LoadGraph(*dataDir)
	if err != nil {
		log.Fatalf("LoadGraph: %v", err)
	}
	log.Printf("%s graph ready (%d cells, %d boundary nodes, %d edges)", customizationBenchLogPrefix, len(g.Cells), len(g.BoundaryBaseIdxs), len(g.Edges))

	store := traffic.NewStore()
	dirtyEdges := seedDirtyStore(g, store)
	log.Printf("%s store ready (%d dirty edges)", customizationBenchLogPrefix, dirtyEdges)

	traffic.CustomizeOverlayWeights(g, store)
	log.Printf("%s finished initial customization", customizationBenchLogPrefix)

	for _, workers := range workerCounts {
		runtime.GOMAXPROCS(workers)

		var total time.Duration
		for run := 1; run <= *runs; run++ {
			store.RefillPendingForBenchmarks()
			start := time.Now()
			traffic.CustomizeOverlayWeights(g, store)
			elapsed := time.Since(start)
			total += elapsed
			log.Printf("%s workers=%d run=%d duration=%s", customizationBenchLogPrefix, workers, run, elapsed.Round(time.Millisecond))
		}

		avg := total / time.Duration(*runs)
		fmt.Printf("%s workers=%d avg=%s runs=%d\n", customizationBenchLogPrefix, workers, avg.Round(time.Millisecond), *runs)
	}
}

func seedDirtyStore(g *builder.Graph, store *traffic.Store) int {
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
