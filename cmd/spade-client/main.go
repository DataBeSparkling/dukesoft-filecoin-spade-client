package main

import (
	"context"
	"filecoin-spade-client/pkg/build"
	"filecoin-spade-client/pkg/client"
	"filecoin-spade-client/pkg/config"
	"filecoin-spade-client/pkg/log"
	"fmt"
	"github.com/urfave/cli/v2"
	"os"
	"os/signal"
	"syscall"
)

func main() {

	cli.VersionFlag = &cli.BoolFlag{
		Name:    "version",
		Aliases: []string{"v"},
		Usage:   "show version information",
	}

	cli.VersionPrinter = func(cCtx *cli.Context) {
		fmt.Printf("%s-%s (%s)", build.VERSION, build.BUILD, build.COMMIT)
	}

	log.StartLogger(false)

	app := &cli.App{
		Usage:       "Filecoin Spade Client",
		Name:        "spade-client",
		Description: "A client for Filecoin's Spade service",
		Version:     build.VERSION,

		Commands: []*cli.Command{
			{
				Name:    "run",
				Aliases: []string{"r"},
				Usage:   "Runs DukeSoft's Spade Client for Lotus",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "download-path",
						Value: "/tmp/filecoin-spade-downloads",
						Usage: "The location where the downloaded files should reside",
					},
					&cli.IntFlag{
						Name:  "max-spade-deals-active",
						Value: 2,
						Usage: "Total number of spade deals that should be actively downloading / requesting (This doesn't include other deals or sealing!)",
					},
					&cli.IntFlag{
						Name:  "boost-graphql-port",
						Value: 8080,
						Usage: "Boost's GraphQL port",
					},
				},
				Action: func(cCtx *cli.Context) error {
					cfg := config.NewDefaultConfiguration()
					cfg.DownloadPath = cCtx.String("download-path")
					cfg.MaxSpadeDealsActive = cCtx.Int("max-spade-deals-active")
					cfg.BoostConfig.GraphQlPort = cCtx.Int("boost-graphql-port")

					startClient(cCtx.Context, cfg)
					return nil
				},
			},
		},
	}

	ctx, cancelCtx := context.WithCancel(context.Background())

	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\r Caught termination signal")
		cancelCtx()
	}()

	if err := app.RunContext(ctx, os.Args); err != nil {
		log.Fatal(err)
	}
}

func startClient(ctx context.Context, config config.Configuration) {
	printVersion()
	log.Infof("Config: %+v", config)

	var app = client.New(config)

	// Start/verify the backend.
	err := app.Start(ctx)
	if err != nil {
		log.Warnf("Backend error: %v", err)
	}

	log.Fatalf("Stopping program")
}

func printVersion() {
	log.StartLogger(false)
	log.Infof("DukeSoft's Filecoin Spade Client %s-%s (%s)", build.VERSION, build.BUILD, build.COMMIT)
}
