// server is the HTTP/WebSocket navigation server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"nav-system/src/graph/model"
	graphstore "nav-system/src/graph/store"
	navigationsessions "nav-system/src/navigation/sessions"
	navigationworkers "nav-system/src/navigation/workers"
	routingengine "nav-system/src/routing/engine"
	routingentities "nav-system/src/routing/entities"
	"nav-system/src/simulation"
	trafficstore "nav-system/src/traffic/store"
	"nav-system/src/transport"
	"nav-system/src/utilities"
)

const serverLogPrefix = "server:"

func main() {
	dataDir := flag.String("data", "", "Directory containing binary graph files")
	addr := flag.String("addr", "", "HTTP listen address")
	routingModeFlag := flag.String("routing-mode", "", "Routing mode: hierarchical|base-astar|base-dijkstra")
	flag.Parse()

	resolvedAddr, err := resolveListenAddr(*addr)
	if err != nil {
		log.Fatalf("%s resolve listen addr: %v", serverLogPrefix, err)
	}

	routingMode, err := routingentities.ParseRoutingMode(resolveRoutingMode(*routingModeFlag))
	if err != nil {
		log.Fatalf("ParseRoutingMode: %v", err)
	}

	resolvedDataDir, err := resolveDataDir(*dataDir)
	if err != nil {
		log.Fatalf("%s resolve data dir: %v", serverLogPrefix, err)
	}

	log.Printf("%s loading graph from %s", serverLogPrefix, resolvedDataDir)
	t := time.Now()
	g, err := graphstore.LoadGraph(resolvedDataDir)
	if err != nil {
		log.Fatalf("LoadGraph: %v", err)
	}
	log.Printf("%s graph ready in %s (%d nodes, %d edges, %d cells, %d gate nodes, %d overlay edges)",
		serverLogPrefix,
		time.Since(t).Round(time.Millisecond),
		len(g.Nodes), len(g.Edges), len(g.Cells),
		len(g.Overlay.Offsets)-1, len(g.Overlay.OverlayEdges))

	log.Printf("%s building snap index", serverLogPrefix)
	t = time.Now()
	si := routingengine.BuildSnapIndex(g)
	log.Printf("%s snap index ready in %s", serverLogPrefix, time.Since(t).Round(time.Millisecond))

	store := trafficstore.NewStoreWithCapacity(len(g.Edges))
	store.InitOverlayWeights(g)
	customizer := trafficstore.NewCustomizer(g)
	mgr := navigationsessions.NewManager()
	router := routingengine.NewRouterWithMode(g, si, routingMode)
	router.SetOverlayWeightFunc(func(edgeIdx uint32, overlayEdge *model.OverlayEdge) float32 {
		return store.OverlayWeight(edgeIdx, overlayEdge)
	})
	sim := simulation.NewManager(g, store, router, func() routingengine.WeightFunc {
		return func(e *model.Edge) float32 {
			return store.LiveWeight(e.ID, e.BaseWeight)
		}
	})
	log.Printf("%s routing mode: %s", serverLogPrefix, routingMode)
	customizer.Customize(store)
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
	go customizer.Run(ctx, store)
	go mgr.RunExpiry(ctx)
	go navigationworkers.RunPropagation(ctx, mgr, store, g)
	go srv.RunOptimizationSweep(ctx)
	go srv.RunRouteCacheGC(ctx)
	go sim.Run(ctx)

	httpSrv := &http.Server{
		Addr:    resolvedAddr,
		Handler: srv,
	}

	go func() {
		log.Printf("%s listening on %s", serverLogPrefix, resolvedAddr)
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

func resolveListenAddr(flagValue string) (string, error) {
	if value := strings.TrimSpace(flagValue); value != "" {
		return value, nil
	}
	if value, ok := utilities.LookupEnvTrimmed("NAV_SERVER_ADDR"); ok {
		return value, nil
	}

	host, ok := utilities.LookupEnvTrimmed("TEST_HOST")
	if !ok {
		return "", fmt.Errorf("NAV_SERVER_ADDR must be set, or TEST_HOST with TEST_SERVER_PORT/TEST_BENCH_SERVER_PORT must be configured")
	}
	if port, ok := utilities.LookupEnvTrimmed("TEST_BENCH_SERVER_PORT"); ok {
		return net.JoinHostPort(host, port), nil
	}
	if port, ok := utilities.LookupEnvTrimmed("TEST_SERVER_PORT"); ok {
		return net.JoinHostPort(host, port), nil
	}
	return "", fmt.Errorf("NAV_SERVER_ADDR must be set, or TEST_HOST with TEST_SERVER_PORT/TEST_BENCH_SERVER_PORT must be configured")
}

func resolveRoutingMode(flagValue string) string {
	if value := strings.TrimSpace(flagValue); value != "" {
		return value
	}
	if value, ok := utilities.LookupAnyEnvTrimmed(
		"NAV_SERVER_ROUTING_MODE",
		"DEV_ROUTING_MODE",
		"TEST_BENCH_ROUTING_MODE",
		"TEST_ROUTING_MODE",
	); ok {
		return value
	}
	return string(routingentities.RoutingModeHierarchical)
}

func resolveDataDir(flagValue string) (string, error) {
	if value := strings.TrimSpace(flagValue); value != "" {
		return utilities.ResolveModulePath(value)
	}
	if value, ok := utilities.LookupEnvTrimmed("NAV_SERVER_DATA_DIR"); ok {
		return utilities.ResolveModulePath(value)
	}
	if value, ok := utilities.LookupAnyEnvTrimmed("DEV_REGION_DIR", "TEST_REGION_DIR"); ok {
		mapRoot, err := resolveMapRoot()
		if err != nil {
			return "", err
		}
		return utilities.ResolveMapPath(mapRoot, value), nil
	}
	return "", fmt.Errorf("NAV_SERVER_DATA_DIR or DEV_REGION_DIR or TEST_REGION_DIR must be set")
}

func resolveMapRoot() (string, error) {
	mapRoot := utilities.RequireEnv("NAV_MAP_ROOT")
	return utilities.ResolveModulePath(mapRoot)
}
