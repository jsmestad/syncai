package renderers

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteSkillsRejectsSourceConflictWithBuiltIn(t *testing.T) {
	source := filepath.Join(t.TempDir(), "syncai")
	if err := os.MkdirAll(source, 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := WriteSkills(t.TempDir(), "skills", []string{source}, []BuiltInSkill{{Name: "syncai", Content: []byte("built in")}})
	if err == nil || !strings.Contains(err.Error(), "conflicts with a built-in SyncAI skill") {
		t.Fatalf("WriteSkills error = %v", err)
	}
}
