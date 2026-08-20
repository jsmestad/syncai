package cli

import (
	"context"
	"io"

	"github.com/jsmestad/syncai/internal/renderers"
	"github.com/jsmestad/syncai/internal/renderers/antigravity"
	"github.com/jsmestad/syncai/internal/renderers/claude"
	"github.com/jsmestad/syncai/internal/renderers/codex"
	"github.com/jsmestad/syncai/internal/renderers/omp"
	"github.com/jsmestad/syncai/internal/renderers/opencode"
	"github.com/jsmestad/syncai/internal/renderers/pi"
	"github.com/spf13/cobra"
)

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
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetIn(a.streams.In)
	root.SetOut(a.streams.Out)
	root.SetErr(a.streams.Err)
	root.AddCommand(
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
