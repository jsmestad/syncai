package importer

import (
	"os"
	"path/filepath"
	"testing"
)

func writeSkillFile(t *testing.T, path, body string) {
	t.Helper()
	writeFile(t, path, body)
}

// S1: ScanSkills returns one candidate per orphan skill directory across
// tool dirs.
func TestScanSkillsFindsOrphans(t *testing.T) {
	home := t.TempDir()
	source := t.TempDir()
	writeSkillFile(t, filepath.Join(home, ".claude/skills/minga-preview/SKILL.md"), "---\nname: minga-preview\n---\nBody.\n")
	writeSkillFile(t, filepath.Join(home, ".pi/agent/skills/frontend-design/SKILL.md"), "---\nname: frontend-design\n---\nBody.\n")
	writeSkillFile(t, filepath.Join(source, "skills/already-tracked/SKILL.md"), "---\nname: already-tracked\n---\nBody.\n")

	candidates, err := ScanSkills(home, source)
	if err != nil {
		t.Fatalf("ScanSkills: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d: %+v", len(candidates), candidates)
	}
	names := map[string]bool{}
	for _, c := range candidates {
		names[c.Name] = true
	}
	if !names["minga-preview"] || !names["frontend-design"] {
		t.Errorf("missing expected names: %v", names)
	}
}

// S2: ScanSkills excludes directories whose name matches a source skill.
func TestScanSkillsSkipsKnownNames(t *testing.T) {
	home := t.TempDir()
	source := t.TempDir()
	writeSkillFile(t, filepath.Join(home, ".claude/skills/plan/SKILL.md"), "x")
	writeSkillFile(t, filepath.Join(source, "skills/plan/SKILL.md"), "x")

	candidates, err := ScanSkills(home, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Errorf("known name should be excluded, got %+v", candidates)
	}
}

// S3: ScanSkills skips dot-prefixed directories (e.g. Codex's built-in
// .system/ skills dir, which isn't user content).
func TestScanSkillsSkipsDotPrefixed(t *testing.T) {
	home := t.TempDir()
	source := t.TempDir()
	writeSkillFile(t, filepath.Join(home, ".codex/skills/.system/skill-creator/SKILL.md"), "x")

	candidates, err := ScanSkills(home, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Errorf("dot-prefixed dir should be excluded, got %+v", candidates)
	}
}

func TestScanSkillsSkipsSyncAIBuiltIn(t *testing.T) {
	home := t.TempDir()
	source := t.TempDir()
	writeSkillFile(t, filepath.Join(home, ".claude/skills/syncai/SKILL.md"), "---\nname: syncai\n---\n")

	candidates, err := ScanSkills(home, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Errorf("SyncAI built-in should be excluded, got %+v", candidates)
	}
}

// S4: ScanSkills requires a SKILL.md inside the directory to treat it as a
// candidate, guarding against unrelated junk directories.
func TestScanSkillsRequiresSkillMD(t *testing.T) {
	home := t.TempDir()
	source := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude/skills/not-a-skill"), 0o755); err != nil {
		t.Fatal(err)
	}

	candidates, err := ScanSkills(home, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Errorf("directory without SKILL.md should be excluded, got %+v", candidates)
	}
}

// S5: ScanSkills skips symlinked directories — those are already managed
// elsewhere (e.g. a project-local skill symlinked in from another
// dfiles-managed tree).
func TestScanSkillsSkipsSymlinks(t *testing.T) {
	home := t.TempDir()
	source := t.TempDir()
	realDir := filepath.Join(home, "elsewhere", "git-identity")
	writeSkillFile(t, filepath.Join(realDir, "SKILL.md"), "x")
	linkPath := filepath.Join(home, ".pi/agent/skills/git-identity")
	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDir, linkPath); err != nil {
		t.Fatal(err)
	}

	candidates, err := ScanSkills(home, source)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 0 {
		t.Errorf("symlinked dir should be excluded, got %+v", candidates)
	}
}

// S6: PortSkill copies the installed skill directory verbatim into source,
// preserving nested files (e.g. helper scripts alongside SKILL.md).
func TestPortSkillCopiesDirectory(t *testing.T) {
	home := t.TempDir()
	source := t.TempDir()
	input := filepath.Join(home, ".claude/skills/minga-preview")
	writeSkillFile(t, filepath.Join(input, "SKILL.md"), "---\nname: minga-preview\n---\nBody.\n")
	writeSkillFile(t, filepath.Join(input, "scripts/capture.sh"), "#!/bin/sh\necho hi\n")

	c := SkillCandidate{
		Name:       "minga-preview",
		Tool:       "claude",
		InputPath:  input,
		SourcePath: filepath.Join(source, "skills", "minga-preview"),
	}
	if err := PortSkill(source, c); err != nil {
		t.Fatalf("PortSkill: %v", err)
	}
	skillMD, err := os.ReadFile(filepath.Join(c.SourcePath, "SKILL.md"))
	if err != nil {
		t.Fatalf("reading ported SKILL.md: %v", err)
	}
	if string(skillMD) != "---\nname: minga-preview\n---\nBody.\n" {
		t.Errorf("SKILL.md content mismatch: %q", skillMD)
	}
	if _, err := os.Stat(filepath.Join(c.SourcePath, "scripts/capture.sh")); err != nil {
		t.Errorf("expected nested script to be copied: %v", err)
	}
}
