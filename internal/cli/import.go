package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jsmestad/syncai/internal/importer"
	"github.com/jsmestad/syncai/internal/profiles"
	"github.com/spf13/cobra"
)

type importOptions struct {
	source string
	all    bool
}

func importCommand() *cobra.Command {
	var options importOptions
	command := &cobra.Command{
		Use:   "import [name...]",
		Short: "Find installed agents not tracked in ai-source/ and port them into source",
		Long: `Scan installed agent directories ($HOME/.pi, $HOME/.claude, etc.) for agent
files not represented in ai-source/agents/ and either list them or port them
into source. With no args and no --all, prints the list of candidates and
exits. With --all, ports every Pi-format candidate (other tools require manual
port — their formats convert lossily). Specific names can be passed as args.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImport(cmd.Context(), cmd.OutOrStdout(), options, args)
		},
	}
	command.Flags().StringVar(&options.source, "source", "ai-source", "canonical source directory")
	command.Flags().BoolVar(&options.all, "all", false, "port every auto-portable candidate (currently Pi only)")
	return command
}

func runImport(ctx context.Context, outWriter io.Writer, options importOptions, names []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	sourceRoot, err := filepath.Abs(options.source)
	if err != nil {
		return fmt.Errorf("resolving source root %s: %w", options.source, err)
	}
	candidates, err := importer.Scan(home, sourceRoot)
	if err != nil {
		return err
	}
	extensionCandidates, err := importer.ScanExtensions(home, sourceRoot)
	if err != nil {
		return err
	}
	extensionDirectories, err := importer.ScanExtensionDirectories(home, sourceRoot)
	if err != nil {
		return err
	}
	skillCandidates, err := importer.ScanSkills(home, sourceRoot)
	if err != nil {
		return err
	}

	if len(candidates) == 0 && len(extensionCandidates) == 0 && len(extensionDirectories) == 0 && len(skillCandidates) == 0 {
		fmt.Fprintln(outWriter, "ok: every installed agent, skill, and extension is tracked in ai-source/")
		return nil
	}

	if !options.all && len(names) == 0 {
		printImportCandidates(outWriter, candidates, extensionCandidates, skillCandidates)
		printDirectoryGuidance(outWriter, extensionDirectories)
		if len(candidates) > 0 || len(extensionCandidates) > 0 || len(skillCandidates) > 0 {
			fmt.Fprintln(outWriter, "Run `syncai import --all` to port every auto-portable candidate,")
			fmt.Fprintln(outWriter, "or `syncai import <name>` to port specific ones by name.")
		}
		return nil
	}

	profile, err := profiles.LoadWithProfile(filepath.Join(sourceRoot, "model-profiles.json"), "")
	if err != nil {
		return err
	}
	selected := selectImports(candidates, options.all, names)
	selectedExtensions := selectExtensionImports(extensionCandidates, options.all, names)
	selectedSkills := selectSkillImports(skillCandidates, options.all, names)
	if len(selected) == 0 && len(selectedExtensions) == 0 && len(selectedSkills) == 0 {
		fmt.Fprintln(outWriter, "no candidates matched the requested names")
		return nil
	}
	for _, candidate := range selected {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !candidate.AutoPortable {
			fmt.Fprintf(outWriter, "skip: %s (from %s) — manual port required\n", candidate.Name, candidate.Tool)
			continue
		}
		if err := importer.Port(sourceRoot, candidate, profile); err != nil {
			return fmt.Errorf("porting %s: %w", candidate.Name, err)
		}
		fmt.Fprintf(outWriter, "imported agent: %s ← %s\n  → %s\n", candidate.Name, candidate.InputPath, candidate.SourcePath)
	}
	for _, candidate := range selectedExtensions {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := importer.PortExtension(sourceRoot, candidate); err != nil {
			return fmt.Errorf("porting extension %s: %w", candidate.Name, err)
		}
		fmt.Fprintf(outWriter, "imported extension: %s ← %s\n  → %s\n", candidate.Name, candidate.InputPath, candidate.SourcePath)
		fmt.Fprintf(outWriter, "  (no sidecar written; default scope = universal. Add ai-source/extensions/%s.toml or %s/extension.toml to scope to home/work.)\n", candidate.Name, candidate.Name)
	}
	for _, candidate := range selectedSkills {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := importer.PortSkill(sourceRoot, candidate); err != nil {
			return fmt.Errorf("porting skill %s: %w", candidate.Name, err)
		}
		fmt.Fprintf(outWriter, "imported skill: %s ← %s [from %s]\n  → %s\n", candidate.Name, candidate.InputPath, candidate.Tool, candidate.SourcePath)
	}
	fmt.Fprintln(outWriter)
	fmt.Fprintln(outWriter, "Review the new source files, edit as needed, then run `make ai-sync PROFILE=<home|work>`.")
	return nil
}

func printImportCandidates(outWriter io.Writer, candidates []importer.Candidate, extensions []importer.ExtensionCandidate, skills []importer.SkillCandidate) {
	if len(candidates) > 0 {
		fmt.Fprintf(outWriter, "Found %d installed agent(s) not in ai-source/agents/:\n\n", len(candidates))
		for _, candidate := range candidates {
			marker := "  (manual port — auto-import only supports Pi, Claude, Codex)"
			if candidate.AutoPortable {
				marker = "  (auto-portable)"
			}
			fmt.Fprintf(outWriter, "  %s [from %s]\n%s\n    source: %s\n    target: %s\n\n", candidate.Name, candidate.Tool, marker, candidate.InputPath, candidate.SourcePath)
		}
	}
	if len(extensions) > 0 {
		fmt.Fprintf(outWriter, "Found %d installed extension(s) not in ai-source/extensions/:\n\n", len(extensions))
		for _, candidate := range extensions {
			fmt.Fprintf(outWriter, "  %s [extension]\n  (auto-portable, scope=universal by default)\n    source: %s\n    target: %s\n\n", candidate.Name, candidate.InputPath, candidate.SourcePath)
		}
	}
	if len(skills) > 0 {
		fmt.Fprintf(outWriter, "Found %d installed skill(s) not in ai-source/skills/:\n\n", len(skills))
		for _, candidate := range skills {
			fmt.Fprintf(outWriter, "  %s [from %s]\n  (auto-portable, scope=universal by default)\n    source: %s\n    target: %s\n\n", candidate.Name, candidate.Tool, candidate.InputPath, candidate.SourcePath)
		}
	}
}

func selectSkillImports(candidates []importer.SkillCandidate, all bool, names []string) []importer.SkillCandidate {
	wanted := map[string]bool{}
	for _, name := range names {
		wanted[name] = true
	}
	selected := []importer.SkillCandidate{}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if seen[candidate.Name] || (!all && !wanted[candidate.Name]) {
			continue
		}
		seen[candidate.Name] = true
		selected = append(selected, candidate)
	}
	return selected
}

func printDirectoryGuidance(outWriter io.Writer, directories []importer.ExtensionDirectory) {
	if len(directories) == 0 {
		return
	}
	groups := map[string][]importer.ExtensionDirectory{}
	for _, directory := range directories {
		groups[directory.Hint()] = append(groups[directory.Hint()], directory)
	}
	fmt.Fprintln(outWriter, "Directory entries under ~/.pi/agent/extensions/ not in source (manual vendoring only):")
	fmt.Fprintln(outWriter)
	order := []string{"prototype", "check", "external"}
	labels := map[string]string{
		"prototype": "likely user-managed (no package.json):",
		"check":     "ambiguous (package.json without a name field) — inspect before vendoring:",
		"external":  "likely external pi extensions (have package.json with a name) — probably skip:",
	}
	for _, hint := range order {
		group := groups[hint]
		if len(group) == 0 {
			continue
		}
		fmt.Fprintf(outWriter, "  %s\n", labels[hint])
		for _, directory := range group {
			detail := ""
			if directory.PackageName != "" {
				detail = fmt.Sprintf(" (name: %s)", directory.PackageName)
			}
			fmt.Fprintf(outWriter, "    - %s%s\n", directory.Name, detail)
		}
		fmt.Fprintln(outWriter)
	}
	example := directories[0].Name
	for _, hint := range []string{"prototype", "check"} {
		if group := groups[hint]; len(group) > 0 {
			example = group[0].Name
			break
		}
	}
	fmt.Fprintln(outWriter, "To vendor a directory extension into source:")
	fmt.Fprintf(outWriter, "  cp -R ~/.pi/agent/extensions/%s ai-source/extensions/\n", example)
	fmt.Fprintf(outWriter, "  printf 'scope = \"work\"\\n' > ai-source/extensions/%s/extension.toml\n", example)
	fmt.Fprintln(outWriter, "  syncai render --scope work")
	fmt.Fprintln(outWriter)
	fmt.Fprintln(outWriter, "After the first render, syncai owns the install path. Edits in")
	fmt.Fprintln(outWriter, "either tree are visible to `syncai status` / `syncai pull`.")
	fmt.Fprintln(outWriter)
}

func selectExtensionImports(candidates []importer.ExtensionCandidate, all bool, names []string) []importer.ExtensionCandidate {
	if all {
		return candidates
	}
	wanted := map[string]bool{}
	for _, name := range names {
		wanted[name] = true
	}
	selected := []importer.ExtensionCandidate{}
	for _, candidate := range candidates {
		if wanted[candidate.Name] {
			selected = append(selected, candidate)
		}
	}
	return selected
}

func selectImports(candidates []importer.Candidate, all bool, names []string) []importer.Candidate {
	if all {
		selected := []importer.Candidate{}
		seen := map[string]bool{}
		for _, candidate := range candidates {
			if candidate.Tool == "pi" && !seen[candidate.Name] {
				selected = append(selected, candidate)
				seen[candidate.Name] = true
			}
		}
		for _, candidate := range candidates {
			if !seen[candidate.Name] {
				selected = append(selected, candidate)
				seen[candidate.Name] = true
			}
		}
		return selected
	}
	wanted := map[string]bool{}
	for _, name := range names {
		wanted[name] = true
	}
	selected := []importer.Candidate{}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		if wanted[candidate.Name] && !seen[candidate.Name] {
			selected = append(selected, candidate)
			seen[candidate.Name] = true
		}
	}
	return selected
}
