package openapi

import "testing"

func TestMergeStrictFailsOnConflict(t *testing.T) {
	a := []Route{{Method: "GET", Path: "/pets", Source: "a.yaml"}}
	b := []Route{{Method: "GET", Path: "/pets", Source: "b.yaml"}}

	if _, err := Merge([][]Route{a, b}, ConflictStrict); err == nil {
		t.Fatal("expected conflict error")
	}
}

func TestMergeModeLastWins(t *testing.T) {
	a := []Route{{Method: "GET", Path: "/pets", Source: "a.yaml"}}
	b := []Route{{Method: "GET", Path: "/pets", Source: "b.yaml"}}

	out, err := Merge([][]Route{a, b}, ConflictMerge)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 || out[0].Source != "b.yaml" {
		t.Fatalf("expected last spec (b.yaml) to win, got %+v", out)
	}
}

func TestPriorityModeFirstWins(t *testing.T) {
	a := []Route{{Method: "GET", Path: "/pets", Source: "a.yaml"}}
	b := []Route{{Method: "GET", Path: "/pets", Source: "b.yaml"}}

	out, err := Merge([][]Route{a, b}, ConflictPriority)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 1 || out[0].Source != "a.yaml" {
		t.Fatalf("expected first (highest-priority) spec (a.yaml) to win, got %+v", out)
	}
}

func TestMergeNoConflictKeepsAllRoutes(t *testing.T) {
	a := []Route{{Method: "GET", Path: "/pets", Source: "a.yaml"}}
	b := []Route{{Method: "GET", Path: "/owners", Source: "b.yaml"}}

	out, err := Merge([][]Route{a, b}, ConflictStrict)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 routes, got %d: %+v", len(out), out)
	}
}

func TestMergePreservesDiscoveryOrder(t *testing.T) {
	a := []Route{
		{Method: "GET", Path: "/z", Source: "a.yaml"},
		{Method: "GET", Path: "/a", Source: "a.yaml"},
	}

	out, err := Merge([][]Route{a}, ConflictStrict)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out) != 2 || out[0].Path != "/z" || out[1].Path != "/a" {
		t.Fatalf("expected discovery order preserved, got %+v", out)
	}
}
