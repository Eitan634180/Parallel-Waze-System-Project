package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"nav-system/src/graph/builder"
	"nav-system/src/graph/store"
	"nav-system/src/utilities"
)

const builderLogPrefix = "builder:"

const (
	defaultMapBuilderCellSize = 2000
	defaultMapBuilderWorkers  = 0
)

func main() {
	pbfPath := flag.String("pbf", "", "Path to .osm.pbf file (required)")
	outDir := flag.String("out", "", "Output directory")
	cellSize := flag.Int("cell-size", defaultMapBuilderCellSize, "Max nodes per cell (Inertial Flow)")
	workers := flag.Int("workers", defaultMapBuilderWorkers, "Deprecated: set process-wide GOMAXPROCS for map building")
	flag.Parse()

	if *pbfPath == "" {
		fmt.Fprintln(os.Stderr, "error: --pbf is required")
		flag.Usage()
		os.Exit(1)
	}
	if *outDir == "" {
		*outDir = utilities.RequireEnv("NAV_MAP_ROOT")
	}

	if *workers > 0 {
		runtime.GOMAXPROCS(*workers)
	}
	parallelism := max(runtime.GOMAXPROCS(0), 1)

	total := time.Now()

	log.Printf("%s parsing %s", builderLogPrefix, *pbfPath)
	t := time.Now()
	pr, err := builder.ParsePBF(*pbfPath)
	if err != nil {
		log.Fatalf("ParsePBF: %v", err)
	}
	log.Printf("%s parsed input in %s (%d nodes, %d ways)",
		builderLogPrefix, time.Since(t).Round(time.Millisecond), len(pr.Nodes), len(pr.Ways))

	log.Printf("%s building base graph", builderLogPrefix)
	t = time.Now()
	g, err := builder.BuildBaseGraph(pr)
	if err != nil {
		log.Fatalf("BuildGraph: %v", err)
	}
	log.Printf("%s base graph ready in %s (%d nodes, %d edges)",
		builderLogPrefix, time.Since(t).Round(time.Millisecond), len(g.Nodes), len(g.Edges))

	log.Printf("%s partitioning cells (max size %d)", builderLogPrefix, *cellSize)
	t = time.Now()
	builder.PartitionCells(g, *cellSize)
	log.Printf("%s partitioned graph in %s (%d cells)",
		builderLogPrefix, time.Since(t).Round(time.Millisecond), len(g.Cells))

	log.Printf("%s detecting boundary nodes", builderLogPrefix)
	t = time.Now()
	builder.DetectBoundaryNodes(g)
	log.Printf("%s boundary nodes ready in %s (%d nodes)",
		builderLogPrefix, time.Since(t).Round(time.Millisecond), len(g.BoundaryBaseIdxs))

	log.Printf("%s using GOMAXPROCS=%d", builderLogPrefix, parallelism)
	log.Printf("%s building overlay graph", builderLogPrefix)
	t = time.Now()
	builder.BuildOverlayGraph(g, 0)
	log.Printf("%s overlay ready in %s (%d edges)",
		builderLogPrefix, time.Since(t).Round(time.Millisecond), len(g.OverlayAdj.OverlayEdges))

	log.Printf("%s writing graph to %s", builderLogPrefix, *outDir)
	t = time.Now()
	if err := store.SaveGraph(g, *outDir); err != nil {
		log.Fatalf("SaveGraph: %v", err)
	}
	log.Printf("%s graph written in %s", builderLogPrefix, time.Since(t).Round(time.Millisecond))
	log.Printf("%s build complete in %s (%d nodes, %d edges, %d cells, %d boundary nodes, %d overlay edges)",
		builderLogPrefix,
		time.Since(total).Round(time.Millisecond),
		len(g.Nodes),
		len(g.Edges),
		len(g.Cells),
		len(g.BoundaryBaseIdxs),
		len(g.OverlayAdj.OverlayEdges),
	)
}
