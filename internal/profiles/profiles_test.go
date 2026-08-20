package profiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadWithEnvironmentDeepMergesRoleMappings(t *testing.T) {
	root := t.TempDir()
	basePath := filepath.Join(root, "model-profiles.json")
	writeProfileFile(t, basePath, `{
  "activeProfile": "openai",
  "profiles": {
    "openai": {
      "pi": {
        "code-fast": "openai-codex/fast",
        "code-high": "openai-codex/high"
      },
      "opencode": {
        "code-fast": "openai/fast"
      }
    }
  },
  "fixed": {
    "codex": {
      "code-fast": "fast"
    }
  }
}`)
	writeProfileFile(t, filepath.Join(root, "model-overrides", "work.json"), `{
  "profiles": {
    "openai": {
      "pi": {
        "code-fast": "beta-openai/luna"
      }
    }
  },
  "fixed": {
    "codex": {
      "code-high": "high"
    }
  }
}`)

	got, err := LoadWithEnvironment(basePath, "openai", "work")
	if err != nil {
		t.Fatal(err)
	}
	assertModel(t, got, "pi", "code-fast", "beta-openai/luna")
	assertModel(t, got, "pi", "code-high", "openai-codex/high")
	assertModel(t, got, "opencode", "code-fast", "openai/fast")
	assertModel(t, got, "codex", "code-fast", "fast")
	assertModel(t, got, "codex", "code-high", "high")
	if got.Environment != "work" {
		t.Fatalf("environment = %q, want work", got.Environment)
	}
}

func TestLoadWithEnvironmentAllowsMissingOverride(t *testing.T) {
	root := t.TempDir()
	basePath := filepath.Join(root, "model-profiles.json")
	writeProfileFile(t, basePath, `{
  "activeProfile": "openai",
  "profiles": {"openai": {"pi": {"code-fast": "base/fast"}}},
  "fixed": {}
}`)

	got, err := LoadWithEnvironment(basePath, "openai", "home")
	if err != nil {
		t.Fatal(err)
	}
	assertModel(t, got, "pi", "code-fast", "base/fast")
}

func TestLoadWithEnvironmentRejectsInvalidOverlay(t *testing.T) {
	root := t.TempDir()
	basePath := filepath.Join(root, "model-profiles.json")
	writeProfileFile(t, basePath, `{
  "activeProfile": "openai",
  "profiles": {"openai": {"pi": {"code-fast": "base/fast"}}},
  "fixed": {}
}`)
	writeProfileFile(t, filepath.Join(root, "model-overrides", "work.json"), `{not-json}`)

	if _, err := LoadWithEnvironment(basePath, "openai", "work"); err == nil {
		t.Fatal("expected invalid environment override to fail")
	}
}

func TestResolveSortsFixedTargetsInErrors(t *testing.T) {
	profile := &File{Fixed: map[string]map[string]string{
		"zeta":  {"review": "zeta-review"},
		"alpha": {"review": "alpha-review"},
	}}
	_, err := profile.Resolve("missing", "review")
	if err == nil {
		t.Fatal("expected missing target error")
	}
	if !strings.Contains(err.Error(), "fixed targets [alpha zeta]") {
		t.Fatalf("fixed targets are not sorted in error: %v", err)
	}
}

func assertModel(t *testing.T, f *File, target, role, want string) {
	t.Helper()
	got, err := f.Resolve(target, role)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Resolve(%q, %q) = %q, want %q", target, role, got, want)
	}
}

func writeProfileFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
