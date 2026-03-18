package main

import (
	"fmt"
	"os"

	"github.com/alecthomas/kong"
)

type CLI struct {
	ConfigFolderPath string `arg:"" name:"configFolder" help:"Path to config folder containing dev.yaml and queries/"`
	DBStatus         bool   `name:"db-status" help:"Ping the configured database and exit"`
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

	gj, db, err := NewGraphJinFromDevConfig(cfg.graphjinDev)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	if cli.DBStatus {
		if err := db.Ping(); err != nil {
			fmt.Fprintln(os.Stderr, "db ping failed:", err)
			os.Exit(1)
		}
		fmt.Println("db ping: OK")
		return
	}

	if err := Compile(cfg, gj); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
