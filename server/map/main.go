// build-map serializes graph data structures to disk for fast startup.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"time"

	"nav-system/map/builder"
	"nav-system/map/importer"
)

/*
Parse program flags
Run build pipeline (parse -> build graph -> create cells -> build overlay -> save)
*/
func main() {
	pbfPath := flag.String("pbf", "", "Path to .osm.pbf file (required)")
	outDir := flag.String("out", "./map/data", "Output directory")
	cellSize := flag.Int("cell-size", 2000, "Max nodes per cell (Inertial Flow)")
	workers := flag.Int("workers", 0, "Goroutines for overlay construction (default: NumCPU)")
	flag.Parse()

	if *pbfPath == "" {
		fmt.Fprintln(os.Stderr, "error: --pbf is required")
		flag.Usage()
		os.Exit(1)
	}

	if *workers <= 0 {
		*workers = runtime.NumCPU()
	}

	total := time.Now()

	log.Printf("[BUILD] Parsing PBF: %s...", *pbfPath)
	t := time.Now()
	pr, err := importer.ParsePBF(*pbfPath)
	if err != nil {
		log.Fatalf("ParsePBF: %v", err)
	}
	log.Printf("[BUILD] Parsed in %s - %d nodes, %d ways",
		time.Since(t).Round(time.Millisecond), len(pr.Nodes), len(pr.Ways))

	log.Println("[BUILD] Building base graph...")
	t = time.Now()
	g, err := builder.BuildGraph(pr)
	if err != nil {
		log.Fatalf("BuildGraph: %v", err)
	}
	log.Printf("[BUILD] Base graph built in %s - %d nodes, %d edges",
		time.Since(t).Round(time.Millisecond), len(g.Nodes), len(g.Edges))

	log.Printf("[BUILD] Partitioning cells (maxCellSize=%d)...", *cellSize)
	t = time.Now()
	builder.PartitionCells(g, *cellSize)
	log.Printf("[BUILD] Partitioned in %s - %d cells",
		time.Since(t).Round(time.Millisecond), len(g.Cells))

	log.Println("[BUILD] Detecting boundary nodes...")
	t = time.Now()
	builder.DetectBoundaryNodes(g)
	log.Printf("[BUILD] Boundary nodes detected in %s - %d nodes",
		time.Since(t).Round(time.Millisecond), len(g.BoundaryNodes))

	log.Printf("[BUILD] Building overlay graph (%d workers)...", *workers)
	t = time.Now()
	builder.BuildOverlayGraph(g, *workers)
	log.Printf("[BUILD] Overlay built in %s - %d overlay edges",
		time.Since(t).Round(time.Millisecond), len(g.OverlayAdj.OverlayEdges))

	log.Printf("[SAVE] Serializing to %s...", *outDir)
	t = time.Now()
	if err := builder.SaveGraph(g, *outDir); err != nil {
		log.Fatalf("SaveGraph: %v", err)
	}
	log.Printf("[SAVE] Done in %s", time.Since(t).Round(time.Millisecond))

	log.Println("-----------------------------------------")
	log.Printf("Total time       : %s", time.Since(total).Round(time.Millisecond))
	log.Printf("Nodes            : %d", len(g.Nodes))
	log.Printf("Edges            : %d", len(g.Edges))
	log.Printf("Cells            : %d", len(g.Cells))
	log.Printf("Boundary nodes   : %d", len(g.BoundaryNodes))
	log.Printf("Overlay edges    : %d", len(g.OverlayAdj.OverlayEdges))
	log.Println("-----------------------------------------")
}
