package utilities

// Heap is a typed binary min-heap.
type Heap[T any] struct {
	items []T
	less  func(a, b T) bool
}

// NewHeap returns an empty heap ordered by less.
func NewHeap[T any](less func(a, b T) bool) *Heap[T] {
	return &Heap[T]{less: less}
}

// Len reports the number of items currently stored in the heap.
func (h *Heap[T]) Len() int {
	return len(h.items)
}

// Push inserts x into the heap.
func (h *Heap[T]) Push(x T) {
	h.items = append(h.items, x)
	h.up(len(h.items) - 1)
}

// Pop removes and returns the minimum item according to less.
// It panics if the heap is empty.
func (h *Heap[T]) Pop() T {
	n := len(h.items)
	if n == 0 {
		panic("utilities.Heap.Pop on empty heap")
	}

	last := n - 1
	h.swap(0, last)
	item := h.items[last]
	h.items = h.items[:last]
	if last > 0 {
		h.down(0)
	}
	return item
}

func (h *Heap[T]) up(child int) {
	for child > 0 {
		parent := (child - 1) / 2
		if !h.less(h.items[child], h.items[parent]) {
			return
		}
		h.swap(child, parent)
		child = parent
	}
}

func (h *Heap[T]) down(parent int) {
	n := len(h.items)
	for {
		left := 2*parent + 1
		if left >= n || left < 0 {
			return
		}

		best := left
		right := left + 1
		if right < n && h.less(h.items[right], h.items[left]) {
			best = right
		}

		if !h.less(h.items[best], h.items[parent]) {
			return
		}

		h.swap(parent, best)
		parent = best
	}
}

func (h *Heap[T]) swap(i, j int) {
	h.items[i], h.items[j] = h.items[j], h.items[i]
}
