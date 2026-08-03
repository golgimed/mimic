package behavior

import (
	"math"
	"testing"
	"time"
)

func TestSampleFixed(t *testing.T) {
	d := Distribution{Kind: DistFixed, MS: 100}
	if got := d.Sample(); got != 100*time.Millisecond {
		t.Fatalf("expected 100ms, got %v", got)
	}
}

func TestSampleUniformBounds(t *testing.T) {
	d := Distribution{Kind: DistUniform, MinMS: 50, MaxMS: 60}
	for i := 0; i < 1000; i++ {
		got := d.Sample()
		if got < 50*time.Millisecond || got >= 60*time.Millisecond+time.Millisecond {
			t.Fatalf("sample %v out of [50,60]ms bounds", got)
		}
	}
}

func TestSampleNormalMean(t *testing.T) {
	d := Distribution{Kind: DistNormal, MeanMS: 100, StdDev: 10}
	const n = 20000
	var sum float64
	for i := 0; i < n; i++ {
		sum += float64(d.Sample()) / float64(time.Millisecond)
	}
	mean := sum / n
	if math.Abs(mean-100) > 3 {
		t.Fatalf("expected mean near 100ms, got %f", mean)
	}
}

func TestSampleNormalClampsNegative(t *testing.T) {
	d := Distribution{Kind: DistNormal, MeanMS: -1000, StdDev: 1}
	if got := d.Sample(); got < 0 {
		t.Fatalf("expected non-negative duration, got %v", got)
	}
}

func TestParseDistribution(t *testing.T) {
	d, err := ParseDistribution(`{"kind":"uniform","minMs":10,"maxMs":20}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if d.Kind != DistUniform || d.MinMS != 10 || d.MaxMS != 20 {
		t.Fatalf("unexpected parse result: %+v", d)
	}
}

func TestParseDistributionInvalidJSON(t *testing.T) {
	if _, err := ParseDistribution("not json"); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}
