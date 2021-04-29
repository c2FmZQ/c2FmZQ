// The kringle-client binary can securely encrypt, store, and share files,
// including but not limited to pictures and videos.
//
// It is API-compatible with the Stingle Photos app (https://github.com/stingle/stingle-photos-android)
// published by stingle.org.
package main

import (
	"os"

	"kringle/internal/log"
	"kringle/kringle-client/internal"
)

func main() {
	app := internal.New()
	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
