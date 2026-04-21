package utilities

import "math"

type FlowEdge struct {
	To, Rev int
	Cap     int
}

type FlowNet struct {
	Graph [][]FlowEdge
	Level []int
	Iter  []int
	Q     []int
}

func NewFlowNet(n int) *FlowNet {
	fn := &FlowNet{
		Graph: make([][]FlowEdge, n),
		Level: make([]int, n),
		Iter:  make([]int, n),
		Q:     make([]int, 0, n),
	}
	for i := range fn.Level {
		fn.Level[i] = -1
	}
	return fn
}

func (fn *FlowNet) AddEdge(u, v, cap int) {
	fn.Graph[u] = append(fn.Graph[u], FlowEdge{v, len(fn.Graph[v]), cap})
	fn.Graph[v] = append(fn.Graph[v], FlowEdge{u, len(fn.Graph[u]) - 1, 0})
}

func (fn *FlowNet) bfs(s int) {
	for _, v := range fn.Q {
		fn.Level[v] = -1
		fn.Iter[v] = 0
	}

	fn.Q = fn.Q[:0]
	fn.Q = append(fn.Q, s)
	fn.Level[s] = 0

	for head := 0; head < len(fn.Q); head++ {
		v := fn.Q[head]
		for _, e := range fn.Graph[v] {
			if e.Cap > 0 && fn.Level[e.To] < 0 {
				fn.Level[e.To] = fn.Level[v] + 1
				fn.Q = append(fn.Q, e.To)
			}
		}
	}
}

func (fn *FlowNet) dfs(v, t, f int) int {
	if v == t {
		return f
	}
	for ; fn.Iter[v] < len(fn.Graph[v]); fn.Iter[v]++ {
		e := &fn.Graph[v][fn.Iter[v]]
		if e.Cap > 0 && fn.Level[v] < fn.Level[e.To] {
			d := fn.dfs(e.To, t, min(f, e.Cap))
			if d > 0 {
				e.Cap -= d
				fn.Graph[e.To][e.Rev].Cap += d
				return d
			}
		}
	}
	return 0
}

func (fn *FlowNet) MaxFlow(s, t int) int {
	flow := 0
	for {
		fn.bfs(s)
		if fn.Level[t] < 0 {
			return flow
		}

		for {
			f := fn.dfs(s, t, math.MaxInt32)
			if f == 0 {
				break
			}
			flow += f
		}
	}
}

// reachableFrom reports which residual-network nodes remain reachable from s.
func (fn *FlowNet) ReachableFrom(s int) []bool {
	n := len(fn.Graph)
	vis := make([]bool, n)
	q := []int{s}
	vis[s] = true
	for len(q) > 0 {
		v := q[0]
		q = q[1:]
		for _, e := range fn.Graph[v] {
			if e.Cap > 0 && !vis[e.To] {
				vis[e.To] = true
				q = append(q, e.To)
			}
		}
	}
	return vis
}
