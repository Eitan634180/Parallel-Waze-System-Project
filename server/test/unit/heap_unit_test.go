package unit_test

import (
	"testing"

	"nav-system/src/utilities"
)

func TestHeapMaintainsAscendingOrder(t *testing.T) {
	h := utilities.NewHeap(func(a, b int) bool { return a < b })
	for _, value := range []int{5, 1, 3, 2, 4} {
		h.Push(value)
	}

	for expected := 1; expected <= 5; expected++ {
		if got := h.Pop(); got != expected {
			t.Fatalf("pop %d: got %d want %d", expected, got, expected)
		}
	}
}

func TestHeapHandlesDuplicatePriorities(t *testing.T) {
	h := utilities.NewHeap(func(a, b int) bool { return a < b })
	h.Push(2)
	h.Push(2)
	h.Push(1)

	if got := h.Pop(); got != 1 {
		t.Fatalf("first pop got %d want 1", got)
	}
	if got := h.Pop(); got != 2 {
		t.Fatalf("second pop got %d want 2", got)
	}
	if got := h.Pop(); got != 2 {
		t.Fatalf("third pop got %d want 2", got)
	}
}

func TestHeapPopOnEmptyPanics(t *testing.T) {
	h := utilities.NewHeap(func(a, b int) bool { return a < b })

	defer func() {
		if recover() == nil {
			t.Fatal("expected empty pop to panic")
		}
	}()

	_ = h.Pop()
}
