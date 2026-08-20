package schema

import (
	"strings"
	"testing"
)

// S1: Happy path parse populates name, description, targets, scope.
func TestParseAgentHappyPath(t *testing.T) {
	src := `---
name: archie
description: Architectural advisor
targets: pi, claude
scope: home
tools: read, bash
---

You are an architectural advisor.
`
	a, err := ParseAgent("test/archie.md", []byte(src))
	if err != nil {
		t.Fatalf("ParseAgent: %v", err)
	}
	if a.Name != "archie" {
		t.Errorf("Name: %q", a.Name)
	}
	if a.Description != "Architectural advisor" {
		t.Errorf("Description: %q", a.Description)
	}
	if len(a.Targets) != 2 || a.Targets[0] != "pi" || a.Targets[1] != "claude" {
		t.Errorf("Targets: %v", a.Targets)
	}
	if !equalStrings(a.Scope, []string{"home"}) {
		t.Errorf("Scope: %v", a.Scope)
	}
	if a.Body != "\nYou are an architectural advisor.\n" {
		t.Errorf("Body: %q", a.Body)
	}
}

// S2: Targets default to [pi] when absent.
func TestParseAgentDefaultsTargetsToPi(t *testing.T) {
	src := `---
name: foo
description: A no-targets agent.
---

body
`
	a, err := ParseAgent("test/foo.md", []byte(src))
	if err != nil {
		t.Fatalf("ParseAgent: %v", err)
	}
	if len(a.Targets) != 1 || a.Targets[0] != "pi" {
		t.Errorf("expected default [pi], got %v", a.Targets)
	}
}

// S3: Missing leading --- errors.
func TestParseAgentMissingLeadingDelimiter(t *testing.T) {
	src := "name: foo\n"
	_, err := ParseAgent("test/foo.md", []byte(src))
	if err == nil || !strings.Contains(err.Error(), "leading frontmatter delimiter") {
		t.Errorf("expected leading-delimiter error, got %v", err)
	}
}

// S4: Missing closing --- errors.
func TestParseAgentMissingClosingDelimiter(t *testing.T) {
	src := "---\nname: foo\ndescription: bar\n"
	_, err := ParseAgent("test/foo.md", []byte(src))
	if err == nil || !strings.Contains(err.Error(), "closing frontmatter delimiter") {
		t.Errorf("expected closing-delimiter error, got %v", err)
	}
}

// S5: Missing name → error.
func TestParseAgentMissingNameErrors(t *testing.T) {
	src := "---\ndescription: missing name\n---\n\nbody\n"
	_, err := ParseAgent("test/foo.md", []byte(src))
	if err == nil || !strings.Contains(err.Error(), "name is required") {
		t.Errorf("expected name-required error, got %v", err)
	}
}

// S6: Missing description → error.
func TestParseAgentMissingDescriptionErrors(t *testing.T) {
	src := "---\nname: foo\n---\n\nbody\n"
	_, err := ParseAgent("test/foo.md", []byte(src))
	if err == nil || !strings.Contains(err.Error(), "description is required") {
		t.Errorf("expected description-required error, got %v", err)
	}
}

// S7: Unknown target → error naming the bad value.
func TestParseAgentUnknownTargetErrors(t *testing.T) {
	src := `---
name: foo
description: bar
targets: pi, totally-not-a-real-target
---

body
`
	_, err := ParseAgent("test/foo.md", []byte(src))
	if err == nil || !strings.Contains(err.Error(), "totally-not-a-real-target") {
		t.Errorf("expected error naming bad target, got %v", err)
	}
}

// S8: Values containing colons are preserved verbatim. The line parser
// uses the FIRST `:` as the delimiter; everything after is the value.
func TestParseAgentMultipleColonsInValue(t *testing.T) {
	src := `---
name: foo
description: foo: bar baz with multiple: colons
---

body
`
	a, err := ParseAgent("test/foo.md", []byte(src))
	if err != nil {
		t.Fatalf("ParseAgent: %v", err)
	}
	want := "foo: bar baz with multiple: colons"
	if a.Description != want {
		t.Errorf("Description: got %q, want %q", a.Description, want)
	}
}

// S9: Blank frontmatter lines are skipped, not treated as fields.
func TestParseAgentSkipsBlankLines(t *testing.T) {
	src := "---\nname: foo\n\ndescription: bar\n\n\n---\n\nbody\n"
	a, err := ParseAgent("test/foo.md", []byte(src))
	if err != nil {
		t.Fatalf("ParseAgent: %v", err)
	}
	if a.Description != "bar" {
		t.Errorf("Description: %q", a.Description)
	}
}

// S10: Lines without a colon are skipped silently (current behavior — pin
// it so future refactors notice if this changes).
func TestParseAgentSkipsLinesWithoutColon(t *testing.T) {
	src := "---\nname: foo\nrandom-line-no-colon\ndescription: bar\n---\n\nbody\n"
	a, err := ParseAgent("test/foo.md", []byte(src))
	if err != nil {
		t.Fatalf("ParseAgent should tolerate colon-less lines: %v", err)
	}
	if a.Name != "foo" || a.Description != "bar" {
		t.Errorf("fields wrong: %+v", a)
	}
}

// S11: Source field order is preserved in Fields. Pi renderer relies on
// this to emit in the same order as the source file.
func TestParseAgentPreservesFieldOrder(t *testing.T) {
	src := `---
name: foo
description: bar
targets: pi
scope: home
tools: read, bash
---

body
`
	a, err := ParseAgent("test/foo.md", []byte(src))
	if err != nil {
		t.Fatalf("ParseAgent: %v", err)
	}
	wantOrder := []string{"name", "description", "targets", "scope", "tools"}
	if len(a.Fields) != len(wantOrder) {
		t.Fatalf("Fields count: got %d, want %d", len(a.Fields), len(wantOrder))
	}
	for i, want := range wantOrder {
		if a.Fields[i].Key != want {
			t.Errorf("Fields[%d].Key: got %q, want %q", i, a.Fields[i].Key, want)
		}
	}
}

// S12: Empty value after `key:` is preserved as empty string.
func TestParseAgentEmptyValuePreserved(t *testing.T) {
	src := "---\nname: foo\ndescription: bar\noutput:\n---\n\nbody\n"
	a, err := ParseAgent("test/foo.md", []byte(src))
	if err != nil {
		t.Fatalf("ParseAgent: %v", err)
	}
	if a.Lookup("output") != "" {
		t.Errorf("expected empty output value, got %q", a.Lookup("output"))
	}
	// But the field itself should be present in Fields (so Pi renderer can
	// pass it through).
	found := false
	for _, kv := range a.Fields {
		if kv.Key == "output" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("output: field should be present in Fields even with empty value")
	}
}

// S13: HasTarget matrix.
func TestAgentHasTarget(t *testing.T) {
	a := &Agent{Targets: []string{"pi", "claude"}}
	cases := []struct {
		t    Target
		want bool
	}{
		{TargetPi, true},
		{TargetClaude, true},
		{TargetCodex, false},
		{TargetOpenCode, false},
	}
	for _, c := range cases {
		if got := a.HasTarget(c.t); got != c.want {
			t.Errorf("HasTarget(%s): got %v, want %v", c.t, got, c.want)
		}
	}
}

// S14: MatchesScope matrix.
func TestAgentMatchesScope(t *testing.T) {
	cases := []struct {
		agentScope []string
		filter     string
		want       bool
		desc       string
	}{
		{nil, "", true, "no scope, no filter"},
		{nil, "home", true, "universal agent matches any filter"},
		{[]string{"home"}, "", true, "no filter matches any agent"},
		{[]string{"home"}, "home", true, "exact match"},
		{[]string{"home"}, "work", false, "explicit mismatch"},
		{[]string{"home", "work"}, "home", true, "list contains filter"},
		{[]string{"home", "work"}, "work", true, "list contains filter (other)"},
		{[]string{"home", "work"}, "", true, "no filter matches list"},
	}
	for _, c := range cases {
		a := &Agent{Scope: c.agentScope}
		if got := a.MatchesScope(c.filter); got != c.want {
			t.Errorf("%s: MatchesScope(%q) on Scope=%v: got %v, want %v",
				c.desc, c.filter, c.agentScope, got, c.want)
		}
	}
}

func TestAgentScopeCSVParse(t *testing.T) {
	raw := []byte("---\nname: x\ndescription: y\nscope: home, work\n---\nbody\n")
	a, err := ParseAgent("x.md", raw)
	if err != nil {
		t.Fatal(err)
	}
	if !equalStrings(a.Scope, []string{"home", "work"}) {
		t.Errorf("want [home work], got %v", a.Scope)
	}
}

func TestAgentInvalidScopeRejected(t *testing.T) {
	raw := []byte("---\nname: x\ndescription: y\nscope: production\n---\nbody\n")
	if _, err := ParseAgent("x.md", raw); err == nil {
		t.Error("expected error for unknown scope, got nil")
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// S15: Lookup returns "" for absent keys.
func TestAgentLookupAbsent(t *testing.T) {
	a := &Agent{Fields: []KV{{Key: "tools", Value: "read"}}}
	if a.Lookup("modelRole") != "" {
		t.Errorf("expected empty for absent key, got %q", a.Lookup("modelRole"))
	}
	if a.Lookup("tools") != "read" {
		t.Errorf("expected present key to resolve")
	}
}

// S16: SplitCSV trims, drops empties, returns nil for empty input.
func TestSplitCSV(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"a", []string{"a"}},
		{"a, b, c", []string{"a", "b", "c"}},
		{" a , b ,  c ", []string{"a", "b", "c"}},
		{"a, , c", []string{"a", "c"}},
		{",,", nil},
	}
	for _, c := range cases {
		got := SplitCSV(c.in)
		if !equalSlices(got, c.want) {
			t.Errorf("SplitCSV(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
