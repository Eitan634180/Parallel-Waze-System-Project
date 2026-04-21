package benchmark_test

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"

	"nav-system/src/graph/model"
	"nav-system/src/graph/store"
	"nav-system/src/routing"
	"nav-system/src/core/utilities"
)

type CorpusSpec struct {
	Region          string  `json:"region"`
	Seed            int64   `json:"seed"`
	Count           int     `json:"count"`
	MinNodeIndexGap int     `json:"min_node_index_gap"`
	MinDistanceM    float64 `json:"min_distance_m"`
	MaxDistanceM    float64 `json:"max_distance_m"`
	MaxAttempts     int     `json:"max_attempts"`
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
	Region string
	Graph  *model.Graph
	Snap   *routing.SnapIndex
	Corpus []CorpusCase
}

func LoadFixture(corpusName string) (*Fixture, error) {
	corpusSpec, err := loadJSON[CorpusSpec](filepath.Join(testdataRoot(), "benchmark-cases", corpusName))
	if err != nil {
		return nil, err
	}

	graph, err := store.LoadGraph(filepath.Join(moduleRoot(), "data", "map", corpusSpec.Region))
	if err != nil {
		return nil, fmt.Errorf("load graph %q: %w", corpusSpec.Region, err)
	}

	corpus, err := BuildCorpus(graph, corpusSpec)
	if err != nil {
		return nil, err
	}

	return &Fixture{
		Region: corpusSpec.Region,
		Graph:  graph,
		Snap:   routing.BuildSnapIndex(graph),
		Corpus: corpus,
	}, nil
}

func BuildCorpus(g *model.Graph, spec CorpusSpec) ([]CorpusCase, error) {
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

		src := g.Nodes[srcIdx]
		dst := g.Nodes[dstIdx]
		dist := utilities.HaversineM(src.Lat, src.Lon, dst.Lat, dst.Lon)
		if spec.MinDistanceM > 0 {
			if dist < spec.MinDistanceM || dist > spec.MaxDistanceM {
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

		if len(cases)%100 == 0 {
			log.Printf("Generated %d/%d benchmark cases for region %q\n", len(cases), spec.Count, spec.Region)
		}
	}

	if len(cases) != spec.Count {
		return nil, fmt.Errorf("generated %d/%d benchmark cases for region %q", len(cases), spec.Count, spec.Region)
	}
	return cases, nil
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
