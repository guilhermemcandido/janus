package engine

import "testing"

func TestTickArray_GetOnUnsetPriceReturnsNil(t *testing.T) {
	var arr tickArray
	if got := arr.get(50); got != nil {
		t.Fatalf("get(50) on empty array = %v, want nil", got)
	}
}

func TestTickArray_SetThenGetRoundTrips(t *testing.T) {
	var arr tickArray
	pl := NewPriceLevel(50)

	if ok := arr.set(50, pl); !ok {
		t.Fatalf("set(50) = false, want true")
	}
	if got := arr.get(50); got != pl {
		t.Fatalf("get(50) = %v, want %v", got, pl)
	}
}

func TestTickArray_SetGrowsToFitHigherPrices(t *testing.T) {
	var arr tickArray
	low, high := NewPriceLevel(1), NewPriceLevel(1000)

	arr.set(1, low)
	arr.set(1000, high)

	if got := arr.get(1); got != low {
		t.Fatalf("get(1) = %v, want %v", got, low)
	}
	if got := arr.get(1000); got != high {
		t.Fatalf("get(1000) = %v, want %v", got, high)
	}
}

func TestTickArray_SetRejectsNonPositiveOrAboveMax(t *testing.T) {
	var arr tickArray
	for _, price := range []int64{0, -1, maxTickPrice + 1} {
		if ok := arr.set(price, NewPriceLevel(price)); ok {
			t.Fatalf("set(%d) = true, want false", price)
		}
	}
}

func TestTickArray_SetAcceptsMaxTickPrice(t *testing.T) {
	var arr tickArray
	if ok := arr.set(maxTickPrice, NewPriceLevel(maxTickPrice)); !ok {
		t.Fatalf("set(maxTickPrice) = false, want true")
	}
}

func TestTickArray_DeleteClearsSlotWithoutShrinking(t *testing.T) {
	var arr tickArray
	arr.set(50, NewPriceLevel(50))

	arr.delete(50)

	if got := arr.get(50); got != nil {
		t.Fatalf("get(50) after delete = %v, want nil", got)
	}
}

func TestTickArray_DeleteOnUnsetPriceIsNoop(t *testing.T) {
	var arr tickArray
	arr.delete(50) // must not panic on an empty backing array
}
