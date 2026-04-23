package store

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync"

	"nav-system/src/graph"
	"nav-system/src/graph/model"
	"nav-system/src/utilities"

	"golang.org/x/sync/errgroup"
)

// LoadGraph reads all graph data from the given directory.
func LoadGraph(dir string) (*model.Graph, error) {
	log.Printf("%s reading graph from %s", graphStoreLogPrefix, dir)

	g := &model.Graph{}
	eg := new(errgroup.Group)

	eg.Go(func() error {
		nodes, err := loadNodes(filepath.Join(dir, nodesFileName))
		if err != nil {
			return fmt.Errorf("nodes: %w", err)
		}
		g.Nodes = nodes
		return nil
	})

	eg.Go(func() error {
		edges, err := loadEdges(filepath.Join(dir, edgesFileName))
		if err != nil {
			return fmt.Errorf("edges: %w", err)
		}
		g.Edges = edges
		return nil
	})

	eg.Go(func() error {
		baseAdj, err := loadBaseAdj(filepath.Join(dir, baseAdjFileName))
		if err != nil {
			return fmt.Errorf("base_adj: %w", err)
		}
		g.BaseAdj = baseAdj
		return nil
	})

	eg.Go(func() error {
		cells, err := loadCells(filepath.Join(dir, cellsFileName))
		if err != nil {
			return fmt.Errorf("cells: %w", err)
		}
		g.Cells = cells
		return nil
	})

	eg.Go(func() error {
		boundaryBaseIdxs, err := loadBoundary(filepath.Join(dir, boundaryFileName))
		if err != nil {
			return fmt.Errorf("boundary: %w", err)
		}
		g.BoundaryBaseIdxs = boundaryBaseIdxs
		return nil
	})

	eg.Go(func() error {
		overlayAdj, err := loadOverlayAdj(filepath.Join(dir, overlayAdjFileName))
		if err != nil {
			return fmt.Errorf("overlay_adj: %w", err)
		}
		g.OverlayAdj = overlayAdj
		return nil
	})

	eg.Go(func() error {
		if err := loadMeta(g, filepath.Join(dir, metaFileName)); err != nil {
			log.Printf("%s meta.json unavailable (%v), deferring bbox computation from nodes", graphStoreLogPrefix, err)
		}
		return nil
	})

	if err := eg.Wait(); err != nil {
		return nil, err
	}

	g.BoundaryNodeIdx = make([]int32, len(g.Nodes))
	for i := range g.BoundaryNodeIdx {
		g.BoundaryNodeIdx[i] = -1
	}
	for i, idx := range g.BoundaryBaseIdxs {
		g.BoundaryNodeIdx[idx] = int32(i)
	}

	if g.BBox.IsZero() {
		log.Printf("%s recomputing bounding box from node data", graphStoreLogPrefix)
		g.BBox = boundingBoxFromNodes(g.Nodes)
	}
	g.ProjectionRefLat = g.BBox.CenterLat()
	reprojectNodes(g)

	log.Printf("%s graph ready (%d nodes, %d edges, %d cells, %d boundary nodes, %d overlay edges)",
		graphStoreLogPrefix,
		len(g.Nodes),
		len(g.Edges),
		len(g.Cells),
		len(g.BoundaryBaseIdxs),
		len(g.OverlayAdj.OverlayEdges),
	)
	return g, nil
}

func loadNodes(path string) ([]model.Node, error) {
	f, count, err := openFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, graph.FileBufferSize)

	nodes := make([]model.Node, count)
	for i := range nodes {
		var b nodeBin
		if err := binary.Read(br, le, &b); err != nil {
			return nil, err
		}
		nodes[i] = model.Node{ID: b.ID, Lat: b.Lat, Lon: b.Lon, X: b.X, Y: b.Y, CellID: b.CellID}
	}
	return nodes, nil
}

func loadEdges(path string) ([]model.Edge, error) {
	f, count, err := openFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, graph.FileBufferSize)

	edges := make([]model.Edge, count)
	for i := range edges {
		var b edgeBin
		if err := binary.Read(br, le, &b); err != nil {
			return nil, err
		}
		edges[i] = model.Edge{
			ID:          b.ID,
			FromNodeIdx: b.FromNodeIdx,
			ToNodeIdx:   b.ToNodeIdx,
			Weight:      b.Weight,
			DistanceM:   b.DistanceM,
			SpeedKmh:    b.SpeedKmh,
			RoadClass:   b.RoadClass,
			Flags:       b.Flags,
		}
	}
	return edges, nil
}

func loadBaseAdj(path string) (model.AdjacencyList, error) {
	f, offsetCount, err := openFile(path)
	if err != nil {
		return model.AdjacencyList{}, err
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, graph.FileBufferSize)

	offsets := make([]uint32, offsetCount)
	for i := range offsets {
		v, err := readUint32(br)
		if err != nil {
			return model.AdjacencyList{}, err
		}
		offsets[i] = v
	}

	edgeCount, err := readUint64(br)
	if err != nil {
		return model.AdjacencyList{}, err
	}
	edgeIDs := make([]uint32, edgeCount)
	for i := range edgeIDs {
		v, err := readUint32(br)
		if err != nil {
			return model.AdjacencyList{}, err
		}
		edgeIDs[i] = v
	}
	return model.AdjacencyList{Offsets: offsets, EdgeIDs: edgeIDs}, nil
}

func loadCells(path string) ([]model.Cell, error) {
	f, count, err := openFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, graph.FileBufferSize)

	cells := make([]model.Cell, count)
	for i := range cells {
		id, err := readUint32(br)
		if err != nil {
			return nil, err
		}
		bc, err := readUint32(br)
		if err != nil {
			return nil, err
		}
		boundary := make([]uint32, bc)
		for j := range boundary {
			v, err := readUint32(br)
			if err != nil {
				return nil, err
			}
			boundary[j] = v
		}
		cells[i] = model.Cell{ID: id, BoundaryNodeIdxs: boundary}
	}
	return cells, nil
}

func loadBoundary(path string) ([]uint32, error) {
	f, count, err := openFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, graph.FileBufferSize)

	nodes := make([]uint32, count)
	for i := range nodes {
		v, err := readUint32(br)
		if err != nil {
			return nil, err
		}
		nodes[i] = v
	}
	return nodes, nil
}

func loadOverlayAdj(path string) (model.OverlayAdjList, error) {
	f, offsetCount, err := openFile(path)
	if err != nil {
		return model.OverlayAdjList{}, err
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, graph.FileBufferSize)

	offsets := make([]uint32, offsetCount)
	for i := range offsets {
		v, err := readUint32(br)
		if err != nil {
			return model.OverlayAdjList{}, err
		}
		offsets[i] = v
	}

	edgeCount, err := readUint64(br)
	if err != nil {
		return model.OverlayAdjList{}, err
	}
	edges := make([]model.OverlayEdge, edgeCount)
	for i := range edges {
		var b overlayEdgeBin
		if err := binary.Read(br, le, &b); err != nil {
			return model.OverlayAdjList{}, err
		}
		edges[i] = model.OverlayEdge{
			FromNodeIdx: b.FromNodeIdx,
			ToNodeIdx:   b.ToNodeIdx,
			Weight:      b.Weight,
			DistanceM:   b.DistanceM,
			IsCrossCell: b.IsCrossCell != 0,
		}
	}
	return model.OverlayAdjList{
		Mu:           &sync.RWMutex{},
		Offsets:      offsets,
		OverlayEdges: edges,
	}, nil
}

func loadMeta(g *model.Graph, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, graph.FileBufferSize)
	var m graphMeta
	if err := json.NewDecoder(br).Decode(&m); err != nil {
		return err
	}
	g.BBox = m.BBox
	return nil
}

func boundingBoxFromNodes(nodes []model.Node) model.BoundingBox {
	if len(nodes) == 0 {
		return model.BoundingBox{}
	}

	bbox := model.BoundingBox{
		MinLat: math.Inf(1),
		MaxLat: math.Inf(-1),
		MinLon: math.Inf(1),
		MaxLon: math.Inf(-1),
	}
	for i := range nodes {
		node := &nodes[i]
		if node.Lat < bbox.MinLat {
			bbox.MinLat = node.Lat
		}
		if node.Lat > bbox.MaxLat {
			bbox.MaxLat = node.Lat
		}
		if node.Lon < bbox.MinLon {
			bbox.MinLon = node.Lon
		}
		if node.Lon > bbox.MaxLon {
			bbox.MaxLon = node.Lon
		}
	}
	return bbox
}

func reprojectNodes(g *model.Graph) {
	for i := range g.Nodes {
		node := &g.Nodes[i]
		node.X, node.Y = utilities.ProjectAtReferenceLat(node.Lat, node.Lon, g.ProjectionRefLat)
	}
}
