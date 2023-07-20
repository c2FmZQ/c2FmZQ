//
// Copyright 2021-2022 TTBT Enterprises LLC
//
// This file is part of c2FmZQ (https://c2FmZQ.org/).
//
// c2FmZQ is free software: you can redistribute it and/or modify it under the
// terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later
// version.
//
// c2FmZQ is distributed in the hope that it will be useful, but WITHOUT ANY
// WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR
// A PARTICULAR PURPOSE. See the GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License along with
// c2FmZQ. If not, see <https://www.gnu.org/licenses/>.

//go:build !windows
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
