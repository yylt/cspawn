# cspawn - 轻量级容器运行时

## 简介

`cspawn` 是一个轻量级容器运行时工具，旨在基于本地文件系统或 containerd 准备的 rootfs 快速启动容器。它专注于提供最小化的容器隔离，支持 rootfs 隔离、设备挂载和文件系统绑定，但不提供网络和存储隔离。

## 特性

- **多种 rootfs 准备方式**：支持本地目录和 containerd 镜像
- **Overlay 文件系统**：支持只读 rootfs + 可写层，保护原始镜像
- **YAML 配置文件**：支持配置文件，命令行参数优先级更高
- **设备支持**：自动准备 `/dev`、`/proc`、`/sys` 等常用设备文件系统
- **文件系统挂载**：自动挂载 /proc、/sys、/dev 等必要的文件系统
- **绑定挂载**：支持将宿主机目录绑定到容器内
- **环境变量**：支持文件和环境变量设置
- **用户切换**：支持指定运行用户和组
- **工作目录**：支持指定容器内工作目录
- **systemd 集成**：设计为可与 systemd 服务配合使用

## 默认数据路径

- **数据根目录**：`/var/lib/cspawn/`
- **rootfs 存储**：`/var/lib/cspawn/rootfs/`
- **overlay 工作目录**：`/var/lib/cspawn/workdirs/`
- **配置文件**：`/etc/cspawn/config.yaml`

## 安装

```bash
# 构建
make build

# 安装
sudo cp _out/linux/amd64/cspawn /usr/local/bin/
```

## 命令选项

### 基本选项

| 选项 | 简写 | 类型 | 描述 |
|------|------|------|------|
| `--config` | `-f` | string | 配置文件路径 (默认: `/etc/cspawn/config.yaml`) |
| `--runtime` | `-r` | string | 运行时类型：`local://path` (默认: `local:///var/lib/cspawn`) 或 `containerd://socket` |
| `--dir` | `-d` | string | 本地 rootfs 目录路径 (与 `-i` 互斥) |
| `--image` | `-i` | string | 容器镜像 (名称:标签 或 名称@sha256:摘要) (与 `-d` 互斥) |
| `--workdir` | `-w` | string | Overlay 工作目录 (默认: `workdirs/<名称>`) |
| `--overlay` | - | bool | 启用 overlay 文件系统 |
| `--env` | `-e` | string | 环境变量 (可多次指定，格式：`KEY=VALUE`) |
| `--envfile` | `-E` | string | 环境变量文件路径 |
| `--user` | `-u` | string | 运行用户 (格式：`uid:gid`) |
| `--chdir` | `-c` | string | 容器内工作目录 |
| `--bind` | `-b` | string | 绑定挂载 (格式：`host:container[:ro\|rw]`) |

### 命令参数

- `cmd [args...]`：在容器内执行的命令及其参数

## YAML 配置文件

配置文件默认路径：`/etc/cspawn/config.yaml`，可通过 `-f` 指定其他路径。

**优先级**：命令行参数 > 配置文件 > 默认值

### 配置文件格式

```yaml
# 运行时配置
runtime: local:///var/lib/cspawn
# runtime: containerd://unix:///run/containerd/containerd.sock

# 镜像或 rootfs 目录 (二选一)
image: debian:trixie-slim
# rootfs_dir: /path/to/rootfs

# Overlay 工作目录
work_dir: /var/lib/cspawn/workdirs/myapp

# 禁用 overlay
no_overlay: false

# 环境变量
env:
  - TERM=xterm
  - LANG=C.UTF-8
  - GOPATH=/go

# 环境变量文件
env_file: /path/to/envfile

# 绑定挂载
binds:
  - /host/path:/container/path:rw
  - /host/data:/data:ro

# 运行用户 (uid:gid)
user: "1000:1000"

# 容器内工作目录
chdir: /workspace

# 调试模式
debug: false
```

## Overlay 文件系统

默认**不使用** overlay 文件系统，直接使用 rootfs。如需使用 overlay 文件系统（将 rootfs 作为只读层，所有写入在独立的可写层进行），需要明确启用。

### 工作目录结构

```
/var/lib/cspawn/workdirs/<名称>/
├── upper/    # 可写层
├── work/     # overlay 工作目录
└── merged/   # 合并后的挂载点
```

### 目录命名规则

| 镜像格式 | 目录名 |
|---------|--------|
| `name:tag` | `name_tag` |
| `name@sha256:digest` | `name_sha256-digest` (前12位) |
| 自定义 rootfs | 使用目录名 |

### 启用 Overlay

使用 `--overlay` 选项时，启用 overlay 文件系统，将 rootfs 作为只读层，所有写入在独立的可写层进行：

```bash
cspawn --overlay -i debian:trixie-slim /bin/bash
```

或在配置文件中：

```yaml
overlay: true
```

## 命名空间隔离

| 命名空间 | 状态 | 说明 |
|---------|------|------|
| `CLONE_NEWNS` | ✅ | mount 隔离，独立文件系统视图 |
| `CLONE_NEWUTS` | ✅ | hostname 隔离，不影响宿主 |
| `CLONE_NEWPID` | ❌ | 进程隔离 (Go runtime 兼容性问题) |
| `CLONE_NEWIPC` | ❌ | IPC 隔离 |

## 使用示例

### 1. 使用镜像启动

```bash
# 基本使用
cspawn -i debian:trixie-slim /bin/bash

# 指定环境变量
cspawn -i golang:1.25 -e GOPATH=/go -e GOCACHE=/root/.cache/go-build /bin/bash

# 使用自定义镜像仓库
cspawn -i docker.yylt.gq/library/debian:trixie-slim /bin/bash
```

### 2. 使用配置文件

```bash
# 使用默认配置文件
cspawn /bin/bash

# 指定配置文件
cspawn -f /path/to/config.yaml /bin/bash

# 命令行覆盖配置文件
cspawn -f /path/to/config.yaml -i ubuntu:22.04 /bin/bash
```

### 3. 使用本地 rootfs

```bash
cspawn -d /path/to/rootfs /bin/bash
```

### 4. 设置用户和工作目录

```bash
cspawn -i debian:trixie-slim -u 1000:1000 -c /home/user /bin/bash
```

### 5. 绑定挂载

```bash
# 读写绑定
cspawn -i debian:trixie-slim -b /host/data:/container/data /bin/bash

# 只读绑定
cspawn -i debian:trixie-slim -b /host/config:/container/config:ro /bin/bash
```

### 6. 完整示例

```bash
cspawn \
  -i golang:1.25 \
  -e GOPATH=/go \
  -e GOCACHE=/root/.cache/go-build \
  -e GOPROXY=https://goproxy.cn,direct \
  -u 1000:1000 \
  -c /workspace \
  -b /home/user/projects:/workspace:rw \
  -b /var/run/docker.sock:/var/run/docker.sock:ro \
  /bin/bash
```

更多示例（包括不同场景的配置文件和 systemd service 文件）见 `examples/` 目录。

## systemd 服务集成

### 基本服务文件

创建 `/etc/systemd/system/myapp.service`：

```ini
[Unit]
Description=My Application in cspawn
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/cspawn -f /etc/cspawn/myapp.yaml /usr/bin/myapp --port 8080
Restart=on-failure
RestartSec=5

# 资源限制
LimitNOFILE=65536
LimitNPROC=4096

# 安全设置
ProtectSystem=strict
ProtectHome=yes
NoNewPrivileges=yes

[Install]
WantedBy=multi-user.target
```

### 配置文件 `/etc/cspawn/myapp.yaml`

```yaml
runtime: local:///var/lib/cspawn
image: myapp:latest

env:
  - APP_ENV=production
  - LOG_LEVEL=info

binds:
  - /var/lib/myapp/data:/data:rw
  - /var/log/myapp:/var/log/app:rw

user: "1000:1000"
work_dir: /var/lib/cspawn/workdirs/myapp
```

### 启用服务

```bash
systemctl daemon-reload
systemctl enable myapp
systemctl start myapp
```

更多示例见 `examples/` 目录。

## 运行时实现

### Local Runtime

- 拉取镜像并解压到数据目录内的 rootfs
- 支持 overlay 文件系统
- 自动缓存已拉取的镜像

### Containerd Runtime

- 使用 containerd API 拉取镜像
- 自动提取镜像层到 `/var/lib/cspawn/rootfs/<image-name>`
- 默认使用 containerd 命名空间 `default`

## rootfs 准备

启动容器前，`cspawn` 会准备以下内容：

### 1. 设备文件系统

- 挂载 `/proc`（进程文件系统）
- 挂载 `/sys`（系统文件系统）  
- 挂载 `/dev`（设备文件系统）
- 创建必要的设备节点：
  - `/dev/null`
  - `/dev/zero`
  - `/dev/random`
  - `/dev/urandom`
  - `/dev/tty`
  - `/dev/pts`

### 2. 挂载点

- `tmpfs` 挂载到 `/tmp`、`/run`
- `devpts` 挂载到 `/dev/pts`

### 3. Overlay 层 (默认启用)

- rootfs 作为只读层 (lower)
- `workdirs/<名称>/upper` 作为可写层
- 所有写入在可写层进行，不影响原始 rootfs

### 4. 绑定挂载

- 用户通过 `-b` 指定的所有绑定挂载
- 挂载顺序：先设备文件系统，再 overlay，最后绑定挂载

## 隔离级别

`cspawn` 提供以下隔离：

- ✅ **文件系统隔离**：通过 `chroot`/`pivot_root`
- ✅ **主机名隔离**：通过独立的 UTS 命名空间
- ✅ **挂载隔离**：通过独立的 mount 命名空间
- ✅ **用户隔离**：通过 `setuid`/`setgid` 切换用户
- ✅ **Overlay 隔离**：只读 rootfs + 可写层
- ❌ **网络隔离**：与宿主机共享网络栈
- ❌ **存储隔离**：存储仅通过绑定挂载方式提供

## 日志

`cspawn` 日志输出到标准错误和标准输出，可通过 systemd 的 `StandardOutput` 和 `StandardError` 配置重定向。

## 依赖项

- **containerd**：使用 containerd 运行时需要
- **Linux 内核**：需要支持命名空间、chroot 和 overlay 文件系统
- **Go 1.26+**：构建依赖

## 故障排查

### 常见问题

1. **权限不足**：确保以 root 权限运行
2. **rootfs 不存在**：检查 `-d` 指定的路径
3. **containerd 连接失败**：确保 containerd 服务正在运行
4. **设备挂载失败**：检查内核是否支持相关文件系统
5. **overlay 挂载失败**：检查内核是否支持 overlay 文件系统

### 调试模式

```bash
# 开启详细日志（通过环境变量）
CSPAWN_DEBUG=1 cspawn -i debian:trixie-slim /bin/bash

# 或在配置文件中
debug: true
```

## 许可证

MIT License

## 贡献

欢迎提交 Issue 和 Pull Request。
