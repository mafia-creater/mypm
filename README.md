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
mypm install          # Link all deps from existing node_modules into store
mypm add react@18     # (Phase 3) Fetch + link a package
mypm remove lodash    # (Phase 3) Remove a package
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

1. **Phase 1 (current)** — Run `npm install` once, then `mypm install` moves all packages into the global store (`~/.mypm/store`) and replaces node_modules entries with hard links. Zero extra disk space for any subsequent project sharing the same packages.

2. **Phase 3 (planned)** — Full resolver + fetcher. `mypm install` replaces npm entirely.

## Architecture

```
mypm install
  └─ cmd/install.go       CLI flag parsing, orchestration
  └─ internal/config      Reads ~/.mypmrc, manages store paths
  └─ internal/store       Content-addressable store (SHA-256 keyed)
  └─ internal/linker      Hard link / symlink / copy strategy with fallback
  └─ internal/logger      Colored terminal output
```
