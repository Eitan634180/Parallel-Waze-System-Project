package builder

import (
	"context"
	"fmt"
	"math"
	"os"
	"runtime"

	"github.com/paulmach/osm"
	"github.com/paulmach/osm/osmpbf"
)

// ---------------------------------------------------------------------------
// Road classification helpers
// ---------------------------------------------------------------------------

var highwayClass = map[string]uint8{
	"motorway":       1,
	"motorway_link":  1,
	"trunk":          2,
	"trunk_link":     2,
	"primary":        3,
	"primary_link":   3,
	"secondary":      4,
	"secondary_link": 4,
	"tertiary":       5,
	"tertiary_link":  5,
	"residential":    6,
	"unclassified":   8,
	"service":        7,
}

// IsRoutable returns the road class (1–8) for a highway tag value,
// or 0 if the road is not considered drivable.
func IsRoutable(highwayVal string) uint8 {
	c, ok := highwayClass[highwayVal]
	if !ok {
		return 0
	}
	return c
}

// ---------------------------------------------------------------------------
// Output types (raw OSM data, before graph construction)
// ---------------------------------------------------------------------------

// RawNode holds the coordinates of an OSM node referenced by at least one routable way.
type RawNode struct {
	ID  uint64
	Lat float64
	Lon float64
}

// RawWay holds the parsed fields of an OSM way relevant for routing.
type RawWay struct {
	ID        uint64
	NodeRefs  []uint64
	RoadClass uint8
	IsOneWay  bool
	MaxSpeed  float32 // 0 means "use default for road class"
}

// ParseResult is returned by the PBF parser.
type ParseResult struct {
	Nodes map[uint64]*RawNode // keyed by OSM node ID
	Ways  []*RawWay
}

// ---------------------------------------------------------------------------
// PBF parser — two-pass streaming using paulmach/osm/osmpbf Scanner
// ---------------------------------------------------------------------------

// ParsePBF reads an OpenStreetMap PBF file and extracts all nodes and ways
// needed to build a routable road graph.
//
// Pass 1: scan Ways, collect referenced NodeIDs and classify roads.
// Pass 2: scan Nodes, keep only those referenced in Pass 1.
func ParsePBF(path string) (*ParseResult, error) {
	procs := runtime.NumCPU()

	// ── Pass 1: collect routable ways and referenced node IDs ──────────────
	fmt.Println("[OSM] Pass 1: scanning ways …")

	needed := make(map[uint64]struct{})
	var ways []*RawWay

	if err := scanPBF(path, procs, func(obj osm.Object) {
		w, ok := obj.(*osm.Way)
		if !ok {
			return
		}
		hw := w.Tags.Find("highway")
		if hw == "" {
			return
		}
		class := IsRoutable(hw)
		if class == 0 || len(w.Nodes) < 2 {
			return
		}

		raw := &RawWay{
			ID:        uint64(w.ID),
			NodeRefs:  make([]uint64, len(w.Nodes)),
			RoadClass: class,
			IsOneWay:  w.Tags.Find("oneway") == "yes",
		}
		for i, wn := range w.Nodes {
			raw.NodeRefs[i] = uint64(wn.ID)
			needed[uint64(wn.ID)] = struct{}{}
		}

		// Parse maxspeed tag
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

	fmt.Printf("[OSM] Pass 1 done: %d routable ways, %d unique node refs\n", len(ways), len(needed))

	// ── Pass 2: collect coordinates for needed nodes ────────────────────────
	fmt.Println("[OSM] Pass 2: scanning nodes …")

	nodes := make(map[uint64]*RawNode, len(needed))

	if err := scanPBF(path, procs, func(obj osm.Object) {
		n, ok := obj.(*osm.Node)
		if !ok {
			return
		}
		nid := uint64(n.ID)
		if _, want := needed[nid]; !want {
			return
		}
		nodes[nid] = &RawNode{ID: nid, Lat: n.Lat, Lon: n.Lon}
	}); err != nil {
		return nil, fmt.Errorf("pass 2: %w", err)
	}

	fmt.Printf("[OSM] Pass 2 done: %d nodes loaded\n", len(nodes))
	return &ParseResult{Nodes: nodes, Ways: ways}, nil
}

// ---------------------------------------------------------------------------
// Internal: generic PBF scanner using paulmach/osm/osmpbf.Scanner
// ---------------------------------------------------------------------------

func scanPBF(path string, procs int, fn func(osm.Object)) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := osmpbf.New(context.Background(), f, procs)
	defer scanner.Close()

	for scanner.Scan() {
		fn(scanner.Object())
	}
	return scanner.Err()
}

// ---------------------------------------------------------------------------
// Coordinate projection (equirectangular)
// ---------------------------------------------------------------------------

const (
	earthRadiusM = 6_371_000.0
	// lat0Deg is the reference latitude for the equirectangular projection.
	// 31.5° is approximately the geographic centre of Israel.
	lat0Deg = 31.5
)

var cosLat0 = math.Cos(lat0Deg * math.Pi / 180.0)

// ProjectXY converts WGS-84 lat/lon to flat (X, Y) in metres using an
// equirectangular projection centred on lat0.
func ProjectXY(lat, lon float64) (x, y float32) {
	x = float32(lon * math.Pi / 180.0 * cosLat0 * earthRadiusM)
	y = float32(lat * math.Pi / 180.0 * earthRadiusM)
	return
}

// HaversineM returns the great-circle distance in metres between two WGS-84 coords.
func HaversineM(lat1, lon1, lat2, lon2 float64) float64 {
	const r = earthRadiusM
	rad := math.Pi / 180.0
	dLat := (lat2 - lat1) * rad
	dLon := (lon2 - lon1) * rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*rad)*math.Cos(lat2*rad)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	return r * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
