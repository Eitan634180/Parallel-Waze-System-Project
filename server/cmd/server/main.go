// server is the HTTP/WebSocket navigation server.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"nav-system/internal/api"
	"nav-system/internal/graph/builder"
	"nav-system/internal/routing"
	"nav-system/internal/session"
	"nav-system/internal/simulation"
	"nav-system/internal/traffic"
)

const serverLogPrefix = "server:"

func main() {
	dataDir := flag.String("data", "./data/map", "Directory containing binary graph files")
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

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
