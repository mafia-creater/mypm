package cmd

import (
	"fmt"
	"os"
)

// ─── RUN ───────────────────────────────────────────────────────────────────────

func runRun(args []string) {
	fmt.Println("  mypm run — Execute a script from package.json")
	os.Exit(0)
}

// ─── START ─────────────────────────────────────────────────────────────────────

func runStart(args []string) {
	fmt.Println("  mypm start — Start the application")
	os.Exit(0)
}

// ─── TEST ──────────────────────────────────────────────────────────────────────

func runTest(args []string) {
	fmt.Println("  mypm test — Run tests")
	os.Exit(0)
}

// ─── BUILD ─────────────────────────────────────────────────────────────────────

func runBuild(args []string) {
	fmt.Println("  mypm build — Build the application")
	os.Exit(0)
}
