# golocate ⚡

[![Go Version](https://img.shields.io/badge/Go-1.21%2B-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20Windows-orange.svg)](https://github.com)

一个高性能的文件定位工具，使用 Go 语言编写。**golocate** 提供即时文件搜索功能，支持多种协议和现代化的 Web 界面。

**[English](README.md)**

---

## ✨ 特性

- 🚀 **极速搜索** - 高性能搜索，支持子串、正则和通配符模式
- 🔄 **实时索引** - 自动文件系统监控和索引更新
- 🔌 **多协议支持** - 支持快速协议、JSON 和 JSON-RPC
- 🌐 **Web 界面** - 内置 H5 界面，访问便捷
- 🖥️ **GTK 图形界面** - 原生桌面应用
- 🔒 **安全可靠** - Unix socket 带权限限制
- 📦 **轻量级** - 最小的内存占用和依赖

---

## 📦 安装

### 从源码构建

```bash
# 克隆仓库
git clone https://github.com/yourusername/golocate.git
cd golocate

# 构建
go build -o bin/golocated ./cmd/golocated/
go build -o bin/golocate ./cmd/golocate/
go build -o bin/golocate-h5 ./ui/h5/cmd/golocate-h5/

# 可选：构建 GTK GUI（需要 GTK 依赖）
go build -o bin/golocate-gtk ./ui/gtk/cmd/golocate-gtk/
```

### 快速开始

```bash
# 启动守护进程
./bin/golocated --service &

# 搜索文件
./bin/golocate test

# 带选项搜索
./bin/golocate -i -l 20 pattern
```

---

## 🚀 使用方法

### 命令行

```bash
# 基本搜索
golocate <pattern>

# 忽略大小写
golocate -i <pattern>

# 限制结果数量
golocate -l 20 <pattern>

# 正则表达式搜索
golocate -r <regex_pattern>

# 仅搜索文件名
golocate -b <pattern>
```

### 协议 API

golocate 支持三种协议用于编程访问：

#### 快速协议（默认）

```bash
echo "method=search
data_content=test
ignore_case=true
limit=10
" | nc -U /tmp/golocate.sock
```

#### JSON 协议

```bash
printf '{"method":"search","data_content":"test","ignore_case":true,"limit":10}\n' | nc -U /tmp/golocate.sock
```

#### JSON-RPC 协议

```bash
printf '{"jsonrpc":"2.0","method":"search","params":{"data_content":"test"},"id":1}\n' | nc -U /tmp/golocate.sock
```

---

## 🌐 平台说明

### Linux
- 使用 **Unix socket** (`/tmp/golocate.sock`)
- 高性能且安全
- 推荐用于生产环境

### Windows
- 使用 **Named Pipe** (`\\.\pipe\golocate`)
- Windows 原生 IPC 机制
- 比 TCP socket 更安全（只有当前用户可访问）
- 无端口冲突

```powershell
# Windows 示例（PowerShell）
# 启动守护进程
.\bin\golocated.exe --service

# 搜索文件
.\bin\golocate.exe test

# Named Pipe 路径：\\.\pipe\golocate
```

```bash
printf '{"jsonrpc":"2.0","method":"search","params":{"path":"test"},"id":1}\n' | nc -U /tmp/golocate.sock
```

---

## ⚙️ 配置

配置文件：`~/.config/golocate/config.yaml`

```yaml
# 要索引的目录
directories:
  - /home/user/projects
  - /home/user/documents

# 忽略模式
ignore_patterns:
  - "*.log"
  - "*.tmp"
  - ".git"
  - "node_modules"

# 性能设置
worker_count: 4
index_interval: 3h
max_file_size: 10485760  # 10MB

# 服务器设置
socket_path: /tmp/golocate.sock
```

---

## 🏗️ 架构

```
┌─────────────────────────────────────────────────────────────┐
│                      golocate                                │
├─────────────────────────────────────────────────────────────┤
│  命令行客户端   │  H5 Web界面  │  GTK GUI   │  API 客户端   │
├─────────────────────────────────────────────────────────────┤
│              协议层 (Fast/JSON/JSON-RPC)                     │
├─────────────────────────────────────────────────────────────┤
│                     golocated (守护进程)                     │
│  ┌──────────────┬──────────────┬────────────────────────┐  │
│  │  索引构建    │  文件监控    │  搜索引擎              │  │
│  └──────────────┴──────────────┴────────────────────────┘  │
├─────────────────────────────────────────────────────────────┤
│                 Unix Socket (/tmp/golocate.sock)            │
└─────────────────────────────────────────────────────────────┘
```

---

## 📊 性能

| 索引文件数 | 搜索延迟（目标） | 索引构建时间（目标） |
|------------|------------------|----------------------|
| 10万       | < 10ms           | < 1s                 |
| 100万      | < 50ms           | < 10s                |
| 1000万     | < 100ms          | < 2分钟              |

*设计目标。参考硬件实际性能（Intel i7-10700K, NVMe SSD）：100万文件约 262ms。优化状态见 `docs/INDEX_STRATEGY.md` 和 `docs/overdesign-analysis.md`。*

---

## 🔧 开发

### 项目结构

```
golocate/
├── cmd/                    # 应用程序入口
│   ├── golocate/           # CLI 客户端
│   ├── golocated/          # 守护进程服务器
│   └── memory-analyze/     # 内存使用分析工具
├── internal/               # 私有包
│   ├── client/             # Socket 客户端
│   ├── database/           # BBolt 持久化
│   ├── scheduler/          # 定时索引重建
│   ├── server/             # 请求处理器
│   ├── socket/             # 平台 socket 抽象
│   ├── svc/                # 服务生命周期
│   └── testutil/           # 测试辅助
├── pkg/                    # 公共包
│   ├── cli/                # CLI 参数解析
│   ├── config/             # YAML 配置
│   ├── content/            # 文件内容索引
│   ├── errors/             # 错误类型
│   ├── ignore/             # 忽略模式匹配
│   ├── index/              # 内存索引 + 搜索
│   ├── message/            # 协议解析 + 工作分发
│   │   └── protocol/       # Fast / JSON / JSON-RPC 编码器
│   ├── pool/               # 对象池
│   ├── search/             # 结果格式化
│   ├── security/           # 权限检查
│   └── watcher/            # fanotify / fsnotify
└── ui/                     # 用户界面
    ├── h5/                 # Web UI
    └── gtk/                # GTK GUI
```

### 运行测试

```bash
# 快速测试（推荐用于 CI）
go test -short ./...

# 完整测试（包含性能基准测试）
go test ./...
```

---

## 🤝 贡献

欢迎贡献代码！请随时提交 Pull Request。

1. Fork 本仓库
2. 创建功能分支 (`git checkout -b feature/amazing-feature`)
3. 提交更改 (`git commit -m 'Add amazing feature'`)
4. 推送到分支 (`git push origin feature/amazing-feature`)
5. 打开 Pull Request

---

## 📝 许可证

本项目采用 MIT 许可证 - 详情请见 [LICENSE](LICENSE) 文件。

---

## 🙏 致谢

- 灵感来源于 GNU `locate` 和 `fsearch`
- 使用 Go 语言用心构建

---

## 📧 联系方式

如有问题或反馈，请在 GitHub 上提交 issue。

**祝搜索愉快！🔍**
