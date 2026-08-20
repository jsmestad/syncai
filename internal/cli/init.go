package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jsmestad/syncai/internal/config"
	"github.com/jsmestad/syncai/internal/load"
	"github.com/jsmestad/syncai/internal/profiles"
	"github.com/jsmestad/syncai/internal/renderers"
	"github.com/spf13/cobra"
)

var starterFiles = map[string]string{
	"agents/worker.md": `---
name: worker
description: Handles focused implementation and validation tasks.
targets: pi, omp, claude, codex, opencode, antigravity
tools: read, bash, edit, write, grep, find
modelRole: code-medium
---

Implement the requested change, update tests when behavior changes, and report the validation you ran.
`,
	"instructions/global.md": `# Shared agent instructions

Make the smallest coherent change that satisfies the request. Preserve unrelated user work and report concrete validation evidence.
`,
	"model-profiles.json": `{
  "activeProfile": "openai",
  "profiles": {
    "openai": {
      "pi": {
        "code-medium": "openai-codex/gpt-5.6-sol:medium"
      },
      "omp": {
        "code-medium": "openai-codex/gpt-5.6-sol:medium"
      },
      "opencode": {
        "code-medium": "openai/gpt-5.6-terra"
      }
    }
  },
  "fixed": {
    "claude": {
      "code-medium": "sonnet"
    },
    "codex": {
      "code-medium": "gpt-5.6-sol:medium"
    },
    "antigravity": {
      "code-medium": "gemini-2.5-pro"
    }
  }
}
`,
	"packages.json": `{
  "pi": {
    "packages": [],
    "packagesByScope": {},
    "npmCommand": [],
    "npmCommandByScope": {}
  },
  "claude": {
    "marketplaces": [],
    "plugins": []
  },
  "codex": {
    "plugins": []
  },
  "antigravity": {
    "plugins": []
  }
}
`,
}

func (a *App) initCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "init [source-dir]",
		Short: "Create or register the canonical source used by every command",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runInit(cmd.Context(), a.renderers, cmd.OutOrStdout(), args)
		},
	}
}

func runInit(ctx context.Context, available []renderers.Renderer, outWriter io.Writer, args []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	var source string
	var err error
	if len(args) == 1 {
		source, err = filepath.Abs(args[0])
	} else {
		source, err = config.DefaultSource()
	}
	if err != nil {
		return fmt.Errorf("resolving source path: %w", err)
	}

	created, err := ensureSource(source)
	if err != nil {
		return err
	}
	if err := validateInitSource(ctx, available, source); err != nil {
		return fmt.Errorf("source %s is not renderable: %w", source, err)
	}
	configPath, err := config.SaveSource(source)
	if err != nil {
		return err
	}

	action := "registered existing source"
	if created {
		action = "created starter source"
	}
	fmt.Fprintf(outWriter, "%s at %s\n", action, source)
	fmt.Fprintf(outWriter, "saved default source in %s\n\n", configPath)
	fmt.Fprintln(outWriter, "Next:")
	fmt.Fprintln(outWriter, "  syncai validate")
	fmt.Fprintln(outWriter, "  syncai render --out \"$(mktemp -d)\"")
	return nil
}

func ensureSource(source string) (bool, error) {
	info, err := os.Stat(source)
	if err == nil {
		if !info.IsDir() {
			return false, fmt.Errorf("source path %s is not a directory", source)
		}
		entries, err := os.ReadDir(source)
		if err != nil {
			return false, fmt.Errorf("reading source directory %s: %w", source, err)
		}
		if len(entries) > 0 {
			return false, nil
		}
	} else if !os.IsNotExist(err) {
		return false, fmt.Errorf("inspecting source path %s: %w", source, err)
	} else if err := os.MkdirAll(source, 0o755); err != nil {
		return false, fmt.Errorf("creating source directory %s: %w", source, err)
	}

	for relative, body := range starterFiles {
		if err := load.WriteFileReplacing(source, filepath.Join(source, relative), []byte(body), 0o644); err != nil {
			return false, fmt.Errorf("creating starter file %s: %w", relative, err)
		}
	}
	return true, nil
}

func validateInitSource(ctx context.Context, available []renderers.Renderer, source string) error {
	base, err := profiles.Load(filepath.Join(source, "model-profiles.json"))
	if err != nil {
		return err
	}
	in, err := loadInputs(source, base.ActiveProfile, false, "")
	if err != nil {
		return err
	}
	preview, err := os.MkdirTemp("", "syncai-init-preview-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(preview)
	for _, renderer := range available {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := renderer.Render(in, preview); err != nil {
			return fmt.Errorf("%s: %w", renderer.Name(), err)
		}
	}
	return nil
}
