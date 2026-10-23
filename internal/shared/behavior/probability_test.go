package behavior

import "testing"

func TestShouldApplyNil(t *testing.T) {
	if !ShouldApply(r, nil) {
		t.Fatal("nil probability should always apply")
	}
}

func TestShouldApplyBounds(t *testing.T) {
	one := 1.0
	if !ShouldApply(r, &one) {
		t.Fatal("probability 1 should always apply")
	}
	zero := 0.0
	if ShouldApply(r, &zero) {
		t.Fatal("probability 0 should never apply")
	}
	above := 1.5
	if !ShouldApply(r, &above) {
		t.Fatal("probability > 1 should always apply")
	}
	below := -0.5
	if ShouldApply(r, &below) {
		t.Fatal("probability < 0 should never apply")
	}
}

func TestShouldApplyDistribution(t *testing.T) {
	p := 0.5
	hits := 0
	const n = 20000
	for i := 0; i < n; i++ {
		if ShouldApply(r, &p) {
			hits++
		}
	}
	ratio := float64(hits) / n
	if ratio < 0.45 || ratio > 0.55 {
		t.Fatalf("expected ratio near 0.5, got %f", ratio)
	}
}
