package cmd

import (
	"flag"
	"fmt"
	"os"
)

// Version of mypm
const Version = "0.1.0"

// Execute is the main entrypoint called from main.go
func Execute() {
	flag.Usage = usage

	if len(os.Args) < 2 {
		usage()
		os.Exit(0)
	}

	subcommand := os.Args[1]

	switch subcommand {
	case "install", "i":
		runInstall(os.Args[2:])
	case "add":
		runAdd(os.Args[2:])
	case "remove", "rm":
		runRemove(os.Args[2:])
	case "store":
		runStore(os.Args[2:])
	case "doctor":
		runDoctor(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Printf("mypm v%s\n", Version)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "  unknown command: %q\n\n", subcommand)
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Printf(`
  mypm v%s — content-addressable Node.js package manager

  USAGE
    mypm <command> [flags]

  COMMANDS
    install              Install all dependencies from package.json
    add <pkg>[@ver]      Add a new dependency
    remove <pkg>         Remove a dependency
    store path           Print the global store path
    store status         Show store stats and disk savings
    doctor               Check store health and link integrity
    version              Print mypm version

  FLAGS
    --frozen-lockfile    Fail if lockfile is missing or out of sync (install only)
    --link-type          Override link strategy: hardlink | symlink | copy
    --store-dir          Override store directory path

  EXAMPLES
    mypm install
    mypm add react@18
    mypm add -D typescript
    mypm remove lodash
    mypm store status
    mypm doctor

  STORE
    Default: ~/.mypm/store
    Override in ~/.mypmrc:
      store-dir  = /custom/path
      link-type  = hardlink
      concurrency = 8

`, Version)
}
