# golocate ⚡

[![Go Version](https://img.shields.io/badge/Go-1.21%2B-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20Windows-orange.svg)](https://github.com)

A high-performance file location tool, written in Go. **golocate** provides instant file search capabilities with multiple protocol support and a modern web UI.

**Inspired by [Everything](https://www.voidtools.com/) (Windows) and Linux `locate` command, combining the best of both worlds.**

**🤖 This project is implemented with AI Vibe Coding.**

**[中文文档](README_CN.md)**

---

## ✨ Features

- 🚀 **Blazing Fast** - Substring, regex, wildcard, and multi-term (terms) modes
- 🎯 **Advanced Filters** - scope, exclude globs, type/size/mtime filters, hidden-file toggle, hard-link dedupe (--dedupe)
- 📄 **Content Search** - grep-style output (line numbers/context), UTF-8 / GBK / UTF-16 auto-detection, newest files scanned first
- 🔄 **Real-time Index** - Automatic file system monitoring and index updates
- 💾 **Pluggable Persistence** - incremental (default, low-write) / snapshot / none, instant startup on reboot
- 🔌 **Multiple Protocols** - Fast, JSON, and JSON-RPC; protocol version exposed via status
- 🌐 **Web UI** - Built-in H5: zh/en toggle, directory management, server-side sorting, export, open/copy paths, offline banner, /healthz
- 🖥️ **GTK GUI** - Native desktop application
- ⌨️ **Complete CLI** - locate-compatible (-0/-e/exit codes), --open/--open-dir/--copy, --long format, shell completion, user autostart
- 📊 **Ops** - Build progress/stats (per-directory & history), log file with rotation, crash self-healing
- 🔒 **Secure** - Unix socket permissions + path whitelist validation
- 📦 **Lightweight** - Minimal memory footprint and dependencies

---

## 📦 Installation

### Build from Source

```bash
# Clone the repository
git clone https://github.com/yourusername/golocate.git
cd golocate

# Build
go build -o bin/golocated ./cmd/golocated/
go build -o bin/golocate ./cmd/golocate/
go build -o bin/golocate-h5 ./ui/h5/cmd/golocate-h5/

# Optional: Build GTK GUI (requires GTK4 dev + cgo)
#   Debian/Ubuntu: sudo apt install libgtk-4-dev gcc pkg-config libglib2.0-dev
#   macOS: brew install gtk4
# gotk4 generates full bindings on first build (slower the first time);
# keep the module/tool cache outside /tmp to avoid rebuilds if /tmp is wiped.
export CGO_ENABLED=1
go build -o bin/golocate-gtk ./ui/gtk/cmd/golocate-gtk/
# verify: ./bin/golocate-gtk -h   (prints gotk4/GApplication options; -s connects a socket)
```

### Quick Start

```bash
# Start the daemon
./bin/golocated --service &

# Search for files
./bin/golocate test

# Search with options
./bin/golocate -i -l 20 pattern
```

---

## 🚀 Usage

### Command Line

```bash
# Basic search
golocate <pattern>

# Case-insensitive search
golocate -i <pattern>

# Limit results
golocate -l 20 <pattern>

# Regex search
golocate -r <regex_pattern>

# Basename search only
golocate -b <pattern>

# Advanced queries
golocate --terms "server -test"      # multi-term AND + exclude
golocate --scope pkg/ main.go        # scope to a directory
golocate --exclude "*.test.*" pkg    # exclude globs (repeatable)
golocate --type go,md --min-size 1K --max-size 10M   # type/size filters
golocate --mtime-after 2024-01-01 --no-hidden        # time/hidden filters
golocate --long main.go              # long format: size<TAB>time<TAB>path
golocate --dedupe main.go            # collapse hard links to one result

# Content search (grep-style; GBK / UTF-16 / UTF-8 aware)
golocate --content keyword [path]

# Actions on the first match: open / open directory / copy path
golocate --open main.go              # open with the default application
golocate --open-dir main.go          # open its parent directory
golocate --copy main.go              # copy the path to the clipboard

# locate compatibility
golocate -0 main.go | xargs -0 ls -l # NUL-separated output
golocate -e main.go                  # only existing files (exit 0=found / 1=none)

# Daemon management
golocated --autostart                # install a user autostart entry
golocated --no-autostart             # remove the autostart entry
golocated --service-status           # status incl. build progress/stats
```

### Ops Endpoints (served by golocate-h5)

```bash
curl http://127.0.0.1:8080/healthz   # liveness probe (200 when the daemon is reachable, else 503)
curl http://127.0.0.1:8080/metrics    # Prometheus text metrics (search/content/open/build counters + indexed files)
```

### Protocol API

golocate supports three protocols for programmatic access:

#### Fast Protocol (Default)

```bash
echo "method=search
content=test
ignore_case=true
limit=10
" | nc -U /tmp/golocate.sock
```

#### JSON Protocol

```bash
printf '{"method":"search","content":"test","ignore_case":true,"limit":10}\n' | nc -U /tmp/golocate.sock
```

#### Method List

`search` / `status` / `get-config` / `set-config` / `build` / `reload-config` / `open` (opens a file/directory with the default app after pathValidator whitelist check) / `stop`

#### JSON-RPC Protocol

```bash
printf '{"jsonrpc":"2.0","method":"search","params":{"content":"test"},"id":1}\n' | nc -U /tmp/golocate.sock
```

---

## 🌐 Remote Access (optional)

golocate is local-only by default (Unix socket + H5 bound to `127.0.0.1`). For LAN access, bind the H5 bridge to a non-loopback address (**no authentication — trusted networks only**):

```bash
golocate-h5 -addr 0.0.0.0:8080
```

The daemon's search socket itself is never exposed directly.

## 🌐 Platform Notes

### Linux
- Uses **Unix socket** (`/tmp/golocate.sock`)
- High performance and secure
- Recommended for production use

#### fanotify Requirements

golocate uses **fanotify** for real-time file system monitoring on Linux. To use fanotify:

**Kernel Requirements**:
- Linux kernel 2.6.37 or later
- fanotify support enabled in kernel config (`CONFIG_FANOTIFY=y`)

**Permission Requirements**:
- **Option 1**: Run as root (not recommended for production)
- **Option 2**: Set `CAP_SYS_ADMIN` capability on the binary:
  ```bash
  sudo setcap cap_sys_admin+ep ./bin/golocated
  ```

**Fallback Mode**:
- If fanotify is not available, golocate will automatically fallback to **inotify** or **fsnotify**
- No manual configuration needed - the system auto-detects the best available watcher

**Checking fanotify Support**:
```bash
# Check kernel version (requires 2.6.37+)
uname -r

# Check if fanotify is enabled in kernel config
grep CONFIG_FANOTIFY /boot/config-$(uname -r)
```

### Windows
- Uses **Named Pipe** (`\\.\pipe\golocate`)
- Native Windows IPC mechanism
- More secure than TCP socket (only current user can access)
- No port conflicts

```powershell
# Windows example (PowerShell)
# Start the daemon
.\bin\golocated.exe --service

# Search for files
.\bin\golocate.exe test

# Named Pipe path: \\.\pipe\golocate
```

---

## ⚙️ Configuration

Configuration file: `~/.config/golocate/config.yaml`

```yaml
# Directories to index
directories:
  - /home/user/projects
  - /home/user/documents

# Ignore patterns
# When unset, a sensible default set applies: VCS metadata (.git/.svn/.hg/.bzr),
# node_modules, cache/temp/backup files (*.cache/ *.tmp/ *.swp/ *.swo/ *.bak),
# .DS_Store / Thumbs.db
ignore_patterns:
  - "*.log"
  - "*.tmp"
  - ".git"
  - "node_modules"

# Performance settings
worker_count: 4
index_interval: 24h
max_file_size: 10485760  # 10MB
throttle_index: true     # throttle periodic/background rebuilds
throttle_window: 10m     # boot window: scans run low-IO; a search request lifts the throttle instantly

# Persistence strategy (pluggable component)
persist_mode: incremental # incremental (default) | snapshot | none
persist_flush_interval: 30s # incremental mode: batch flush interval for watcher changes
snapshot_max_age: 24h    # snapshot mode: snapshots older than this trigger a background rebuild on start

# Optional in-memory content token index (precise single-word content-search
# candidates; rebuilt on every index build, kept fresh by watcher events)
content_index: false

# Server settings
socket_path: /tmp/golocate.sock
```

### 💾 Persistence strategies

The index lives in memory and is authoritative; persistence only exists to
make restarts cheap (search works immediately instead of waiting for a full
rescan). The persistence layer is a pluggable component (`internal/persist`)
selected by `persist_mode`:

| Mode | Behavior | When to use |
|------|----------|-------------|
| `incremental` (default) | Full baseline after each build, then watcher-driven changes applied in batched low-volume writes (`persist_flush_interval`, default 30s). The stored index stays current with **no periodic full rewrites** — a quiet filesystem writes nothing | System services / large indexes: instant startup AND SSD-friendly (write volume ≈ actual changes, e.g. a few MB/day instead of hundreds of MB every cycle) |
| `snapshot` | Full snapshot written after each index build; restored on start when the directory fingerprint matches, the snapshot is not marked dirty, and it is not older than `snapshot_max_age` | Explicit full-snapshot semantics; rebuild-triggered writes only |
| `none` | No persistence at all; cold start rebuilds in the background (throttled) | Small indexes, maximum SSD friendliness (zero writes) |

When no usable snapshot exists (first run, config change, stale or dirty
snapshot), the daemon starts serving immediately with an empty index and
rebuilds **in the background with throttled IO**, hot-swapping the result —
so boot does not freeze the machine and search is available in seconds.

Within `throttle_window` after service start (default `10m`), automatic
background scans run at low IO; **the first search request lifts the throttle
to full speed immediately**, so a user waiting for results never pays for the
boot-friendly pacing.

---

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      golocate                                │
├─────────────────────────────────────────────────────────────┤
│  CLI Client    │  H5 Web UI    │  GTK GUI    │  API Client  │
├─────────────────────────────────────────────────────────────┤
│              Protocol Layer (Fast/JSON/JSON-RPC)            │
├─────────────────────────────────────────────────────────────┤
│                     golocated (Daemon)                      │
│  ┌──────────────┬──────────────┬────────────────────────┐  │
│  │  Index Build │  File Watch  │  Search Engine         │  │
│  └──────────────┴──────────────┴────────────────────────┘  │
├─────────────────────────────────────────────────────────────┤
│                 Unix Socket (/tmp/golocate.sock)            │
└─────────────────────────────────────────────────────────────┘
```

---

## 📊 Performance

| Files Indexed | Search Latency (target) | Index Build Time (target) |
|---------------|-------------------------|---------------------------|
| 100K          | < 10ms                  | < 1s                      |
| 1M            | < 50ms                  | < 10s                     |
| 10M           | < 100ms                 | < 2min                    |

*Design targets. Actual performance on reference hardware (Intel i7-10700K, NVMe SSD): ~262ms for 1M files. See `docs/INDEX_STRATEGY.md` and `docs/overdesign-analysis.md` for optimization status.*

---

## 🔧 Development

### Project Structure

```
golocate/
├── cmd/                    # Application entry points
│   ├── golocate/           # CLI client
│   ├── golocated/          # Daemon server
│   └── memory-analyze/     # Memory usage analysis tool
├── internal/               # Private packages
│   ├── client/             # Socket client
│   ├── database/           # BBolt persistence
│   ├── persist/            # Pluggable persistence strategies (snapshot/none)
│   ├── scheduler/          # Periodic index rebuild
│   ├── server/             # Request handlers
│   ├── socket/             # Platform socket abstractions
│   ├── svc/                # Service lifecycle
│   └── testutil/           # Test helpers
├── pkg/                    # Public packages
│   ├── cli/                # CLI flag parsing
│   ├── config/             # YAML configuration
│   ├── content/            # File content indexing
│   ├── errors/             # Error types
│   ├── ignore/             # Ignore pattern matching
│   ├── index/              # In-memory index + search
│   ├── message/            # Protocol parsing + worker dispatch
│   │   └── protocol/       # Fast / JSON / JSON-RPC encoders
│   ├── pool/               # Object pools
│   ├── search/             # Result formatting
│   ├── security/           # Permission checks
│   └── watcher/            # fanotify / fsnotify
└── ui/                     # User interfaces
    ├── h5/                 # Web UI
    └── gtk/                # GTK GUI
test/                       # Integration tests (socket-level API tests)
```

### Running Tests

```bash
# Fast suite (recommended for CI)
go test -short ./...

# Full suite (includes performance benchmarks)
go test ./...
```

---

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

---

## 📝 License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

---

## 🙏 Acknowledgments

- Inspired by GNU `locate` and `fsearch`
- Built with love using Go

---

## 📧 Contact

For questions or feedback, please open an issue on GitHub.

**Happy searching! 🔍**
