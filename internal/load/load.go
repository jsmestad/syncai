package load

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jsmestad/syncai/internal/schema"
)

// Agents reads every *.md file under dir as an Agent.
// Returns agents sorted by name for deterministic output.
func Agents(dir string) ([]*schema.Agent, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", dir, err)
	}
	var agents []*schema.Agent
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		// Chains have a different schema and are loaded elsewhere.
		if strings.HasSuffix(e.Name(), ".chain.md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", path, err)
		}
		a, err := schema.ParseAgent(path, raw)
		if err != nil {
			return nil, err
		}
		agents = append(agents, a)
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].Name < agents[j].Name })
	return agents, nil
}
