// Package omp renders canonical agents into Oh My Pi's native task-agent layout.
//
// Global mode writes <outRoot>/.omp/agent/agents/<name>.md.
// Project mode writes <outRoot>/.omp/agents/<name>.md.
package omp

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jsmestad/syncai/internal/load"
	"github.com/jsmestad/syncai/internal/profiles"
	"github.com/jsmestad/syncai/internal/renderers"
	"github.com/jsmestad/syncai/internal/schema"
)

type Renderer struct{}

func New() Renderer { return Renderer{} }

func (Renderer) Name() string { return string(schema.TargetOMP) }

func (Renderer) Render(in renderers.Inputs, outRoot string) ([]string, error) {
	dir := filepath.Join(outRoot, ".omp", "agent", "agents")
	if in.ProjectMode {
		dir = filepath.Join(outRoot, ".omp", "agents")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}

	var written []string
	for _, agent := range in.Agents {
		if !agent.HasTarget(schema.TargetOMP) || strings.HasSuffix(agent.Path, ".chain.md") {
			continue
		}
		body, err := renderAgent(agent, in.Profiles)
		if err != nil {
			return nil, err
		}
		path := filepath.Join(dir, agent.Name+".md")
		if err := load.WriteFileReplacing(path, body, 0o644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", path, err)
		}
		written = append(written, path)
	}
	return written, nil
}

func renderAgent(agent *schema.Agent, profile *profiles.File) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString("---\n")
	fmt.Fprintf(&b, "name: %s\n", agent.Name)
	fmt.Fprintf(&b, "description: %s\n", strconv.Quote(agent.Description))

	if tools := ompTools(agent.Lookup("tools")); len(tools) > 0 {
		fmt.Fprintf(&b, "tools: %s\n", strings.Join(tools, ", "))
	}
	if inheritSkills := agent.Lookup("inheritSkills"); inheritSkills != "" {
		fmt.Fprintf(&b, "autoloadSkills: %s\n", inheritSkills)
	}
	if role := agent.Lookup("modelRole"); role != "" && profile != nil {
		resolved, err := profile.Resolve(string(schema.TargetOMP), role, schema.SplitCSV(agent.Lookup("fallbackRoles"))...)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", agent.Path, err)
		}
		model, thinking := splitModelThinking(resolved)
		fmt.Fprintf(&b, "model: %s\n", model)
		if thinking != "" {
			fmt.Fprintf(&b, "thinkingLevel: %s\n", thinking)
		}
	}
	if spawns := agent.Lookup("ompSpawns"); spawns != "" {
		fmt.Fprintf(&b, "spawns: %s\n", spawns)
	} else if allowsNestedDelegation(agent) {
		b.WriteString("spawns: *\n")
	}
	b.WriteString("---\n")
	b.WriteString(agent.Body)
	return b.Bytes(), nil
}

func ompTools(value string) []string {
	out := make([]string, 0)
	seen := map[string]bool{}
	for _, tool := range schema.SplitCSV(value) {
		mapped := strings.ToLower(tool)
		switch mapped {
		case "find", "ls":
			mapped = "glob"
		case "agent":
			mapped = "task"
		case "get_subagent_result", "steer_subagent":
			mapped = "hub"
		}
		if !seen[mapped] {
			seen[mapped] = true
			out = append(out, mapped)
		}
	}
	return out
}

func allowsNestedDelegation(agent *schema.Agent) bool {
	depth, err := strconv.Atoi(strings.TrimSpace(agent.Lookup("maxSubagentDepth")))
	return err == nil && depth > 1
}

func splitModelThinking(value string) (model, thinking string) {
	colon := strings.LastIndex(value, ":")
	if colon < 0 {
		return value, ""
	}
	candidate := value[colon+1:]
	switch candidate {
	case "off", "minimal", "low", "medium", "high", "xhigh", "max", "auto":
		return value[:colon], candidate
	default:
		return value, ""
	}
}
