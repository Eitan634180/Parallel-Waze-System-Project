package store

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"nav-system/src/graph/model"
)

const (
	fileMagic   = "NAVI"
	fileVersion = uint16(4)

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
)

var le = binary.LittleEndian

type nodeBin struct {
	ID     uint64
	Lat    float64
	Lon    float64
	X      float32
	Y      float32
	CellID uint32
}

type edgeBin struct {
	ID          uint32
	FromNodeIdx uint32
	ToNodeIdx   uint32
	Weight      float32
	DistanceM   float32
	SpeedKmh    float32
	RoadClass   uint8
	Flags       uint8
	Pad         [2]byte
}

type overlayEdgeBin struct {
	FromNodeIdx uint32
	ToNodeIdx   uint32
	Weight      float32
	DistanceM   float32
	IsCrossCell uint8
	Pad         [3]byte
}

type graphMeta struct {
	BBox model.BoundingBox `json:"bbox"`
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
	if ver != fileVersion {
		f.Close()
		return nil, 0, fmt.Errorf("unsupported graph format version %d in %s (expected %d)", ver, path, fileVersion)
	}

	var count uint64
	if err := binary.Read(f, le, &count); err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, count, nil
}

func openSequenceFile(path string) (*os.File, uint64, error) {
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
	if ver != fileVersion {
		f.Close()
		return nil, 0, fmt.Errorf("unsupported graph format version %d in %s (expected %d)", ver, path, fileVersion)
	}

	var count uint64
	if err := binary.Read(f, le, &count); err != nil {
		f.Close()
		return nil, 0, err
	}
	return f, count, nil
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
