package main

import (
	"context"
	"fmt"
	"os"

	"github.com/liamg/happen/feed"
	"github.com/liamg/happen/gui"
)

// This program just prints "Hello, World!".  Press ESC to exit.
func main() {

	conf, err := feed.LoadConfig()
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
