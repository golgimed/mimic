package registry

import "testing"

func TestEnabledNilReturnsAll(t *testing.T) {
	r := New()
	r.Register(&Provider{Name: "zenvia"})
	r.Register(&Provider{Name: "integraicp"})

	got := r.Enabled(nil)
	if len(got) != 2 {
		t.Fatalf("Enabled(nil) = %d providers, want 2", len(got))
	}
}

func TestEnabledFiltersByName(t *testing.T) {
	r := New()
	r.Register(&Provider{Name: "zenvia"})
	r.Register(&Provider{Name: "integraicp"})

	got := r.Enabled([]string{"zenvia"})
	if len(got) != 1 || got[0].Name != "zenvia" {
		t.Fatalf("Enabled([zenvia]) = %v, want [zenvia]", got)
	}
}
