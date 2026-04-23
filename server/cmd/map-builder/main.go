package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"strconv"
	"strings"
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
	cellSize := flag.Int("cell-size", 0, "Max nodes per cell (Inertial Flow)")
	workers := flag.Int("workers", defaultMapBuilderWorkers, "Deprecated: set process-wide GOMAXPROCS for map building")
	flag.Parse()

	resolvedPBFPath, err := resolvePBFPath(*pbfPath)
	if err != nil {
		log.Fatalf("resolve pbf path: %v", err)
	}
	if resolvedPBFPath == "" {
		fmt.Fprintln(os.Stderr, "error: --pbf is required or NAV_MAP_PBF_PATH/TEST_BENCH_PBF_PATH must be set")
		flag.Usage()
		os.Exit(1)
	}
	resolvedOutDir, err := resolveOutDir(*outDir)
	if err != nil {
		log.Fatalf("resolve out dir: %v", err)
	}
	resolvedCellSize, err := resolveCellSize(*cellSize)
	if err != nil {
		log.Fatalf("resolve cell size: %v", err)
	}

	if *workers > 0 {
		runtime.GOMAXPROCS(*workers)
	}
	parallelism := max(runtime.GOMAXPROCS(0), 1)

	total := time.Now()

	log.Printf("%s parsing %s", builderLogPrefix, resolvedPBFPath)
	t := time.Now()
	pr, err := builder.ParsePBF(resolvedPBFPath)
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

	log.Printf("%s partitioning cells (max size %d)", builderLogPrefix, resolvedCellSize)
	t = time.Now()
	builder.PartitionCells(g, resolvedCellSize)
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
		builderLogPrefix, time.Since(t).Round(time.Millisecond), len(g.Overlay.OverlayEdges))

	log.Printf("%s writing graph to %s", builderLogPrefix, resolvedOutDir)
	t = time.Now()
	if err := store.SaveGraph(g, resolvedOutDir); err != nil {
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
		len(g.Overlay.OverlayEdges),
	)
}

func resolvePBFPath(flagValue string) (string, error) {
	value := strings.TrimSpace(flagValue)
	if value != "" {
		return utilities.ResolveModulePath(value)
	}
	if value, ok := utilities.LookupEnvTrimmed("NAV_MAP_PBF_PATH"); ok {
		return utilities.ResolveModulePath(value)
	}
	if value, ok := utilities.LookupEnvTrimmed("TEST_BENCH_PBF_PATH"); ok {
		mapRoot, err := resolveMapRoot()
		if err != nil {
			return "", err
		}
		return utilities.ResolveMapPath(mapRoot, value), nil
	}
	return "", nil
}

func resolveOutDir(flagValue string) (string, error) {
	value := strings.TrimSpace(flagValue)
	if value != "" {
		return utilities.ResolveModulePath(value)
	}
	if value, ok := utilities.LookupEnvTrimmed("NAV_MAP_BUILD_OUT_DIR"); ok {
		return utilities.ResolveModulePath(value)
	}
	return resolveMapRoot()
}

func resolveCellSize(flagValue int) (int, error) {
	if flagValue > 0 {
		return flagValue, nil
	}
	if value, ok := utilities.LookupAnyEnvTrimmed("NAV_MAP_CELL_SIZE", "TEST_BENCH_CELL_SIZE"); ok {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed <= 0 {
			return 0, fmt.Errorf("invalid cell size %q", value)
		}
		return parsed, nil
	}
	return defaultMapBuilderCellSize, nil
}

func resolveMapRoot() (string, error) {
	return utilities.ResolveModulePath(utilities.RequireEnv("NAV_MAP_ROOT"))
}
