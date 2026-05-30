# mypm

A content-addressable Node.js package manager that eliminates `node_modules` duplication using a global hard-link store.

## Install

```bash
git clone https://github.com/mafia-creater/mypm
cd mypm
go build -o mypm .
sudo mv mypm /usr/local/bin/   # or add to PATH
```

## Commands

```bash
mypm init             # Create a new package.json interactively
mypm create next@latest my-app  # Run create-* scaffolder (npx-style)
mypm dlx create-vite@latest my-vite-app # Download and run a package bin
mypm install          # Resolve + fetch + link deps from package.json
mypm add react@18     # Add a dependency (also fetches + links)
mypm remove lodash    # Remove a dependency and relink
mypm run build        # Run a package.json script
mypm start            # Run the "start" script
mypm test             # Run the "test" script
mypm build            # Run the "build" script
mypm store path       # Print store location
mypm store status     # Show store size and package count
mypm doctor           # Health check: filesystem, hard link support, store integrity
mypm version          # Print version
```

## Config (~/.mypmrc)

```ini
store-dir   = /home/user/.mypm/store
registry    = https://registry.npmjs.org
link-type   = hardlink   # hardlink | symlink | copy
concurrency = 8
```

## How it works

1. **Current** — `mypm install` reads `package.json`, resolves versions, fetches tarballs from the registry, stores them in the global store (`~/.mypm/store`), and links them into `node_modules` (hardlink/symlink/copy).

2. **Lockfile** — `mypm.lock` captures the full dependency graph and resolved versions for reproducible installs.

## Architecture

```
mypm install / add / remove
  └─ cmd/*.go             CLI flag parsing, orchestration
  └─ internal/config      Reads ~/.mypmrc, manages store paths
  └─ internal/resolver    Semver + dependency resolution
  └─ internal/fetcher     Registry fetch + tarball unpack
  └─ internal/store       Content-addressable store (SHA-256 keyed)
  └─ internal/linker      Hard link / symlink / copy strategy with fallback
  └─ internal/logger      Colored terminal output
```
