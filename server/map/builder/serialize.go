package builder

// serialize.go — binary save / load for all graph data structures.
//
// File layout (all files):
//   [4 bytes] magic   – "NAVI"
//   [2 bytes] version – uint16, little-endian
//   [8 bytes] count   – uint64, little-endian (number of records)
//   [N bytes] data    – packed structs, little-endian
//
// Files produced:
//   nodes.bin      – Node records
//   edges.bin      – Edge records
//   base_adj.bin   – AdjacencyList (offsets + edge IDs)
//   cells.bin      – Cell records (variable-length node-ID slices)
//   overlay_adj.bin – OverlayAdjList (offsets + OverlayEdge records)
//   boundary.bin   – ordered BoundaryNodes slice

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	magic   = "NAVI"
	version = uint16(1)
)

var le = binary.LittleEndian

// ---------------------------------------------------------------------------
// Save
// ---------------------------------------------------------------------------

// SaveGraph writes all graph data to the given directory.
func SaveGraph(g *Graph, dir string) error {
	fmt.Printf("[SAVE] Writing graph to %s …\n", dir)

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	if err := saveNodes(g, filepath.Join(dir, "nodes.bin")); err != nil {
		return fmt.Errorf("nodes: %w", err)
	}
	if err := saveEdges(g, filepath.Join(dir, "edges.bin")); err != nil {
		return fmt.Errorf("edges: %w", err)
	}
	if err := saveBaseAdj(g, filepath.Join(dir, "base_adj.bin")); err != nil {
		return fmt.Errorf("base_adj: %w", err)
	}
	if err := saveCells(g, filepath.Join(dir, "cells.bin")); err != nil {
		return fmt.Errorf("cells: %w", err)
	}
	if err := saveBoundary(g, filepath.Join(dir, "boundary.bin")); err != nil {
		return fmt.Errorf("boundary: %w", err)
	}
	if err := saveOverlayAdj(g, filepath.Join(dir, "overlay_adj.bin")); err != nil {
		return fmt.Errorf("overlay_adj: %w", err)
	}

	fmt.Println("[SAVE] Done.")
	return nil
}

// ---------------------------------------------------------------------------
// Load
// ---------------------------------------------------------------------------

// LoadGraph reads all graph data from the given directory.
func LoadGraph(dir string) (*Graph, error) {
	fmt.Printf("[LOAD] Reading graph from %s …\n", dir)
	g := &Graph{
		NodeIdx:         make(map[NodeID]uint32),
		CellIdx:         make(map[NodeID]CellID),
		BoundaryNodeIdx: make(map[NodeID]uint32),
	}

	var err error

	if g.Nodes, err = loadNodes(filepath.Join(dir, "nodes.bin")); err != nil {
		return nil, fmt.Errorf("nodes: %w", err)
	}
	// Rebuild NodeIdx
	for i, n := range g.Nodes {
		g.NodeIdx[n.ID] = uint32(i)
		g.CellIdx[n.ID] = n.CellID
	}

	if g.Edges, err = loadEdges(filepath.Join(dir, "edges.bin")); err != nil {
		return nil, fmt.Errorf("edges: %w", err)
	}

	if g.BaseAdj, err = loadBaseAdj(filepath.Join(dir, "base_adj.bin")); err != nil {
		return nil, fmt.Errorf("base_adj: %w", err)
	}

	if g.Cells, err = loadCells(filepath.Join(dir, "cells.bin")); err != nil {
		return nil, fmt.Errorf("cells: %w", err)
	}

	if g.BoundaryNodes, err = loadBoundary(filepath.Join(dir, "boundary.bin")); err != nil {
		return nil, fmt.Errorf("boundary: %w", err)
	}
	for i, nid := range g.BoundaryNodes {
		g.BoundaryNodeIdx[nid] = uint32(i)
	}

	if g.OverlayAdj, err = loadOverlayAdj(filepath.Join(dir, "overlay_adj.bin")); err != nil {
		return nil, fmt.Errorf("overlay_adj: %w", err)
	}

	fmt.Printf("[LOAD] Done: %d nodes, %d edges, %d cells, %d boundary nodes, %d overlay edges\n",
		len(g.Nodes), len(g.Edges), len(g.Cells), len(g.BoundaryNodes), len(g.OverlayAdj.OverlayEdges))
	return g, nil
}

// ---------------------------------------------------------------------------
// Nodes
// ---------------------------------------------------------------------------

// nodeBin is the fixed-width on-disk representation of a Node.
type nodeBin struct {
	ID     uint64
	Lat    float64
	Lon    float64
	X      float32
	Y      float32
	CellID uint32
}

const nodeBinSize = 8 + 8 + 8 + 4 + 4 + 4 // = 36 bytes

func saveNodes(g *Graph, path string) error {
	f, err := createFile(path)
	if err != nil {
		return err
	}
	defer f.Close()
	writeHeader(f, uint64(len(g.Nodes)))
	for i := range g.Nodes {
		n := &g.Nodes[i]
		writeFixed(f, nodeBin{n.ID, n.Lat, n.Lon, n.X, n.Y, n.CellID})
	}
	return nil
}

func loadNodes(path string) ([]Node, error) {
	f, count, err := openFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	nodes := make([]Node, count)
	for i := range nodes {
		var b nodeBin
		if err := binary.Read(f, le, &b); err != nil {
			return nil, err
		}
		nodes[i] = Node{ID: b.ID, Lat: b.Lat, Lon: b.Lon, X: b.X, Y: b.Y, CellID: b.CellID}
	}
	return nodes, nil
}

// ---------------------------------------------------------------------------
// Edges
// ---------------------------------------------------------------------------

type edgeBin struct {
	ID         uint32
	FromNodeID uint64
	ToNodeID   uint64
	Weight     float32
	DistanceM  float32
	SpeedKmh   float32
	RoadClass  uint8
	Flags      uint8
	Pad        [2]byte // align to 4 bytes
}

func saveEdges(g *Graph, path string) error {
	f, err := createFile(path)
	if err != nil {
		return err
	}
	defer f.Close()
	writeHeader(f, uint64(len(g.Edges)))
	for i := range g.Edges {
		e := &g.Edges[i]
		writeFixed(f, edgeBin{
			ID: e.ID, FromNodeID: e.FromNodeID, ToNodeID: e.ToNodeID,
			Weight: e.Weight, DistanceM: e.DistanceM, SpeedKmh: e.SpeedKmh,
			RoadClass: e.RoadClass, Flags: e.Flags,
		})
	}
	return nil
}

func loadEdges(path string) ([]Edge, error) {
	f, count, err := openFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	edges := make([]Edge, count)
	for i := range edges {
		var b edgeBin
		if err := binary.Read(f, le, &b); err != nil {
			return nil, err
		}
		edges[i] = Edge{
			ID: b.ID, FromNodeID: b.FromNodeID, ToNodeID: b.ToNodeID,
			Weight: b.Weight, DistanceM: b.DistanceM, SpeedKmh: b.SpeedKmh,
			RoadClass: b.RoadClass, Flags: b.Flags,
		}
	}
	return edges, nil
}

// ---------------------------------------------------------------------------
// Base adjacency list
// ---------------------------------------------------------------------------

func saveBaseAdj(g *Graph, path string) error {
	f, err := createFile(path)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write offsets count (= num_nodes + 1), then offsets, then edge IDs
	writeUint64(f, uint64(len(g.BaseAdj.Offsets)))
	for _, o := range g.BaseAdj.Offsets {
		writeUint32(f, o)
	}
	writeUint64(f, uint64(len(g.BaseAdj.EdgeIDs)))
	for _, e := range g.BaseAdj.EdgeIDs {
		writeUint32(f, e)
	}
	return nil
}

func loadBaseAdj(path string) (AdjacencyList, error) {
	f, err := os.Open(path)
	if err != nil {
		return AdjacencyList{}, err
	}
	defer f.Close()

	offsetCount, _ := readUint64(f)
	offsets := make([]uint32, offsetCount)
	for i := range offsets {
		v, _ := readUint32(f)
		offsets[i] = v
	}

	edgeCount, _ := readUint64(f)
	edgeIDs := make([]uint32, edgeCount)
	for i := range edgeIDs {
		v, _ := readUint32(f)
		edgeIDs[i] = v
	}

	return AdjacencyList{Offsets: offsets, EdgeIDs: edgeIDs}, nil
}

// ---------------------------------------------------------------------------
// Cells (variable-length node-ID slices)
// ---------------------------------------------------------------------------

func saveCells(g *Graph, path string) error {
	f, err := createFile(path)
	if err != nil {
		return err
	}
	defer f.Close()
	writeHeader(f, uint64(len(g.Cells)))
	for i := range g.Cells {
		c := &g.Cells[i]
		writeUint32(f, c.ID)
		writeUint64(f, uint64(len(c.InternalNodeIDs)))
		for _, nid := range c.InternalNodeIDs {
			writeUint64(f, nid)
		}
		writeUint64(f, uint64(len(c.BoundaryNodeIDs)))
		for _, nid := range c.BoundaryNodeIDs {
			writeUint64(f, nid)
		}
	}
	return nil
}

func loadCells(path string) ([]Cell, error) {
	f, count, err := openFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cells := make([]Cell, count)
	for i := range cells {
		id, _ := readUint32(f)
		ic, _ := readUint64(f)
		internal := make([]NodeID, ic)
		for j := range internal {
			v, _ := readUint64(f)
			internal[j] = v
		}
		bc, _ := readUint64(f)
		boundary := make([]NodeID, bc)
		for j := range boundary {
			v, _ := readUint64(f)
			boundary[j] = v
		}
		cells[i] = Cell{ID: id, InternalNodeIDs: internal, BoundaryNodeIDs: boundary}
	}
	return cells, nil
}

// ---------------------------------------------------------------------------
// Boundary nodes list
// ---------------------------------------------------------------------------

func saveBoundary(g *Graph, path string) error {
	f, err := createFile(path)
	if err != nil {
		return err
	}
	defer f.Close()
	writeHeader(f, uint64(len(g.BoundaryNodes)))
	for _, nid := range g.BoundaryNodes {
		writeUint64(f, nid)
	}
	return nil
}

func loadBoundary(path string) ([]NodeID, error) {
	f, count, err := openFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	nodes := make([]NodeID, count)
	for i := range nodes {
		v, _ := readUint64(f)
		nodes[i] = v
	}
	return nodes, nil
}

// ---------------------------------------------------------------------------
// Overlay adjacency list
// ---------------------------------------------------------------------------

type overlayEdgeBin struct {
	FromNodeID  uint64
	ToNodeID    uint64
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

	writeUint64(f, uint64(len(g.OverlayAdj.Offsets)))
	for _, o := range g.OverlayAdj.Offsets {
		writeUint32(f, o)
	}
	writeUint64(f, uint64(len(g.OverlayAdj.OverlayEdges)))
	for _, e := range g.OverlayAdj.OverlayEdges {
		cc := uint8(0)
		if e.IsCrossCell {
			cc = 1
		}
		writeFixed(f, overlayEdgeBin{e.FromNodeID, e.ToNodeID, e.Weight, e.DistanceM, cc, [3]byte{}})
	}
	return nil
}

func loadOverlayAdj(path string) (OverlayAdjList, error) {
	f, err := os.Open(path)
	if err != nil {
		return OverlayAdjList{}, err
	}
	defer f.Close()

	offsetCount, _ := readUint64(f)
	offsets := make([]uint32, offsetCount)
	for i := range offsets {
		v, _ := readUint32(f)
		offsets[i] = v
	}

	edgeCount, _ := readUint64(f)
	edges := make([]OverlayEdge, edgeCount)
	for i := range edges {
		var b overlayEdgeBin
		if err := binary.Read(f, le, &b); err != nil {
			return OverlayAdjList{}, err
		}
		edges[i] = OverlayEdge{
			FromNodeID:  b.FromNodeID,
			ToNodeID:    b.ToNodeID,
			Weight:      b.Weight,
			DistanceM:   b.DistanceM,
			IsCrossCell: b.IsCrossCell != 0,
		}
	}

	return OverlayAdjList{Offsets: offsets, OverlayEdges: edges}, nil
}

// ---------------------------------------------------------------------------
// Low-level I/O helpers
// ---------------------------------------------------------------------------

func createFile(path string) (*os.File, error) {
	return os.Create(path)
}

func writeHeader(w io.Writer, count uint64) {
	w.Write([]byte(magic))
	binary.Write(w, le, version)
	binary.Write(w, le, count)
}

func openFile(path string) (*os.File, uint64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	hdr := make([]byte, 4)
	f.Read(hdr)
	if string(hdr) != magic {
		f.Close()
		return nil, 0, fmt.Errorf("bad magic in %s", path)
	}
	var ver uint16
	binary.Read(f, le, &ver)
	var count uint64
	binary.Read(f, le, &count)
	return f, count, nil
}

func writeFixed(w io.Writer, v interface{}) { binary.Write(w, le, v) }

func writeUint32(w io.Writer, v uint32) { binary.Write(w, le, v) }
func writeUint64(w io.Writer, v uint64) { binary.Write(w, le, v) }

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
