package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/liamg/happen/feed"
	"github.com/liamg/happen/gui"
)

func main() {

	var ignoreConfig bool
	flag.BoolVar(&ignoreConfig, "i", false, "Ignore config file and use defaults")
	flag.Parse()

	conf, err := feed.LoadConfig(!ignoreConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	g, err := gui.Create(conf)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Startup error: %v\n", err)
		os.Exit(1)
	}
	defer g.Close()
	g.Run(context.Background())
}
