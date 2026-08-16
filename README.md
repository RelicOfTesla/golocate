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

- 🚀 **Blazing Fast** - High-performance search with substring, regex, and wildcard modes
- 🔄 **Real-time Index** - Automatic file system monitoring and index updates
- 🔌 **Multiple Protocols** - Fast protocol, JSON, and JSON-RPC support
- 🌐 **Web UI** - Built-in H5 interface for easy access
- 🖥️ **GTK GUI** - Native desktop application
- 🔒 **Secure** - Unix socket with restricted permissions
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

# Optional: Build GTK GUI (requires GTK dependencies)
go build -o bin/golocate-gtk ./ui/gtk/cmd/golocate-gtk/
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
```

### Protocol API

golocate supports three protocols for programmatic access:

#### Fast Protocol (Default)

```bash
echo "method=search
data_content=test
ignore_case=true
limit=10
" | nc -U /tmp/golocate.sock
```

#### JSON Protocol

```bash
printf '{"method":"search","data_content":"test","ignore_case":true,"limit":10}\n' | nc -U /tmp/golocate.sock
```

#### JSON-RPC Protocol

```bash
printf '{"jsonrpc":"2.0","method":"search","params":{"data_content":"test"},"id":1}\n' | nc -U /tmp/golocate.sock
```

---

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
ignore_patterns:
  - "*.log"
  - "*.tmp"
  - ".git"
  - "node_modules"

# Performance settings
worker_count: 4
index_interval: 3h
max_file_size: 10485760  # 10MB

# Server settings
socket_path: /tmp/golocate.sock
```

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
