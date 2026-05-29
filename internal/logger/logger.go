package logger

import (
	"fmt"
	"os"
	"time"
)

// ANSI color codes - no external dep needed
const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	red    = "\033[31m"
	green  = "\033[32m"
	yellow = "\033[33m"
	blue   = "\033[34m"
	cyan   = "\033[36m"
	gray   = "\033[90m"
)

func Success(format string, args ...any) {
	fmt.Printf(green+"  ✓ "+reset+format+"\n", args...)
}

func Info(format string, args ...any) {
	fmt.Printf(blue+"  → "+reset+format+"\n", args...)
}

func Warn(format string, args ...any) {
	fmt.Printf(yellow+"  ⚠ "+reset+format+"\n", args...)
}

func Error(format string, args ...any) {
	fmt.Fprintf(os.Stderr, red+"  ✗ "+reset+format+"\n", args...)
}

func Fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, red+bold+"  ✗ FATAL: "+reset+format+"\n", args...)
	os.Exit(1)
}

func Step(step, total int, format string, args ...any) {
	prefix := fmt.Sprintf(gray+"[%d/%d]"+reset+" ", step, total)
	fmt.Printf(prefix+format+"\n", args...)
}

func Timing(label string, start time.Time) {
	elapsed := time.Since(start)
	fmt.Printf(gray+"  ⏱ %s: %s"+reset+"\n", label, elapsed.Round(time.Millisecond))
}

func Banner(name, version string) {
	fmt.Println()
	fmt.Printf(bold+cyan+"  mypm"+reset+" "+gray+"v%s"+reset+"\n", version)
	fmt.Println()
}

func Summary(installed, cached, skipped int, storeMB float64, elapsed time.Duration) {
	fmt.Println()
	fmt.Printf(bold+"  Done"+reset+" in %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf(gray+"  ├─"+reset+" %d packages installed\n", installed)
	fmt.Printf(gray+"  ├─"+reset+" %d from store cache (no download)\n", cached)
	fmt.Printf(gray+"  ├─"+reset+" %d already linked\n", skipped)
	fmt.Printf(gray+"  └─"+reset+" store size: %.1f MB\n", storeMB)
	fmt.Println()
}
