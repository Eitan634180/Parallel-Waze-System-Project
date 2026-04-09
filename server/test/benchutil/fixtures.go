package benchutil

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"

	"nav-system/src/graph/builder"
	"nav-system/src/routing"
	"nav-system/src/traffic"
)

type CorpusSpec struct {
	Region          string `json:"region"`
	Seed            int64  `json:"seed"`
	Count           int    `json:"count"`
	MinNodeIndexGap int    `json:"min_node_index_gap"`
	MaxAttempts     int    `json:"max_attempts"`
}

type TrafficProfileSpec struct {
	Region            string  `json:"region"`
	Seed              int64   `json:"seed"`
	EdgeCount         int     `json:"edge_count"`
	MinMultiplier     float32 `json:"min_multiplier"`
	MaxMultiplier     float32 `json:"max_multiplier"`
	MinEdgeDistanceM  float32 `json:"min_edge_distance_m"`
}

type CorpusCase struct {
	Name   string
	SrcIdx uint32
	DstIdx uint32
	SrcLat float64
	SrcLon float64
	DstLat float64
	DstLon float64
}

type Fixture struct {
	Region         string
	Graph          *builder.Graph
	Snap           *routing.SnapIndex
	Store          *traffic.Store
	Corpus         []CorpusCase
	TrafficProfile string
}

func LoadFixture(corpusName, trafficName string) (*Fixture, error) {
	corpusSpec, err := loadJSON[CorpusSpec](filepath.Join(testdataRoot(), "benchmark-cases", corpusName))
	if err != nil {
		return nil, err
	}

	graph, err := builder.LoadGraph(filepath.Join(moduleRoot(), "data", "map", corpusSpec.Region))
	if err != nil {
		return nil, fmt.Errorf("load graph %q: %w", corpusSpec.Region, err)
	}

	corpus, err := BuildCorpus(graph, corpusSpec)
	if err != nil {
		return nil, err
	}

	store := traffic.NewStore()
	profileName := "static"
	if trafficName != "" {
		trafficSpec, err := loadJSON[TrafficProfileSpec](filepath.Join(testdataRoot(), "benchmark-traffic", trafficName))
		if err != nil {
			return nil, err
		}
		if trafficSpec.Region != corpusSpec.Region {
			return nil, fmt.Errorf("traffic profile region %q does not match corpus region %q", trafficSpec.Region, corpusSpec.Region)
		}
		if err := ApplyTrafficProfile(graph, store, trafficSpec); err != nil {
			return nil, err
		}
		profileName = trafficName
	}

	return &Fixture{
		Region:         corpusSpec.Region,
		Graph:          graph,
		Snap:           routing.BuildSnapIndex(graph),
		Store:          store,
		Corpus:         corpus,
		TrafficProfile: profileName,
	}, nil
}

func BuildCorpus(g *builder.Graph, spec CorpusSpec) ([]CorpusCase, error) {
	if spec.Count <= 0 {
		return nil, fmt.Errorf("corpus count must be positive")
	}
	if len(g.Nodes) == 0 {
		return nil, fmt.Errorf("graph contains no nodes")
	}

	maxAttempts := spec.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = spec.Count * 200
	}

	rng := rand.New(rand.NewSource(spec.Seed))
	router := routing.NewRouterWithMode(g, nil, routing.RoutingModeBaseAStar)
	seen := make(map[[2]uint32]struct{}, spec.Count)
	cases := make([]CorpusCase, 0, spec.Count)

	for attempts := 0; len(cases) < spec.Count && attempts < maxAttempts; attempts++ {
		srcIdx := uint32(rng.Intn(len(g.Nodes)))
		dstIdx := uint32(rng.Intn(len(g.Nodes)))
		if srcIdx == dstIdx {
			continue
		}

		if spec.MinNodeIndexGap > 0 {
			gap := int(srcIdx) - int(dstIdx)
			if gap < 0 {
				gap = -gap
			}
			if gap < spec.MinNodeIndexGap {
				continue
			}
		}

		key := [2]uint32{srcIdx, dstIdx}
		if _, ok := seen[key]; ok {
			continue
		}

		routes := router.ComputeFromIndices(srcIdx, dstIdx, 1, routing.BaseWeight)
		if len(routes) == 0 {
			continue
		}

		src := g.Nodes[srcIdx]
		dst := g.Nodes[dstIdx]
		cases = append(cases, CorpusCase{
			Name:   fmt.Sprintf("case-%03d", len(cases)+1),
			SrcIdx: srcIdx,
			DstIdx: dstIdx,
			SrcLat: src.Lat,
			SrcLon: src.Lon,
			DstLat: dst.Lat,
			DstLon: dst.Lon,
		})
		seen[key] = struct{}{}
	}

	if len(cases) != spec.Count {
		return nil, fmt.Errorf("generated %d/%d benchmark cases for region %q", len(cases), spec.Count, spec.Region)
	}
	return cases, nil
}

func ApplyTrafficProfile(g *builder.Graph, store *traffic.Store, spec TrafficProfileSpec) error {
	if spec.EdgeCount <= 0 {
		return fmt.Errorf("traffic edge count must be positive")
	}
	if spec.MinMultiplier <= 1.0 {
		return fmt.Errorf("min multiplier must be greater than 1")
	}
	if spec.MaxMultiplier < spec.MinMultiplier {
		return fmt.Errorf("max multiplier must be >= min multiplier")
	}

	rng := rand.New(rand.NewSource(spec.Seed))
	seen := make(map[builder.EdgeID]struct{}, spec.EdgeCount)

	for attempts := 0; len(seen) < spec.EdgeCount && attempts < spec.EdgeCount*50; attempts++ {
		edgeID := builder.EdgeID(rng.Intn(len(g.Edges)))
		edge := g.Edges[edgeID]
		if edge.DistanceM < spec.MinEdgeDistanceM {
			continue
		}
		if edge.Weight <= 0 {
			continue
		}
		if _, ok := seen[edgeID]; ok {
			continue
		}

		multiplier := spec.MinMultiplier
		if spec.MaxMultiplier > spec.MinMultiplier {
			multiplier += rng.Float32() * (spec.MaxMultiplier - spec.MinMultiplier)
		}
		store.RecordObservation(edgeID, edge.Weight*multiplier, edge.Weight)
		seen[edgeID] = struct{}{}
	}

	if len(seen) != spec.EdgeCount {
		return fmt.Errorf("generated %d/%d live traffic edges for region %q", len(seen), spec.EdgeCount, spec.Region)
	}

	traffic.CustomizeOverlayWeights(g, store)
	return nil
}

func loadJSON[T any](path string) (T, error) {
	var payload T
	data, err := os.ReadFile(path)
	if err != nil {
		return payload, fmt.Errorf("read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return payload, fmt.Errorf("decode %s: %w", path, err)
	}
	return payload, nil
}

func testdataRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "testdata")
}

func moduleRoot() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..")
}
