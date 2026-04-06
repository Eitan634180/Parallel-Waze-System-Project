//go:build !cgo
// +build !cgo

package importer

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"

	"nav-system/internal/graph/builder"

	"github.com/RoaringBitmap/roaring/roaring64"
	"github.com/paulmach/osm"
	"github.com/paulmach/osm/osmpbf"
)

const importerLogPrefix = "importer:"

var highwayClass = map[string]uint8{
	"motorway":       builder.RoadMotorway,
	"motorway_link":  builder.RoadMotorway,
	"trunk":          builder.RoadTrunk,
	"trunk_link":     builder.RoadTrunk,
	"primary":        builder.RoadPrimary,
	"primary_link":   builder.RoadPrimary,
	"secondary":      builder.RoadSecondary,
	"secondary_link": builder.RoadSecondary,
	"tertiary":       builder.RoadTertiary,
	"tertiary_link":  builder.RoadTertiary,
	"residential":    builder.RoadResidential,
	"unclassified":   builder.RoadUnclassified,
	"service":        builder.RoadService,
}

func isRoutable(highwayVal string) uint8 {
	c, ok := highwayClass[highwayVal]
	if !ok {
		return 0
	}
	return c
}

// ParsePBF reads an OpenStreetMap PBF file and extracts all nodes and ways
// needed to build a routable road graph.
//
// Pass 1: scan Ways, collect referenced NodeIDs and classify roads.
// Pass 2: scan Nodes, keep only those referenced in Pass 1.
func ParsePBF(path string) (*builder.ParseResult, error) {
	procs := runtime.NumCPU()

	log.Printf("%s pass 1: scanning ways", importerLogPrefix)

	needed := roaring64.New()
	var ways []*builder.RawWay

	if err := scanPBF(path, procs, true, false, true, func(obj osm.Object) {
		w, ok := obj.(*osm.Way)
		if !ok {
			return
		}
		hw := w.Tags.Find("highway")
		if hw == "" {
			return
		}
		class := isRoutable(hw)
		if class == 0 || len(w.Nodes) < 2 {
			return
		}

		raw := &builder.RawWay{
			ID:        uint64(w.ID),
			NodeRefs:  make([]uint64, len(w.Nodes)),
			RoadClass: class,
			IsOneWay:  w.Tags.Find("oneway") == "yes",
		}
		for i, wn := range w.Nodes {
			raw.NodeRefs[i] = uint64(wn.ID)
			needed.Add(uint64(wn.ID))
		}

		if ms := w.Tags.Find("maxspeed"); ms != "" {
			var v float64
			if _, err := fmt.Sscanf(ms, "%f", &v); err == nil && v > 0 {
				raw.MaxSpeed = float32(v)
			}
		}

		ways = append(ways, raw)
	}); err != nil {
		return nil, fmt.Errorf("pass 1: %w", err)
	}

	log.Printf("%s pass 1 complete (%d routable ways, %d unique node refs)", importerLogPrefix, len(ways), needed.GetCardinality())
	log.Printf("%s pass 2: scanning nodes", importerLogPrefix)

	nodes := make(map[uint64]*builder.RawNode, int(needed.GetCardinality()))
	if err := scanPBF(path, procs, false, true, true, func(obj osm.Object) {
		n, ok := obj.(*osm.Node)
		if !ok {
			return
		}
		nid := uint64(n.ID)
		if !needed.Contains(nid) {
			return
		}
		nodes[nid] = &builder.RawNode{ID: nid, Lat: n.Lat, Lon: n.Lon}
	}); err != nil {
		return nil, fmt.Errorf("pass 2: %w", err)
	}

	log.Printf("%s pass 2 complete (%d nodes loaded)", importerLogPrefix, len(nodes))
	return &builder.ParseResult{Nodes: nodes, Ways: ways}, nil
}

func scanPBF(path string, procs int, skipNodes, skipWays, skipRelations bool, fn func(osm.Object)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := osmpbf.New(context.Background(), f, procs)
	defer scanner.Close()

	scanner.SkipNodes = skipNodes
	scanner.SkipWays = skipWays
	scanner.SkipRelations = skipRelations

	for scanner.Scan() {
		fn(scanner.Object())
	}
	return scanner.Err()
}
