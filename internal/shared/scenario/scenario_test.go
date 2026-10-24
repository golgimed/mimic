package scenario

import (
	"strings"
	"testing"
	"time"
)

const sample = `
- name: doc-signed
  resource: doc
  description: signed after a while
  initial: PENDING
  transitions:
    - to: SIGNED
      after: 2s
      meta: {by: alice}
`

func TestParseAndWalk(t *testing.T) {
	list, err := Parse("prov", []byte(sample), func(resource, from, to string) bool {
		return resource == "doc" && from == "PENDING" && to == "SIGNED"
	})
	if err != nil {
		t.Fatal(err)
	}
	s := list[0]
	if s.Provider != "prov" || strings.Join(s.Lifecycle(), ">") != "PENDING>SIGNED" {
		t.Fatalf("parsed %+v", s)
	}
	step, ok := s.Next("PENDING")
	if !ok || step.To != "SIGNED" || time.Duration(*step.After) != 2*time.Second || step.Meta["by"] != "alice" {
		t.Fatalf("Next(PENDING) = %+v, %v", step, ok)
	}
	if _, ok := s.Next("SIGNED"); ok {
		t.Fatal("terminal state has a next step")
	}
}

func TestParseRejectsInvalid(t *testing.T) {
	never := func(string, string, string) bool { return false }
	cases := map[string]string{
		"disallowed edge": sample,
		"missing name":    "- {description: x}",
		"steps without initial": `
- name: x
  resource: doc
  description: x
  transitions: [{to: SIGNED}]`,
		"bad duration": `
- name: x
  resource: doc
  description: x
  initial: PENDING
  transitions: [{to: SIGNED, after: soon}]`,
	}
	for name, raw := range cases {
		if _, err := Parse("prov", []byte(raw), never); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestRegisterRejectsCrossProviderDuplicates(t *testing.T) {
	if err := Register(Scenario{Name: "dup-test", Provider: "a"}); err != nil {
		t.Fatal(err)
	}
	if err := Register(Scenario{Name: "dup-test", Provider: "a"}); err != nil {
		t.Fatalf("same provider re-register: %v", err)
	}
	if err := Register(Scenario{Name: "dup-test", Provider: "b"}); err == nil {
		t.Fatal("expected a cross-provider duplicate to be rejected")
	}
}
