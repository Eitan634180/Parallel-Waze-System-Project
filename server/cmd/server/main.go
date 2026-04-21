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

	"nav-system/src/graph/model"
	graphstore "nav-system/src/graph/store"
	navigationmanager "nav-system/src/navigation/manager"
	navigationmonitor "nav-system/src/navigation/monitor"
	"nav-system/src/regions"
	"nav-system/src/routing"
	"nav-system/src/simulation"
	"nav-system/src/transport"
	trafficcustomization "nav-system/src/traffic/customization"
	trafficstore "nav-system/src/traffic/store"
)

const serverLogPrefix = "server:"

func main() {
	dataDir := flag.String("data", "", "Directory containing binary graph files")
	addr := flag.String("addr", ":8080", "HTTP listen address")
	routingModeFlag := flag.String("routing-mode", string(routing.RoutingModeHierarchical), "Routing mode: hierarchical|base-astar|base-dijkstra")
	flag.Parse()

	routingMode, err := routing.ParseRoutingMode(*routingModeFlag)
	if err != nil {
		log.Fatalf("ParseRoutingMode: %v", err)
	}

	if *dataDir == "" {
		mapRoot := filepath.Join(".", "data", "map")
		availableRegions, err := regions.ListReady(mapRoot)
		if err != nil {
			log.Fatalf("scanning map directory: %v", err)
		}
		switch len(availableRegions) {
		case 0:
			log.Fatalf("%s no preprocessed regions found in %s. Run region-picker first.", serverLogPrefix, mapRoot)
		case 1:
			*dataDir = availableRegions[0].Dir
			log.Printf("%s auto-selected region: %s", serverLogPrefix, availableRegions[0].ID)
		default:
			log.Printf("%s multiple regions available in %s:", serverLogPrefix, mapRoot)
			for _, r := range availableRegions {
				log.Printf("%s   %s", serverLogPrefix, r.ID)
			}
			log.Fatalf("%s specify --data <dir> to choose a region", serverLogPrefix)
		}
	}

	log.Printf("%s loading graph from %s", serverLogPrefix, *dataDir)
	t := time.Now()
	g, err := graphstore.LoadGraph(*dataDir)
	if err != nil {
		log.Fatalf("LoadGraph: %v", err)
	}
	log.Printf("%s graph ready in %s (%d nodes, %d edges, %d cells, %d boundary nodes, %d overlay edges)",
		serverLogPrefix,
		time.Since(t).Round(time.Millisecond),
		len(g.Nodes), len(g.Edges), len(g.Cells),
		len(g.BoundaryBaseIdxs), len(g.OverlayAdj.OverlayEdges))

	log.Printf("%s building snap index", serverLogPrefix)
	t = time.Now()
	si := routing.BuildSnapIndex(g)
	log.Printf("%s snap index ready in %s", serverLogPrefix, time.Since(t).Round(time.Millisecond))

	store := trafficstore.NewStore()
	mgr := navigationmanager.NewManager()
	router := routing.NewRouterWithMode(g, si, routingMode)
	sim := simulation.NewManager(g, store, router, func() routing.WeightFunc {
		return func(e *model.Edge) float32 {
			return store.LiveWeight(e.ID, e.Weight)
		}
	})
	log.Printf("%s routing mode: %s", serverLogPrefix, routingMode)
	trafficcustomization.CustomizeOverlayWeights(g, store)
	srv := transport.NewServer(g, store, mgr, router, sim)

	warmCtx, warmCancel := context.WithTimeout(context.Background(), 3*time.Second)
	if err := srv.WarmSearch(warmCtx); err != nil {
		log.Printf("%s search warmup skipped: %v", serverLogPrefix, err)
	} else {
		log.Printf("%s search warmup complete", serverLogPrefix)
	}
	warmCancel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go trafficstore.Worker(ctx, store)
	go trafficcustomization.RunCustomization(ctx, g, store)
	go mgr.RunExpiry(ctx)
	go navigationmonitor.RunPropagation(ctx, mgr, store, g)
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
