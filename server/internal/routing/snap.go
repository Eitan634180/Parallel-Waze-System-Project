package routing

import (
	"math"

	"nav-system/map/builder"
)

// ---------------------------------------------------------------------------
// Spatial grid snap
// ---------------------------------------------------------------------------

const gridSize = 500 // G×G cells covering the bounding box of all nodes

// SnapIndex is a prebuilt spatial grid for O(1)-average nearest-node lookups.
type SnapIndex struct {
	g       *builder.Graph
	minX    float32
	minY    float32
	cellW   float32 // width of one grid cell in metres
	cellH   float32
	buckets [][]uint32 // [gridSize*gridSize] slices of node internal indices
}

// BuildSnapIndex builds the spatial grid from the loaded graph.
// Call once at server startup.
func BuildSnapIndex(g *builder.Graph) *SnapIndex {
	if len(g.Nodes) == 0 {
		return &SnapIndex{g: g}
	}

	minX, minY := g.Nodes[0].X, g.Nodes[0].Y
	maxX, maxY := minX, minY
	for i := range g.Nodes {
		n := &g.Nodes[i]
		if n.X < minX {
			minX = n.X
		}
		if n.X > maxX {
			maxX = n.X
		}
		if n.Y < minY {
			minY = n.Y
		}
		if n.Y > maxY {
			maxY = n.Y
		}
	}

	// Add a tiny margin so maxX/maxY nodes don't overflow the last cell.
	maxX += 1
	maxY += 1

	cellW := (maxX - minX) / gridSize
	cellH := (maxY - minY) / gridSize

	buckets := make([][]uint32, gridSize*gridSize)
	for i := range g.Nodes {
		n := &g.Nodes[i]
		cx := int((n.X - minX) / cellW)
		cy := int((n.Y - minY) / cellH)
		idx := cy*gridSize + cx
		buckets[idx] = append(buckets[idx], uint32(i))
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

// Snap returns the internal node index of the graph node nearest to (lat, lon).
func (si *SnapIndex) Snap(lat, lon float64) uint32 {
	qx, qy := projectXY(lat, lon)

	cx0 := int((qx - si.minX) / si.cellW)
	cy0 := int((qy - si.minY) / si.cellH)
	cx0 = clamp(cx0, 0, gridSize-1)
	cy0 = clamp(cy0, 0, gridSize-1)

	bestIdx := uint32(0)
	bestDist := float32(math.MaxFloat32)

	// Spiral outward from the home cell until we find at least one candidate
	// and the ring distance cannot improve on it.
	for ring := 0; ; ring++ {
		ringMinDist := float32(ring) * min32(si.cellW, si.cellH)
		ringMinDistSq := ringMinDist * ringMinDist
		if ring > 0 && ringMinDistSq > bestDist {
			break // no closer node possible in larger rings
		}

		for dy := -ring; dy <= ring; dy++ {
			for dx := -ring; dx <= ring; dx++ {
				// Only process cells on the perimeter of this ring.
				if abs(dx) != ring && abs(dy) != ring {
					continue
				}
				cx := cx0 + dx
				cy := cy0 + dy
				if cx < 0 || cx >= gridSize || cy < 0 || cy >= gridSize {
					continue
				}
				for _, ni := range si.buckets[cy*gridSize+cx] {
					n := &si.g.Nodes[ni]
					d := dist2(qx, qy, n.X, n.Y)
					if d < bestDist {
						bestDist = d
						bestIdx = ni
					}
				}
			}
		}

	}
	return bestIdx
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const (
	earthRadiusM = 6_371_000.0
	lat0Deg      = 31.5
)

var cosLat0 = float32(math.Cos(lat0Deg * math.Pi / 180.0))

func projectXY(lat, lon float64) (x, y float32) {
	x = float32(lon*math.Pi/180.0) * cosLat0 * earthRadiusM
	y = float32(lat * math.Pi / 180.0 * earthRadiusM)
	return
}

func dist2(ax, ay, bx, by float32) float32 {
	dx := ax - bx
	dy := ay - by
	return dx*dx + dy*dy
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func min32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}
