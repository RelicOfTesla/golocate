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
- 🌐 **Web UI (H5)** - zh/en toggle, right-click context menu, name⇄content AND search, mtime filter, tab pages (favorites/settings/server/status), column resize + drag persistence, paged full export (csv/json), open-capability aware buttons, compact one-row header
- 🖥️ **GTK GUI** - Right-click context menu (open / open-dir / copy file-name / copy full-path / favorite), favorites + recent-opened dialogs, pattern dropdown, full filters (basename/type/scope/exclude/size/mtime/dedupe), server-side sort with paging, column drag + persistence, full export (CSV/JSON), collapsed advanced options, window decorations
- ⌨️ **Complete CLI** - locate-compatible (-0/-e/exit codes), --open/--open-dir/--copy, --long format, structured **--json** output, streaming with prefetch (no batch pauses), shell completion, user autostart
- 🔌 **Auto-start daemon** - cli/gtk/h5 auto-start the single shared golocated when unreachable (`--auto-start-server=none|child|background`, cross-process lock)
- ⏱ **Idle auto-exit** - `golocated --idle-timeout 1h` exits after that long without any request (default: never)
- 📊 **Ops** - Build progress/stats (per-directory & history), per-request latency logs + slow-request threshold (`slow_request_ms`), log rotation, crash self-healing
- 🔒 **Secure** - Unix socket permissions + path whitelist validation
- 📦 **Lightweight & Fast** - sort-result cache + streaming pipeline for big result sets; minimal memory footprint

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

# Structured machine-readable output (path results or content matches)
golocate --json '*.go'
golocate --json --content keyword

# Auto-start the daemon when unreachable (cli/gtk/h5 all support this)
golocate --auto-start-server=none    # never auto-start (default: child)
golocate --auto-start-server=child   # spawn as our child; shared by all clients
golocate --auto-start-server=background   # detached daemon, keeps running

# Daemon management
golocated --autostart                # install a user autostart entry
golocated --no-autostart             # remove the autostart entry
golocated --service-status           # status incl. build progress/stats
golocated --stop                     # stop (socket-first; also stops auto-started daemons)
golocated --idle-timeout 1h          # auto-exit after 1h without any request
golocated --idle-timeout=900s        # same, seconds form
```

### Ops Endpoints (served by golocate-h5)

```bash
curl http://127.0.0.1:8080/healthz   # liveness probe (200 when the daemon is reachable, else 503)
curl http://127.0.0.1:8080/metrics    # Prometheus text metrics (search/content/open/build counters + indexed files)
```

### Protocol API

Programmatic access uses Fast / JSON / JSON-RPC over the Unix socket.
Methods: `search` / `status` / `get-config` / `set-config` / `build` / `reload-config` / `open` / `stop`.
Protocol version is exposed via `status`. The CLI exposes this via `golocate --json` and flags.


---

## 🌐 Remote Access (optional)

golocate is local-only by default (Unix socket + H5 bound to `127.0.0.1`). For LAN access, bind the H5 bridge to a non-loopback address (**no authentication — trusted networks only**):

```bash
golocate-h5 -addr 0.0.0.0:8080
```

The daemon's search socket itself is never exposed directly.

## 🌐 Platform Notes

- **Linux** — Unix socket (`/tmp/golocate.sock`), real-time monitoring via **fanotify** (kernel 2.6.37+, `CONFIG_FANOTIFY`); automatic fallback to inotify/fsnotify when unavailable. If fanotify is blocked by permissions, grant it once: `sudo setcap cap_sys_admin+ep ./bin/golocated`.
- **Windows** — Named Pipe (`\\.\pipe\golocate`), current-user only, no port conflicts.
- Local-first by default; see *Remote Access* above.



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

A single daemon (`golocated`) owns the in-memory index, file watcher and search
engine, serving all clients over one Unix socket. Clients are thin: CLI, built-in
H5 web UI, GTK GUI, and the API layer. The CLI/GTK/H5 can auto-start the daemon
when it is unreachable (single shared instance).



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

### Project Structure (highlights)

`cmd/golocate` (CLI), `cmd/golocated` (daemon), `ui/h5` (web), `ui/gtk` (GUI);
`pkg/index` (index+search), `pkg/message/protocol` (Fast/JSON/JSON-RPC),
`pkg/autostart` (auto-start daemon), `internal/svc|persist|watcher|server`,
`test/` (socket-level integration tests).



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
