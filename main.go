package main

import (
	"log"
	"os"

	"github.com/urfave/cli/v2"
)

func main() {
	app := &cli.App{
		Name:  "dumbfilstore",
		Usage: "Storage layer service",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "listen",
				Aliases: []string{"l"},
				Usage:   "[host]:port to listen on",
				Value:   "127.0.0.1:9091",
			},
			&cli.StringFlag{
				Name:  "api",
				Usage: "read only backing API for chain calls",
				Value: "/dns/api.chain.love/wss",
			},
			&cli.StringFlag{
				Name:  "wallet",
				Usage: "backing API for wallet calls - or internal to maintain a local keypair",
				Value: "internal",
			},
			&cli.StringFlag{
				Name:  "root",
				Usage: "where to store data",
				Value: "./.filstore",
			},
		},
		Action: Serve,
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}
