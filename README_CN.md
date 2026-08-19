# golocate ⚡

[![Go Version](https://img.shields.io/badge/Go-1.21%2B-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20Windows-orange.svg)](https://github.com)

一个高性能的文件定位工具，使用 Go 语言编写。**golocate** 提供即时文件搜索功能，支持多种协议和现代化的 Web 界面。

**[English](README.md)**

---

## ✨ 特性

- 🚀 **极速搜索** - 高性能搜索，支持子串、正则、通配符和多词（terms）模式
- 🎯 **高级过滤** - 目录范围（--scope）、排除（--exclude）、类型/大小/修改时间过滤、隐藏文件开关、硬链接去重（--dedupe）
- 📄 **内容搜索** - grep 风格输出（行号/上下文），UTF-8 / GBK / UTF-16 编码自动识别，新文件优先扫描
- 🔄 **实时索引** - 自动文件系统监控和索引更新
- 💾 **可插拔持久化** - incremental（默认，低写量）/ snapshot / none，重启秒开
- 🔌 **多协议支持** - 快速协议、JSON 和 JSON-RPC；status 带协议版本号便于兼容检查
- 🌐 **Web 界面** - 内置 H5：中英切换、目录管理、排序、导出、打开/复制路径、离线提示、/healthz
- 🖥️ **GTK 图形界面** - 原生桌面应用
- ⌨️ **CLI 完备** - locate 兼容（-0/-e/退出码）、--open/--open-dir/--copy、--long 长格式、shell 补全、开机自启
- 📊 **运维能力** - 构建进度/统计（含按目录与历史）、日志落盘与轮转、崩溃自愈
- 🔒 **安全可靠** - Unix socket 权限控制 + 路径白名单校验
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

# 高级查询
golocate --terms "server -test"      # 多词 AND + 排除
golocate --scope pkg/ main.go        # 限定目录
golocate --exclude "*.test.*" pkg    # 排除 glob（可重复）
golocate --type go,md --min-size 1K --max-size 10M   # 类型/大小过滤
golocate --mtime-after 2024-01-01 --no-hidden        # 时间过滤/隐藏文件
golocate --long main.go              # 长格式：大小<TAB>时间<TAB>路径
golocate --dedupe main.go            # 硬链接去重（同一文件只显示一次）

# 内容搜索（grep 风格，支持 GBK/UTF-16/UTF-8 编码）
golocate --content keyword [path]

# 结果操作：打开 / 打开所在目录 / 复制路径
golocate --open main.go              # 用默认应用打开第一个匹配
golocate --open-dir main.go          # 打开其所在目录
golocate --copy main.go              # 复制路径到剪贴板（xclip/xsel/pbcopy）

# 与 locate 兼容
golocate -0 main.go | xargs -0 ls -l # NUL 分隔
golocate -e main.go                  # 仅显示仍存在的文件（0=找到 / 1=未找到）

# 守护进程管理
golocated --autostart                # 开机自启（用户级）
golocated --no-autostart             # 移除自启条目
golocated --service-status           # 查看服务状态（含构建进度/统计）
```

### 运维端点（golocate-h5 提供）

```bash
curl http://127.0.0.1:8080/healthz   # 存活探针（daemon 可达 200 / 不可达 503）
curl http://127.0.0.1:8080/metrics    # Prometheus 文本指标（搜索/内容搜索/打开/构建计数 + 索引文件数）
```

### 协议 API

golocate 支持三种协议用于编程访问：

#### 快速协议（默认）

```bash
echo "method=search
content=test
ignore_case=true
limit=10
" | nc -U /tmp/golocate.sock
```

#### JSON 协议

```bash
printf '{"method":"search","content":"test","ignore_case":true,"limit":10}\n' | nc -U /tmp/golocate.sock
```

#### 方法清单

`search` / `status` / `get-config` / `set-config` / `build` / `reload-config` / `open`（经 pathValidator 白名单校验后用默认应用打开文件/目录）/ `stop`

#### JSON-RPC 协议

```bash
printf '{"jsonrpc":"2.0","method":"search","params":{"content":"test"},"id":1}\n' | nc -U /tmp/golocate.sock
```

---

## 🌐 远程访问（可选）

golocate 默认仅本机使用（Unix socket + H5 绑 `127.0.0.1`）。如需局域网访问，可将 H5 绑定到非回环地址（**注意：无认证，仅建议在可信内网使用**）：

```bash
golocate-h5 -addr 0.0.0.0:8080
```

daemon 本身的搜索接口仍走本机 socket，不会直接暴露。

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
# 未配置时使用默认忽略集：VCS 元数据 (.git/.svn/.hg/.bzr)、node_modules、
# 缓存/临时/备份文件 (*.cache/ *.tmp/ *.swp/ *.swo/ *.bak)、.DS_Store/Thumbs.db
ignore_patterns:
  - "*.log"
  - "*.tmp"
  - ".git"
  - "node_modules"

# 性能设置
worker_count: 4
index_interval: 24h
max_file_size: 10485760  # 10MB
throttle_index: true     # 节流定时/后台重建
throttle_window: 10m     # 开机窗口：后台扫描低频运行；收到搜索请求立即提速

# 可选：内容倒排索引（内存，单 token 内容搜索精确候选；重建时全量生成，watcher 事件增量维护）
content_index: false

# 持久化策略（可插拔组件）
persist_mode: incremental # incremental（默认）| snapshot | none
persist_flush_interval: 30s # incremental 模式：watcher 变更批量落盘间隔
snapshot_max_age: 24h    # snapshot 模式：超过该年龄的快照在启动时触发后台重建

# 服务器设置
socket_path: /tmp/golocate.sock
```

### 💾 持久化策略

索引常驻内存且为权威数据，持久化只为了让重启更廉价（打开即可搜索，无需等待全量重建）。持久化层是可插拔组件（`internal/persist`），由 `persist_mode` 选择：

| 模式 | 行为 | 适用场景 |
|------|------|----------|
| `incremental`（默认） | 构建后写全量基线，之后 watcher 变更以**批量低量写**落盘（`persist_flush_interval`，默认 30s）。存储索引持续保持最新，**无定期全量重写**——文件系统安静时零写入 | 系统服务 / 大索引：启动秒开 + SSD 友好（写量 ≈ 实际变更量，每天几 MB 而非每周期几百 MB） |
| `snapshot` | 每次索引构建完成后写全量快照；启动时按"目录指纹匹配 + 非 dirty + 快照未超过 `snapshot_max_age`"决策恢复 | 明确的全量快照语义；仅在重建时写盘 |
| `none` | 完全不落盘；冷启动后台节流重建 | 小索引、最省 SSD（零写入） |

当没有可用快照时（首次运行、目录变更、快照过期或 dirty），守护进程**立即以空索引提供服务，并以后台节流方式重建**（构建完成后热切换）——开机不卡顿，搜索秒级可用。

服务启动后的 `throttle_window`（默认 `10m`）内，自动后台扫描以低 IO 运行；**收到第一个搜索请求立即把节流提升到全速**——等待结果的用户不会被开机友好的节奏拖慢。

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
│   ├── persist/            # 可插拔持久化策略（snapshot / none）
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
test/                       # 集成测试（socket 级 API 测试）
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
