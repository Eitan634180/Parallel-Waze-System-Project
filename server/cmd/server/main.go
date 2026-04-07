// server is the HTTP/WebSocket navigation server.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"nav-system/src/api"
	"nav-system/src/graph/builder"
	"nav-system/src/mapstore"
	"nav-system/src/routing"
	"nav-system/src/session"
	"nav-system/src/simulation"
	"nav-system/src/traffic"
)

const serverLogPrefix = "server:"

func main() {
	dataDir := flag.String("data", "", "Directory containing binary graph files (auto-detected if empty)")
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	// If --data is not specified, find the region automatically.
	if *dataDir == "" {
		mapRoot := filepath.Join(".", "data", "map")
		regions, err := mapstore.ListReady(mapRoot)
		if err != nil {
			log.Fatalf("scanning map directory: %v", err)
		}
		switch len(regions) {
		case 0:
			log.Fatalf("%s no preprocessed regions found in %s. Run region-picker first.", serverLogPrefix, mapRoot)
		case 1:
			*dataDir = regions[0].Dir
			log.Printf("%s auto-selected region: %s", serverLogPrefix, regions[0].ID)
		default:
			log.Printf("%s multiple regions available in %s:", serverLogPrefix, mapRoot)
			for _, r := range regions {
				log.Printf("%s   %s", serverLogPrefix, r.ID)
			}
			log.Fatalf("%s specify --data <dir> to choose a region", serverLogPrefix)
		}
	}

	log.Printf("%s loading graph from %s", serverLogPrefix, *dataDir)
	t := time.Now()
	g, err := builder.LoadGraph(*dataDir)
	if err != nil {
		log.Fatalf("LoadGraph: %v", err)
	}
	log.Printf("%s graph ready in %s (%d nodes, %d edges, %d cells, %d boundary nodes, %d overlay edges)",
		serverLogPrefix,
		time.Since(t).Round(time.Millisecond),
		len(g.Nodes), len(g.Edges), len(g.Cells),
		len(g.BoundaryNodes), len(g.OverlayAdj.OverlayEdges))

	log.Printf("%s building snap index", serverLogPrefix)
	t = time.Now()
	si := routing.BuildSnapIndex(g)
	log.Printf("%s snap index ready in %s", serverLogPrefix, time.Since(t).Round(time.Millisecond))

	store := traffic.NewStore()
	mgr := session.NewManager()
	sim := simulation.NewManager(g, store)
	router := routing.NewRouter(g, si)
	traffic.CustomizeOverlayWeights(g, store)
	srv := api.NewServer(g, store, mgr, router, sim)

	warmCtx, warmCancel := context.WithTimeout(context.Background(), 3*time.Second)
	if err := srv.WarmSearch(warmCtx); err != nil {
		log.Printf("%s search warmup skipped: %v", serverLogPrefix, err)
	} else {
		log.Printf("%s search warmup complete", serverLogPrefix)
	}
	warmCancel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go traffic.Worker(ctx, store)
	go traffic.RunCustomization(ctx, g, store)
	go mgr.RunExpiry(ctx)
	go mgr.RunPropagation(ctx, store, g)
	go srv.RunOptimizationSweep(ctx)
	go srv.RunRouteCacheGC(ctx)
	go sim.Run(ctx)

	httpSrv := &http.Server{
		Addr:    *addr,
		Handler: srv,
	}

	go func() {
		log.Printf("%s listening on %s", serverLogPrefix, *addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Printf("%s shutting down", serverLogPrefix)
	cancel()
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	_ = httpSrv.Shutdown(shutCtx)
	log.Printf("%s stopped", serverLogPrefix)
}
