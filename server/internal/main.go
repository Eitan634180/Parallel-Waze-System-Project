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
	"nav-system/internal/routing"
	"nav-system/internal/session"
	"nav-system/internal/simulation"
	"nav-system/internal/traffic"
	"nav-system/map/builder"
)

func main() {
	dataDir := flag.String("data", "./map/data", "Directory containing binary graph files")
	addr := flag.String("addr", ":8080", "HTTP listen address")
	flag.Parse()

	log.Printf("[main] Loading graph from %s...", *dataDir)
	t := time.Now()
	g, err := builder.LoadGraph(*dataDir)
	if err != nil {
		log.Fatalf("LoadGraph: %v", err)
	}
	log.Printf("[main] Graph ready in %s - %d nodes, %d edges, %d cells, %d boundary nodes, %d overlay edges",
		time.Since(t).Round(time.Millisecond),
		len(g.Nodes), len(g.Edges), len(g.Cells),
		len(g.BoundaryNodes), len(g.OverlayAdj.OverlayEdges))

	log.Println("[main] Building spatial snap index...")
	t = time.Now()
	si := routing.BuildSnapIndex(g)
	log.Printf("[main] Snap index built in %s", time.Since(t).Round(time.Millisecond))

	store := traffic.NewStore()
	mgr := session.NewManager()
	sim := simulation.NewManager(g, store)
	router := routing.NewRouter(g, si)
	srv := api.NewServer(g, store, mgr, router, sim)

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
		log.Printf("[main] Listening on %s", *addr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[main] Shutting down...")
	cancel()
	shutCtx, shutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutCancel()
	_ = httpSrv.Shutdown(shutCtx)
	log.Println("[main] Goodbye.")
}
