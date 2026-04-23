package engine

import (
	"math"

	graphroot "nav-system/src/graph"
	"nav-system/src/graph/model"
	"nav-system/src/utilities"
)

// SnapIndex is a prebuilt spatial grid for nearest-node lookups.
type SnapIndex struct {
	g       *model.Graph
	minX    float32
	minY    float32
	cellW   float32
	cellH   float32
	buckets [][]uint32
}

// BuildSnapIndex builds the spatial grid once at startup.
func BuildSnapIndex(g *model.Graph) *SnapIndex {
	if len(g.Nodes) == 0 {
		return &SnapIndex{g: g}
	}

	minX, minY := g.Nodes[0].X, g.Nodes[0].Y
	maxX, maxY := minX, minY
	for i := range g.Nodes {
		node := &g.Nodes[i]
		if node.X < minX {
			minX = node.X
		}
		if node.X > maxX {
			maxX = node.X
		}
		if node.Y < minY {
			minY = node.Y
		}
		if node.Y > maxY {
			maxY = node.Y
		}
	}

	maxX++
	maxY++

	cellW := (maxX - minX) / graphroot.SnapGridSize
	cellH := (maxY - minY) / graphroot.SnapGridSize
	buckets := make([][]uint32, graphroot.SnapGridSize*graphroot.SnapGridSize)

	for i := range g.Nodes {
		node := &g.Nodes[i]
		cellX := int((node.X - minX) / cellW)
		cellY := int((node.Y - minY) / cellH)
		bucketIdx := cellY*graphroot.SnapGridSize + cellX
		buckets[bucketIdx] = append(buckets[bucketIdx], uint32(i))
	}

	return &SnapIndex{
		g:       g,
		minX:    minX,
		minY:    minY,
		cellW:   cellW,
		cellH:   cellH,
		buckets: buckets,
	}
}

// Snap returns the internal node index nearest to the given coordinates.
func (si *SnapIndex) Snap(lat, lon float64) uint32 {
	qx, qy := utilities.ProjectAtReferenceLat(lat, lon, si.g.ProjectionRefLat)

	cellX := clampGridCoord(qx, si.minX, si.cellW)
	cellY := clampGridCoord(qy, si.minY, si.cellH)
	maxRing := max(
		max(cellX, graphroot.SnapGridSize-1-cellX),
		max(cellY, graphroot.SnapGridSize-1-cellY),
	)

	bestIdx := uint32(0)
	bestDist := float32(math.MaxFloat32)

	for ring := 0; ring <= maxRing; ring++ {
		ringMinDist := float32(ring) * utilities.MinFloat32(si.cellW, si.cellH)
		if ring > 0 && ringMinDist*ringMinDist > bestDist {
			break
		}

		for dy := -ring; dy <= ring; dy++ {
			for dx := -ring; dx <= ring; dx++ {
				if utilities.Abs(dx) != ring && utilities.Abs(dy) != ring {
					continue
				}

				bucketX := cellX + dx
				bucketY := cellY + dy
				if bucketX < 0 || bucketX >= graphroot.SnapGridSize || bucketY < 0 || bucketY >= graphroot.SnapGridSize {
					continue
				}

				for _, nodeIdx := range si.buckets[bucketY*graphroot.SnapGridSize+bucketX] {
					node := &si.g.Nodes[nodeIdx]
					dist := utilities.DistanceSquared(qx, qy, node.X, node.Y)
					if dist < bestDist {
						bestDist = dist
						bestIdx = nodeIdx
					}
				}
			}
		}
	}

	return bestIdx
}

func clampGridCoord(value, minValue, cellSize float32) int {
	return utilities.ClampInt(int((value-minValue)/cellSize), 0, graphroot.SnapGridSize-1)
}
