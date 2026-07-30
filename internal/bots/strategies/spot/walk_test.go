package spot

import "testing"

func TestRandomWalk_StaysWithinStepAndNeverBelowOne(t *testing.T) {
	w := NewRandomWalk(5, 3)
	prev := int64(5)
	for i := 0; i < 1000; i++ {
		next := w.Next()
		if next < 1 {
			t.Fatalf("price = %d, want >= 1", next)
		}
		delta := next - prev
		if delta > 3 || delta < -3 {
			t.Fatalf("step = %d, want within [-3, 3]", delta)
		}
		prev = next
	}
}
