package routing

import (
	"math"

	"nav-system/src/utilities"
	"nav-system/src/graph/builder"
)

// SnapIndex is a prebuilt spatial grid for nearest-node lookups.
type SnapIndex struct {
	g       *builder.Graph
	minX    float32
	minY    float32
	cellW   float32
	cellH   float32
	buckets [][]uint32
}

// BuildSnapIndex builds the spatial grid once at startup.
func BuildSnapIndex(g *builder.Graph) *SnapIndex {
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

	cellW := (maxX - minX) / gridSize
	cellH := (maxY - minY) / gridSize
	buckets := make([][]uint32, gridSize*gridSize)

	for i := range g.Nodes {
		node := &g.Nodes[i]
		cellX := int((node.X - minX) / cellW)
		cellY := int((node.Y - minY) / cellH)
		bucketIdx := cellY*gridSize + cellX
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
	qx, qy := utilities.Project(lat, lon)

	cellX := clampGridCoord(qx, si.minX, si.cellW)
	cellY := clampGridCoord(qy, si.minY, si.cellH)
	maxRing := max(
		max(cellX, gridSize-1-cellX),
		max(cellY, gridSize-1-cellY),
	)

	bestIdx := uint32(0)
	bestDist := float32(math.MaxFloat32)

	for ring := 0; ring <= maxRing; ring++ {
		ringMinDist := float32(ring) * min32(si.cellW, si.cellH)
		if ring > 0 && ringMinDist*ringMinDist > bestDist {
			break
		}

		for dy := -ring; dy <= ring; dy++ {
			for dx := -ring; dx <= ring; dx++ {
				if abs(dx) != ring && abs(dy) != ring {
					continue
				}

				bucketX := cellX + dx
				bucketY := cellY + dy
				if bucketX < 0 || bucketX >= gridSize || bucketY < 0 || bucketY >= gridSize {
					continue
				}

				for _, nodeIdx := range si.buckets[bucketY*gridSize+bucketX] {
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

func clampInt(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func clampGridCoord(value, minValue, cellSize float32) int {
	return clampInt(int((value-minValue)/cellSize), 0, gridSize-1)
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func min32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}
