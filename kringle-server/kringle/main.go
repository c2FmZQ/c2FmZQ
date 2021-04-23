package main

import (
	"os"

	"kringle-server/log"
)

func main() {
	app := makeKringle()
	if err := app.cli.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
