package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jsmestad/syncai/internal/load"
)

type treeEntry struct {
	path       string
	typeBits   os.FileMode
	permission os.FileMode
	data       []byte
}

type goldenTarget struct {
	name      string
	source    string
	inventory []treeEntry
}

func TestCompleteExampleIsDeterministic(t *testing.T) {
	source := completeExampleSource(t)
	first := renderCompleteExample(t, source, "openai", "")
	second := renderCompleteExample(t, source, "openai", "")

	assertTreeInventoriesEqual(t, inventoryTree(t, first), inventoryTree(t, second))
}

func TestCompleteExampleMatchesGoldenTrees(t *testing.T) {
	source := completeExampleSource(t)
	repositoryRoot := filepath.Dir(filepath.Dir(source))
	actualRoot := renderCompleteExample(t, source, "openai", "")

	var targets []goldenTarget
	for _, target := range []struct {
		name         string
		relativeRoot string
	}{
		{name: "pi", relativeRoot: ".pi"},
		{name: "omp", relativeRoot: ".omp"},
		{name: "claude", relativeRoot: ".claude"},
		{name: "codex", relativeRoot: ".codex"},
		{name: "opencode", relativeRoot: filepath.Join(".config", "opencode")},
		{name: "antigravity", relativeRoot: filepath.Join(".gemini", "antigravity-cli", "plugins", "dfiles")},
	} {
		actual := filepath.Join(actualRoot, target.relativeRoot)
		targets = append(targets, goldenTarget{name: target.name, source: actual, inventory: inventoryTree(t, actual)})
	}

	goldenRoot := filepath.Join(repositoryRoot, "testdata", "golden")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := updateGoldenTrees(goldenRoot, targets); err != nil {
			t.Fatalf("updating golden trees: %v", err)
		}
	}

	for _, target := range targets {
		t.Run(target.name, func(t *testing.T) {
			assertTreeInventoriesEqual(t, inventoryTree(t, filepath.Join(goldenRoot, target.name)), target.inventory)
		})
	}
}

func TestUpdateGoldenTreesPreservesExistingTreeOnStagingFailure(t *testing.T) {
	parent := t.TempDir()
	goldenRoot := filepath.Join(parent, "golden")
	if err := os.MkdirAll(goldenRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(goldenRoot, "sentinel.txt"), []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	validSource := t.TempDir()
	if err := os.WriteFile(filepath.Join(validSource, "rendered.txt"), []byte("rendered"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := inventoryTree(t, goldenRoot)
	err := updateGoldenTrees(goldenRoot, []goldenTarget{
		{name: "valid", source: validSource, inventory: inventoryTree(t, validSource)},
		{name: "missing", source: filepath.Join(parent, "missing")},
	})
	if err == nil {
		t.Fatal("expected staging failure")
	}
	assertTreeInventoriesEqual(t, before, inventoryTree(t, goldenRoot))
	matches, globErr := filepath.Glob(filepath.Join(parent, ".golden.stage-*"))
	if globErr != nil {
		t.Fatal(globErr)
	}
	if len(matches) != 0 {
		t.Fatalf("staging directories remain after failure: %v", matches)
	}
}

func updateGoldenTrees(goldenRoot string, targets []goldenTarget) error {
	parent := filepath.Dir(goldenRoot)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return fmt.Errorf("creating golden parent: %w", err)
	}
	stage, err := os.MkdirTemp(parent, "."+filepath.Base(goldenRoot)+".stage-*")
	if err != nil {
		return fmt.Errorf("creating golden staging directory: %w", err)
	}
	if err := os.Chmod(stage, 0o755); err != nil {
		_ = os.RemoveAll(stage)
		return fmt.Errorf("setting golden staging permissions: %w", err)
	}
	stageExists := true
	defer func() {
		if stageExists {
			_ = os.RemoveAll(stage)
		}
	}()

	for _, target := range targets {
		if err := load.CopyDir(stage, target.source, target.name); err != nil {
			return fmt.Errorf("staging %s golden tree: %w", target.name, err)
		}
	}
	for _, target := range targets {
		staged, err := readTree(filepath.Join(stage, target.name))
		if err != nil {
			return fmt.Errorf("inventorying staged %s golden tree: %w", target.name, err)
		}
		if !reflect.DeepEqual(staged, target.inventory) {
			return fmt.Errorf("staged %s golden tree does not match rendered inventory", target.name)
		}
	}

	if _, err := os.Lstat(goldenRoot); os.IsNotExist(err) {
		if err := os.Rename(stage, goldenRoot); err != nil {
			return fmt.Errorf("installing golden tree: %w", err)
		}
		stageExists = false
		return nil
	} else if err != nil {
		return fmt.Errorf("inspecting golden tree: %w", err)
	}

	backup, err := os.MkdirTemp(parent, "."+filepath.Base(goldenRoot)+".backup-*")
	if err != nil {
		return fmt.Errorf("reserving golden backup: %w", err)
	}
	if err := os.Remove(backup); err != nil {
		return fmt.Errorf("reserving golden backup path: %w", err)
	}
	if err := os.Rename(goldenRoot, backup); err != nil {
		return fmt.Errorf("backing up golden tree: %w", err)
	}
	if err := os.Rename(stage, goldenRoot); err != nil {
		if rollbackErr := os.Rename(backup, goldenRoot); rollbackErr != nil {
			return fmt.Errorf("installing golden tree: %v; rollback failed: %w", err, rollbackErr)
		}
		return fmt.Errorf("installing golden tree: %w", err)
	}
	stageExists = false
	if err := os.RemoveAll(backup); err != nil {
		return fmt.Errorf("removing golden backup: %w", err)
	}
	return nil
}

func inventoryTree(t *testing.T, root string) []treeEntry {
	t.Helper()
	inventory, err := readTree(root)
	if err != nil {
		t.Fatalf("inventorying %s: %v", root, err)
	}
	return inventory
}

func readTree(root string) ([]treeEntry, error) {
	var inventory []treeEntry
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if relative == "." {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		item := treeEntry{path: filepath.ToSlash(relative), typeBits: info.Mode() & os.ModeType}
		switch {
		case info.Mode().IsRegular():
			item.permission = info.Mode().Perm()
			item.data, err = os.ReadFile(path)
		case info.Mode()&os.ModeSymlink != 0:
			var target string
			target, err = os.Readlink(path)
			item.data = []byte(target)
		}
		if err != nil {
			return err
		}
		inventory = append(inventory, item)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return inventory, nil
}

func assertTreeInventoriesEqual(t *testing.T, want, got []treeEntry) {
	t.Helper()
	if reflect.DeepEqual(got, want) {
		return
	}
	if len(got) != len(want) {
		t.Errorf("tree entry count = %d, want %d", len(got), len(want))
	}
	limit := len(want)
	if len(got) < limit {
		limit = len(got)
	}
	for index := 0; index < limit; index++ {
		wantEntry := want[index]
		gotEntry := got[index]
		if gotEntry.path != wantEntry.path {
			t.Errorf("tree entry %d path = %q, want %q", index, gotEntry.path, wantEntry.path)
			continue
		}
		if gotEntry.typeBits != wantEntry.typeBits {
			t.Errorf("%s type = %s, want %s", gotEntry.path, gotEntry.typeBits, wantEntry.typeBits)
		}
		if gotEntry.permission != wantEntry.permission {
			t.Errorf("%s permissions = %04o, want %04o", gotEntry.path, gotEntry.permission, wantEntry.permission)
		}
		if !bytes.Equal(gotEntry.data, wantEntry.data) {
			t.Errorf("%s bytes differ%s", gotEntry.path, describeByteDifference(wantEntry.data, gotEntry.data))
		}
	}
}

func describeByteDifference(want, got []byte) string {
	if bytes.Equal(want, got) {
		return ""
	}
	return fmt.Sprintf(" (got %d bytes, want %d)", len(got), len(want))
}
