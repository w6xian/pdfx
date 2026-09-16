package pdfx

import "testing"

func TestMergeCollinearLines(t *testing.T) {
	in := [][4]float64{
		{153, 215.44, 317, 215.44},
		{153, 215.44, 561, 215.44},
		{34, 34, 34, 100},
		{34, 100, 34, 153},
		{34, 200, 34, 260},
	}
	got := mergeCollinearLines(in)
	if len(got) != 3 {
		t.Fatalf("want 3 segments, got %d: %v", len(got), got)
	}
	want := [][4]float64{
		{153, 215.44, 561, 215.44},
		{34, 34, 34, 153},
		{34, 200, 34, 260},
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("seg %d = %v, want %v", i, got[i], want[i])
		}
	}
}
