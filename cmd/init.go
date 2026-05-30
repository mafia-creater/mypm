package cmd

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// fullPackageJSON is the complete structure we write out
type fullPackageJSON struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Main        string            `json:"main"`
	Scripts     map[string]string `json:"scripts"`
	Keywords    []string          `json:"keywords"`
	Author      string            `json:"author"`
	License     string            `json:"license"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

func runInit(args []string) {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	yes := fs.Bool("y", false, "skip prompts and use defaults")
	fs.BoolVar(yes, "yes", false, "skip prompts and use defaults")
	fs.Parse(args)

	logger_banner()

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ✗ cannot get current directory: %v\n", err)
		os.Exit(1)
	}

	pkgPath := filepath.Join(cwd, "package.json")

	// Warn if package.json already exists
	if _, err := os.Stat(pkgPath); err == nil {
		fmt.Print("  package.json already exists. Overwrite? (y/N): ")
		if !*yes {
			var answer string
			fmt.Scanln(&answer)
			if strings.ToLower(strings.TrimSpace(answer)) != "y" {
				fmt.Println("  aborted.")
				return
			}
		} else {
			fmt.Println("y (--yes)")
		}
	}

	// Defaults derived from the current directory name
	dirName := filepath.Base(cwd)
	defaults := fullPackageJSON{
		Name:        sanitizeName(dirName),
		Version:     "1.0.0",
		Description: "",
		Main:        "index.js",
		Scripts:     map[string]string{"test": "echo \"Error: no test specified\" && exit 1"},
		Keywords:    []string{},
		Author:      "",
		License:     "ISC",
		Dependencies:    map[string]string{},
		DevDependencies: map[string]string{},
	}

	var pkg fullPackageJSON

	if *yes {
		pkg = defaults
		printDefaults(defaults)
	} else {
		pkg = promptUser(defaults)
	}

	// Serialize
	data, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "  ✗ failed to serialize: %v\n", err)
		os.Exit(1)
	}

	// Atomic write
	tmp := pkgPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "  ✗ failed to write: %v\n", err)
		os.Exit(1)
	}
	if err := os.Rename(tmp, pkgPath); err != nil {
		fmt.Fprintf(os.Stderr, "  ✗ failed to save: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Printf("\033[32m  ✓ \033[0mwrote %s\n", pkgPath)
	fmt.Println()
	fmt.Println("  next steps:")
	fmt.Println("    mypm add <package>     add a dependency")
	fmt.Println("    mypm install           install all dependencies")
	fmt.Println()
}

// promptUser interactively asks the user for each field, showing the default in brackets
func promptUser(d fullPackageJSON) fullPackageJSON {
	r := bufio.NewReader(os.Stdin)

	fmt.Println("  This utility will walk you through creating a package.json file.")
	fmt.Println("  Press ^C at any time to quit. Hit enter to use the default value.")
	fmt.Println()

	pkg := d

	pkg.Name        = ask(r, "package name",  d.Name)
	pkg.Version     = ask(r, "version",        d.Version)
	pkg.Description = ask(r, "description",    d.Description)
	pkg.Main        = ask(r, "entry point",    d.Main)
	pkg.Author      = ask(r, "author",         d.Author)
	pkg.License     = ask(r, "license",        d.License)

	fmt.Println()

	// Preview
	preview, _ := json.MarshalIndent(pkg, "", "  ")
	fmt.Println("  About to write:")
	fmt.Println()
	for _, line := range strings.Split(string(preview), "\n") {
		fmt.Println("  " + line)
	}
	fmt.Println()
	fmt.Print("  Is this OK? (yes): ")
	input, _ := r.ReadString('\n')
	input = strings.TrimSpace(input)
	if input != "" && strings.ToLower(input) != "yes" && strings.ToLower(input) != "y" {
		fmt.Println("  aborted.")
		os.Exit(0)
	}

	return pkg
}

// ask prints a prompt with the default value and reads user input
func ask(r *bufio.Reader, label, defaultVal string) string {
	if defaultVal != "" {
		fmt.Printf("  \033[90m%-18s\033[0m (\033[36m%s\033[0m): ", label, defaultVal)
	} else {
		fmt.Printf("  \033[90m%-18s\033[0m: ", label)
	}

	input, err := r.ReadString('\n')
	if err != nil {
		return defaultVal
	}
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultVal
	}
	return input
}

// printDefaults shows what will be written when --yes is used
func printDefaults(pkg fullPackageJSON) {
	data, _ := json.MarshalIndent(pkg, "", "  ")
	fmt.Println("  Using defaults:")
	fmt.Println()
	for _, line := range strings.Split(string(data), "\n") {
		fmt.Println("  " + line)
	}
}

// sanitizeName converts a directory name to a valid npm package name
// e.g. "My Project" → "my-project", "MyApp_v2" → "myapp_v2"
func sanitizeName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, " ", "-")
	// Remove any char that's not alphanumeric, hyphen, underscore, or dot
	var b strings.Builder
	for _, c := range name {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' || c == '.' {
			b.WriteRune(c)
		}
	}
	return b.String()
}

func logger_banner() {
	fmt.Printf("\n\033[1m\033[36m  mypm\033[0m \033[90mv%s\033[0m\n\n", Version)
}
