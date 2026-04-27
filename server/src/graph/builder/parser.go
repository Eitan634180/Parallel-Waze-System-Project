package builder

import (
	"context"
	"fmt"
	"log"
	"os"
	"runtime"
	"strings"

	"nav-system/src/graph/model"

	"github.com/RoaringBitmap/roaring/roaring64"
	osmlib "github.com/paulmach/osm"
	"github.com/paulmach/osm/osmpbf"
)

// NodeRawID is an OSM node ID.
type NodeRawID = uint64

// RawNode holds the coordinates of an input node referenced by at least one routable way.
type RawNode struct {
	ID  NodeRawID
	Lat float64
	Lon float64
}

// RawWay holds the parsed fields of an input way relevant for routing.
type RawWay struct {
	ID            uint64
	NodeRefs      []NodeRawID
	RoadClass     uint8
	IsOneWay      bool
	ReverseOneWay bool
	MaxSpeed      float32 // 0 means "use default for road class"
}

// ParseResult is the normalized graph-building input produced by source parsers.
type ParseResult struct {
	Nodes map[NodeRawID]*RawNode // keyed by source node ID
	Ways  []*RawWay
}

const importerLogPrefix = "importer:"

// ParsePBF reads an OpenStreetMap PBF file and extracts all nodes and ways
// needed to build a routable road graph.
//
// Pass 1: scan Ways, collect referenced NodeRawIDs and classify roads.
// Pass 2: scan Nodes, keep only those referenced in Pass 1.
func ParsePBF(path string) (*ParseResult, error) {
	procs := max(runtime.GOMAXPROCS(0), 1)

	log.Printf("%s pass 1: scanning ways", importerLogPrefix)

	needed := roaring64.New()
	var ways []*RawWay

	if err := scanPBF(path, procs, true, false, true, func(obj osmlib.Object) {
		w, ok := obj.(*osmlib.Way)
		if !ok {
			return
		}
		hw := w.Tags.Find("highway")
		if hw == "" {
			return
		}

		class, ok := model.HighwayClass[hw]
		if !ok || len(w.Nodes) < 2 {
			return
		}

		raw := &RawWay{
			ID:        uint64(w.ID),
			NodeRefs:  make([]uint64, len(w.Nodes)),
			RoadClass: class,
		}
		raw.IsOneWay, raw.ReverseOneWay = parseOneWay(w.Tags.Find("oneway"), w.Tags.Find("junction"))
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

	nodes := make(map[uint64]*RawNode, int(needed.GetCardinality()))
	if err := scanPBF(path, procs, false, true, true, func(obj osmlib.Object) {
		n, ok := obj.(*osmlib.Node)
		if !ok {
			return
		}
		nid := uint64(n.ID)
		if !needed.Contains(nid) {
			return
		}
		nodes[nid] = &RawNode{ID: nid, Lat: n.Lat, Lon: n.Lon}
	}); err != nil {
		return nil, fmt.Errorf("pass 2: %w", err)
	}

	log.Printf("%s pass 2 complete (%d nodes loaded)", importerLogPrefix, len(nodes))
	return &ParseResult{Nodes: nodes, Ways: ways}, nil
}

func parseOneWay(oneway, junction string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(oneway)) {
	case "yes", "true", "1":
		return true, false
	case "-1", "reverse":
		return true, true
	case "no", "false", "0":
		return false, false
	default:
		return strings.EqualFold(strings.TrimSpace(junction), "roundabout"), false
	}
}

func scanPBF(path string, procs int, skipNodes, skipWays, skipRelations bool, fn func(osmlib.Object)) error {
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
