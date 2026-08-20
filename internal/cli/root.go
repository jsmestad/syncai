package cli

import (
	"context"
	"io"
	"runtime/debug"
	"strings"

	"github.com/jsmestad/syncai/internal/renderers"
	"github.com/jsmestad/syncai/internal/renderers/antigravity"
	"github.com/jsmestad/syncai/internal/renderers/claude"
	"github.com/jsmestad/syncai/internal/renderers/codex"
	"github.com/jsmestad/syncai/internal/renderers/omp"
	"github.com/jsmestad/syncai/internal/renderers/opencode"
	"github.com/jsmestad/syncai/internal/renderers/pi"
	"github.com/spf13/cobra"
)

var version = "dev"

func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	return resolveVersion(version, info, ok)
}

func resolveVersion(linkerVersion string, info *debug.BuildInfo, ok bool) string {
	if linkerVersion != "dev" {
		return strings.TrimPrefix(linkerVersion, "v")
	}
	if !ok || info == nil || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return linkerVersion
	}
	return strings.TrimPrefix(info.Main.Version, "v")
}

type Streams struct {
	In  io.Reader
	Out io.Writer
	Err io.Writer
}

type App struct {
	streams   Streams
	renderers []renderers.Renderer
}

func New(streams Streams) *App {
	return &App{
		streams: streams,
		renderers: []renderers.Renderer{
			pi.New(),
			omp.New(),
			claude.New(),
			codex.New(),
			opencode.New(),
			antigravity.New(),
		},
	}
}

func (a *App) Root() *cobra.Command {
	root := &cobra.Command{
		Use:           "syncai",
		Short:         "Render canonical AI configs directly into ~/.pi, ~/.omp, ~/.claude, ~/.codex, ~/.config/opencode, and Antigravity CLI",
		Version:       buildVersion(),
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetIn(a.streams.In)
	root.SetOut(a.streams.Out)
	root.SetErr(a.streams.Err)
	root.AddCommand(
		guideCommand(),
		updateCommand(),
		a.initCommand(),
		a.renderCommand(),
		validateCommand(),
		setProfileCommand(),
		a.useProfileCommand(),
		importCommand(),
		a.statusCommand(),
		a.pullCommand(),
		listCommand(),
		packagesCommand(),
	)
	return root
}

func (a *App) Execute(ctx context.Context, args []string) error {
	root := a.Root()
	root.SetArgs(args)
	return root.ExecuteContext(ctx)
}
