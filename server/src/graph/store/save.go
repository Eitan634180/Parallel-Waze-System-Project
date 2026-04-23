package store

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"nav-system/src/graph"
	"nav-system/src/graph/model"

	"golang.org/x/sync/errgroup"
)

// SaveGraph writes all graph data to the given directory.
func SaveGraph(g *model.Graph, dir string) error {
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

func saveNodes(g *model.Graph, path string) error {
	f, err := createFile(path)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriterSize(f, graph.FileBufferSize)

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

func saveEdges(g *model.Graph, path string) error {
	f, err := createFile(path)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriterSize(f, graph.FileBufferSize)

	if err := writeHeader(bw, uint64(len(g.Edges))); err != nil {
		return err
	}
	for i := range g.Edges {
		e := &g.Edges[i]
		if err := writeFixed(bw, edgeBin{
			ID:          e.ID,
			FromNodeIdx: e.FromNodeIdx,
			ToNodeIdx:   e.ToNodeIdx,
			Weight:      e.Weight,
			DistanceM:   e.DistanceM,
			SpeedKmh:    e.SpeedKmh,
			RoadClass:   e.RoadClass,
			Flags:       e.Flags,
		}); err != nil {
			return err
		}
	}
	return bw.Flush()
}

func saveBaseAdj(g *model.Graph, path string) error {
	f, err := createFile(path)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriterSize(f, graph.FileBufferSize)

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

func saveCells(g *model.Graph, path string) error {
	f, err := createFile(path)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriterSize(f, graph.FileBufferSize)

	if err := writeHeader(bw, uint64(len(g.Cells))); err != nil {
		return err
	}
	for i := range g.Cells {
		c := &g.Cells[i]
		if err := writeUint32(bw, c.ID); err != nil {
			return err
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

func saveBoundary(g *model.Graph, path string) error {
	f, err := createFile(path)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriterSize(f, graph.FileBufferSize)

	if err := writeHeader(bw, uint64(len(g.BoundaryBaseIdxs))); err != nil {
		return err
	}
	for _, idx := range g.BoundaryBaseIdxs {
		if err := writeUint32(bw, idx); err != nil {
			return err
		}
	}
	return bw.Flush()
}

func saveOverlayAdj(g *model.Graph, path string) error {
	f, err := createFile(path)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriterSize(f, graph.FileBufferSize)

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
		if err := writeFixed(bw, overlayEdgeBin{e.FromNodeIdx, e.ToNodeIdx, e.Weight, e.DistanceM, cc, [3]byte{}}); err != nil {
			return err
		}
	}
	return bw.Flush()
}

func saveMeta(g *model.Graph, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	bw := bufio.NewWriterSize(f, graph.FileBufferSize)
	if err := json.NewEncoder(bw).Encode(graphMeta{BBox: g.BBox}); err != nil {
		return err
	}
	return bw.Flush()
}
