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
