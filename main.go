package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
)

type CLI struct {
	ConfigFolderPath string `arg:"" name:"configFolder" help:"Path to config folder containing dev.yaml and queries/"`
}

func main() {
	var cli CLI
	_ = kong.Parse(&cli,
		kong.Name("nimbus-dsl-compile"),
		kong.Description("Reads GraphQL queries from config/queries and matching JSON data from the same folder."),
	)

	cfg, err := LoadConfig(cli.ConfigFolderPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := Compile(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
