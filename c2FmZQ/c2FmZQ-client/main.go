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

// The c2FmZQ-client binary can securely encrypt, store, and share files,
// including but not limited to pictures and videos.
//
// It is API-compatible with the Stingle Photos app (https://github.com/stingle/stingle-photos-android)
// published by stingle.org.
package main

import (
	"os"

	"c2FmZQ/c2FmZQ-client/internal"
	"c2FmZQ/internal/log"
)

func main() {
	app := internal.New()
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
