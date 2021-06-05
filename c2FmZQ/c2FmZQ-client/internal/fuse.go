// +build !windows

package internal

import (
	"github.com/urfave/cli/v2" // cli

	"c2FmZQ/internal/client/fuse"
)

func init() {
	enableFuse = true
}

func (a *App) mount(ctx *cli.Context) error {
	if err := a.init(ctx, false); err != nil {
		return err
	}
	if ctx.Args().Len() != 1 {
		cli.ShowSubcommandHelp(ctx)
		return nil
	}
	return fuse.Mount(a.client, ctx.Args().Get(0), ctx.Bool("read-only"))
}
