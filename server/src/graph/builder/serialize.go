package builder

// serialize.go saves and loads the graph in compact binary files with a shared
// header: 4-byte magic, 2-byte version, and an 8-byte record count.

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sync/errgroup"
)

const (
	fileMagic   = "NAVI"
	fileVersion = uint16(2)

	nodesFileName      = "nodes.bin"
	edgesFileName      = "edges.bin"
	baseAdjFileName    = "base_adj.bin"
	cellsFileName      = "cells.bin"
	boundaryFileName   = "boundary.bin"
	overlayAdjFileName = "overlay_adj.bin"
	metaFileName       = "meta.json"

	graphStoreLogPrefix = "graph-store:"
	graphDataDirPerm    = 0o755
	headerMagicSize     = 4
	crossCellFalse      = uint8(0)
	crossCellTrue       = uint8(1)
	fileBufferSize      = 1 << 20
)

var le = binary.LittleEndian

// SaveGraph writes all graph data to the given directory.
func SaveGraph(g *Graph, dir string) error {
	log.Printf("%s writing graph to %s", graphStoreLogPrefix, dir)

	if err := os.MkdirAll(dir, graphDataDirPerm); err != nil {
		return err
	}

	eg := new(errgroup.Group)

	eg.Go(func() error {
		if err := saveNodes(g, filepath.Join(dir, nodesFileName)); err != nil {
			return fmt.Errorf("nodes: %w", err)
		}
		return nil
	})
	eg.Go(func() error {
		if err := saveEdges(g, filepath.Join(dir, edgesFileName)); err != nil {
			return fmt.Errorf("edges: %w", err)
		}
		return nil
	})
	eg.Go(func() error {
		if err := saveBaseAdj(g, filepath.Join(dir, baseAdjFileName)); err != nil {
			return fmt.Errorf("base_adj: %w", err)
		}
		return nil
	})
	eg.Go(func() error {
		if err := saveCells(g, filepath.Join(dir, cellsFileName)); err != nil {
			return fmt.Errorf("cells: %w", err)
		}
		return nil
	})
	eg.Go(func() error {
		if err := saveBoundary(g, filepath.Join(dir, boundaryFileName)); err != nil {
			return fmt.Errorf("boundary: %w", err)
		}
		return nil
	})
	eg.Go(func() error {
		if err := saveOverlayAdj(g, filepath.Join(dir, overlayAdjFileName)); err != nil {
			return fmt.Errorf("overlay_adj: %w", err)
		}
		return nil
	})
	eg.Go(func() error {
		if err := saveMeta(g, filepath.Join(dir, metaFileName)); err != nil {
			return fmt.Errorf("meta: %w", err)
		}
		return nil
	})

	if err := eg.Wait(); err != nil {
		return err
	}

	log.Printf("%s graph write complete", graphStoreLogPrefix)
	return nil
}

// LoadGraph reads all graph data from the given directory.
func LoadGraph(dir string) (*Graph, error) {
	log.Printf("%s reading graph from %s", graphStoreLogPrefix, dir)

	g := &Graph{}
	eg := new(errgroup.Group)

	eg.Go(func() error {
		nodes, err := loadNodes(filepath.Join(dir, nodesFileName))
		if err != nil {
			return fmt.Errorf("nodes: %w", err)
		}
		g.Nodes = nodes
		g.NodeIdx = make(map[NodeID]uint32, len(nodes))
		for i, n := range nodes {
			g.NodeIdx[n.ID] = uint32(i)
		}
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
		boundaryNodes, err := loadBoundary(filepath.Join(dir, boundaryFileName))
		if err != nil {
			return fmt.Errorf("boundary: %w", err)
		}
		g.BoundaryNodes = boundaryNodes
		g.BoundaryNodeIdx = make(map[NodeID]uint32, len(boundaryNodes))
		for i, nid := range boundaryNodes {
			g.BoundaryNodeIdx[nid] = uint32(i)
		}
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
		len(g.BoundaryNodes),
		len(g.OverlayAdj.OverlayEdges),
	)
	return g, nil
}

// nodeBin is the fixed-width on-disk representation of a Node.
type nodeBin struct {
	ID     uint64
	Lat    float64
	Lon    float64
	X      float32
	Y      float32
	CellID uint32
}

func saveNodes(g *Graph, path string) error {
	f, err := createFile(path)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriterSize(f, fileBufferSize)

	if err := writeHeader(bw, uint64(len(g.Nodes))); err != nil {
		return err
	}
	for i := range g.Nodes {
		n := &g.Nodes[i]
		if err := writeFixed(bw, nodeBin{n.ID, n.Lat, n.Lon, n.X, n.Y, n.CellID}); err != nil {
			return err
		}
	}
	return bw.Flush()
}

func loadNodes(path string) ([]Node, error) {
	f, count, err := openFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, fileBufferSize)

	nodes := make([]Node, count)
	for i := range nodes {
		var b nodeBin
		if err := binary.Read(br, le, &b); err != nil {
			return nil, err
		}
		nodes[i] = Node{ID: b.ID, Lat: b.Lat, Lon: b.Lon, X: b.X, Y: b.Y, CellID: b.CellID}
	}
	return nodes, nil
}

type edgeBin struct {
	ID         uint32
	FromNodeID uint64
	ToNodeID   uint64
	ToNodeIdx  uint32
	Weight     float32
	DistanceM  float32
	SpeedKmh   float32
	RoadClass  uint8
	Flags      uint8
	Pad        [2]byte
}

func saveEdges(g *Graph, path string) error {
	f, err := createFile(path)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriterSize(f, fileBufferSize)

	if err := writeHeader(bw, uint64(len(g.Edges))); err != nil {
		return err
	}
	for i := range g.Edges {
		e := &g.Edges[i]
		if err := writeFixed(bw, edgeBin{
			ID:         e.ID,
			FromNodeID: e.FromNodeID,
			ToNodeID:   e.ToNodeID,
			ToNodeIdx:  e.ToNodeIdx,
			Weight:     e.Weight,
			DistanceM:  e.DistanceM,
			SpeedKmh:   e.SpeedKmh,
			RoadClass:  e.RoadClass,
			Flags:      e.Flags,
		}); err != nil {
			return err
		}
	}
	return bw.Flush()
}

func loadEdges(path string) ([]Edge, error) {
	f, count, err := openFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, fileBufferSize)

	edges := make([]Edge, count)
	for i := range edges {
		var b edgeBin
		if err := binary.Read(br, le, &b); err != nil {
			return nil, err
		}
		edges[i] = Edge{
			ID:         b.ID,
			FromNodeID: b.FromNodeID,
			ToNodeID:   b.ToNodeID,
			ToNodeIdx:  b.ToNodeIdx,
			Weight:     b.Weight,
			DistanceM:  b.DistanceM,
			SpeedKmh:   b.SpeedKmh,
			RoadClass:  b.RoadClass,
			Flags:      b.Flags,
		}
	}
	return edges, nil
}

func saveBaseAdj(g *Graph, path string) error {
	f, err := createFile(path)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriterSize(f, fileBufferSize)

	if err := writeHeader(bw, uint64(len(g.BaseAdj.Offsets))); err != nil {
		return err
	}
	for _, o := range g.BaseAdj.Offsets {
		if err := writeUint32(bw, o); err != nil {
			return err
		}
	}
	if err := writeUint64(bw, uint64(len(g.BaseAdj.EdgeIDs))); err != nil {
		return err
	}
	for _, e := range g.BaseAdj.EdgeIDs {
		if err := writeUint32(bw, e); err != nil {
			return err
		}
	}
	return bw.Flush()
}

func loadBaseAdj(path string) (AdjacencyList, error) {
	f, offsetCount, _, err := openSequenceFile(path)
	if err != nil {
		return AdjacencyList{}, err
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, fileBufferSize)

	offsets := make([]uint32, offsetCount)
	for i := range offsets {
		v, err := readUint32(br)
		if err != nil {
			return AdjacencyList{}, err
		}
		offsets[i] = v
	}

	edgeCount, err := readUint64(br)
	if err != nil {
		return AdjacencyList{}, err
	}
	edgeIDs := make([]uint32, edgeCount)
	for i := range edgeIDs {
		v, err := readUint32(br)
		if err != nil {
			return AdjacencyList{}, err
		}
		edgeIDs[i] = v
	}
	return AdjacencyList{Offsets: offsets, EdgeIDs: edgeIDs}, nil
}

func saveCells(g *Graph, path string) error {
	f, err := createFile(path)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriterSize(f, fileBufferSize)

	if err := writeHeader(bw, uint64(len(g.Cells))); err != nil {
		return err
	}
	for i := range g.Cells {
		c := &g.Cells[i]
		if err := writeUint32(bw, c.ID); err != nil {
			return err
		}
		if err := writeUint32(bw, uint32(len(c.InternalNodeIdxs))); err != nil {
			return err
		}
		for _, idx := range c.InternalNodeIdxs {
			if err := writeUint32(bw, idx); err != nil {
				return err
			}
		}
		if err := writeUint32(bw, uint32(len(c.BoundaryNodeIdxs))); err != nil {
			return err
		}
		for _, idx := range c.BoundaryNodeIdxs {
			if err := writeUint32(bw, idx); err != nil {
				return err
			}
		}
	}
	return bw.Flush()
}

func loadCells(path string) ([]Cell, error) {
	f, count, err := openFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, fileBufferSize)

	cells := make([]Cell, count)
	for i := range cells {
		id, err := readUint32(br)
		if err != nil {
			return nil, err
		}
		ic, err := readUint32(br)
		if err != nil {
			return nil, err
		}
		internal := make([]uint32, ic)
		for j := range internal {
			v, err := readUint32(br)
			if err != nil {
				return nil, err
			}
			internal[j] = v
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
		cells[i] = Cell{ID: id, InternalNodeIdxs: internal, BoundaryNodeIdxs: boundary}
	}
	return cells, nil
}

func saveBoundary(g *Graph, path string) error {
	f, err := createFile(path)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriterSize(f, fileBufferSize)

	if err := writeHeader(bw, uint64(len(g.BoundaryNodes))); err != nil {
		return err
	}
	for _, nid := range g.BoundaryNodes {
		if err := writeUint64(bw, nid); err != nil {
			return err
		}
	}
	return bw.Flush()
}

func loadBoundary(path string) ([]NodeID, error) {
	f, count, err := openFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, fileBufferSize)

	nodes := make([]NodeID, count)
	for i := range nodes {
		v, err := readUint64(br)
		if err != nil {
			return nil, err
		}
		nodes[i] = v
	}
	return nodes, nil
}

type overlayEdgeBin struct {
	FromNodeID  uint64
	ToNodeID    uint64
	ToNodeIdx   uint32
	Weight      float32
	DistanceM   float32
	IsCrossCell uint8
	Pad         [3]byte
}

func saveOverlayAdj(g *Graph, path string) error {
	f, err := createFile(path)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriterSize(f, fileBufferSize)

	if err := writeHeader(bw, uint64(len(g.OverlayAdj.Offsets))); err != nil {
		return err
	}
	for _, o := range g.OverlayAdj.Offsets {
		if err := writeUint32(bw, o); err != nil {
			return err
		}
	}
	if err := writeUint64(bw, uint64(len(g.OverlayAdj.OverlayEdges))); err != nil {
		return err
	}
	for _, e := range g.OverlayAdj.OverlayEdges {
		cc := crossCellFalse
		if e.IsCrossCell {
			cc = crossCellTrue
		}
		if err := writeFixed(bw, overlayEdgeBin{e.FromNodeID, e.ToNodeID, e.ToNodeIdx, e.Weight, e.DistanceM, cc, [3]byte{}}); err != nil {
			return err
		}
	}
	return bw.Flush()
}

func loadOverlayAdj(path string) (OverlayAdjList, error) {
	f, offsetCount, _, err := openSequenceFile(path)
	if err != nil {
		return OverlayAdjList{}, err
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, fileBufferSize)

	offsets := make([]uint32, offsetCount)
	for i := range offsets {
		v, err := readUint32(br)
		if err != nil {
			return OverlayAdjList{}, err
		}
		offsets[i] = v
	}

	edgeCount, err := readUint64(br)
	if err != nil {
		return OverlayAdjList{}, err
	}
	edges := make([]OverlayEdge, edgeCount)
	for i := range edges {
		var b overlayEdgeBin
		if err := binary.Read(br, le, &b); err != nil {
			return OverlayAdjList{}, err
		}
		edges[i] = OverlayEdge{
			FromNodeID:  b.FromNodeID,
			ToNodeID:    b.ToNodeID,
			ToNodeIdx:   b.ToNodeIdx,
			Weight:      b.Weight,
			DistanceM:   b.DistanceM,
			IsCrossCell: b.IsCrossCell != 0,
		}
	}
	return OverlayAdjList{
		Mu:           &sync.RWMutex{},
		Offsets:      offsets,
		OverlayEdges: edges,
	}, nil
}

// graphMeta stores fields written to meta.json alongside the binary files.
type graphMeta struct {
	BBox BoundingBox `json:"bbox"`
}

func saveMeta(g *Graph, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriterSize(f, fileBufferSize)
	if err := json.NewEncoder(bw).Encode(graphMeta{BBox: g.BBox}); err != nil {
		return err
	}
	return bw.Flush()
}

func loadMeta(g *Graph, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	br := bufio.NewReaderSize(f, fileBufferSize)
	var m graphMeta
	if err := json.NewDecoder(br).Decode(&m); err != nil {
		return err
	}
	g.BBox = m.BBox
	return nil
}

func createFile(path string) (*os.File, error) {
	return os.Create(path)
}

func writeHeader(w io.Writer, count uint64) error {
	if _, err := w.Write([]byte(fileMagic)); err != nil {
		return err
	}
	if err := binary.Write(w, le, fileVersion); err != nil {
		return err
	}
	return binary.Write(w, le, count)
}

func openFile(path string) (*os.File, uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}

	hdr := make([]byte, headerMagicSize)
	if _, err := io.ReadFull(f, hdr); err != nil {
		f.Close()
		return nil, 0, err
	}
	if string(hdr) != fileMagic {
		f.Close()
		return nil, 0, fmt.Errorf("bad magic in %s", path)
	}

	var ver uint16
	if err := binary.Read(f, le, &ver); err != nil {
		f.Close()
		return nil, 0, err
	}

	var count uint64
	if err := binary.Read(f, le, &count); err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, count, nil
}

func openSequenceFile(path string) (*os.File, uint64, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, false, err
	}

	hdr := make([]byte, headerMagicSize)
	n, err := io.ReadFull(f, hdr)
	switch {
	case err == nil && string(hdr) == fileMagic:
		var ver uint16
		if err := binary.Read(f, le, &ver); err != nil {
			f.Close()
			return nil, 0, false, err
		}

		var count uint64
		if err := binary.Read(f, le, &count); err != nil {
			f.Close()
			return nil, 0, false, err
		}
		return f, count, true, nil
	case err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF):
		f.Close()
		return nil, 0, false, err
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return nil, 0, false, err
	}
	count, err := readUint64(f)
	if err != nil {
		f.Close()
		if errors.Is(err, io.EOF) && n == 0 {
			return nil, 0, false, io.ErrUnexpectedEOF
		}
		return nil, 0, false, err
	}
	return f, count, false, nil
}

func writeFixed(w io.Writer, v interface{}) error { return binary.Write(w, le, v) }

func writeUint32(w io.Writer, v uint32) error { return binary.Write(w, le, v) }
func writeUint64(w io.Writer, v uint64) error { return binary.Write(w, le, v) }

func readUint32(r io.Reader) (uint32, error) {
	var v uint32
	err := binary.Read(r, le, &v)
	return v, err
}

func readUint64(r io.Reader) (uint64, error) {
	var v uint64
	err := binary.Read(r, le, &v)
	return v, err
}
