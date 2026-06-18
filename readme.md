# cspawn - 轻量级容器运行时

## 简介

`cspawn` 是一个轻量级容器运行时工具，旨在基于本地文件系统或 containerd 准备的 rootfs 快速启动容器。它专注于提供最小化的容器隔离，支持 rootfs 隔离、设备挂载和文件系统绑定，但不提供网络和存储隔离。

## 特性

- **多种 rootfs 准备方式**：支持本地目录和 containerd 镜像
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
- **容器配置**：`/var/lib/cspawn/config/`

## 安装

```bash
# 构建
go build -o cspawn ./cmd/cspawn

# 安装
sudo cp cspawn /usr/local/bin/
```

## 命令选项

### 基本选项

| 选项 | 简写 | 类型 | 描述 |
|------|------|------|------|
| `--runtime` | `-r` | string | 运行时类型：`local(默认)` 或 `containerd` |
| `--rootfs-dir` | `-d` | string | 本地 rootfs 目录路径, 必须已存在且包含完整的文件系统结构（与 `-i` 互斥） |
| `--image` | `-i` | string | 容器镜像名称（与 `-d` 互斥） |
| `--env-file` | `-E` | string | 环境变量文件路径 |
| `--env` | `-e` | string | 设置环境变量（可多次指定，格式：`KEY=VALUE`） |
| `--user` | `-u` | string | 运行用户（格式：`user:group`） |
| `--chdir` | `-c` | string | 容器内工作目录 |
| `--bind` | `-b` | string | 绑定挂载（格式：`host-dir:container-dir[:options]`） |

### 命令参数

- `cmd [args...]`：在容器内执行的命令及其参数

## 使用示例

### 1. 使用本地 rootfs

```bash
# 准备本地 rootfs
mkdir -p /var/lib/cspawn/rootfs/ubuntu
# 解压 Ubuntu rootfs 到该目录...

# 启动容器
cspawn -r local -d /var/lib/cspawn/rootfs/ubuntu /bin/bash
```

### 2. 使用 containerd 镜像

```bash
# 从 containerd 拉取镜像并启动
cspawn -r containerd -i docker.io/library/ubuntu:22.04 /bin/bash
```

### 3. 设置环境变量

```bash
# 单个环境变量
cspawn -r local -d /var/lib/cspawn/rootfs/ubuntu -e PATH=/usr/local/bin:/usr/bin -e HOME=/root /bin/bash

# 环境变量文件
echo "PATH=/usr/local/bin:/usr/bin" > /tmp/env.txt
echo "HOME=/root" >> /tmp/env.txt
cspawn -r local -d /var/lib/cspawn/rootfs/ubuntu -E /tmp/env.txt /bin/bash
```

### 4. 指定用户和工作目录

```bash
cspawn -r local -d /var/lib/cspawn/rootfs/ubuntu -u 1000:1000 -c /home/user /bin/bash
```

### 5. 绑定挂载

```bash
# 只读绑定
cspawn -r local -d /var/lib/cspawn/rootfs/ubuntu -b /host/data:/container/data:ro /bin/bash

# 读写绑定（默认）
cspawn -r local -d /var/lib/cspawn/rootfs/ubuntu -b /host/data:/container/data /bin/bash
```

### 6. 完整示例

```bash
cspawn \
  -r local \
  -d /var/lib/cspawn/rootfs/ubuntu \
  -e TERM=xterm-256color \
  -e LANG=en_US.UTF-8 \
  -u 1000:1000 \
  -c /app \
  -b /tmp/build:/build \
  -b /var/run/docker.sock:/var/run/docker.sock:ro \
  /bin/bash
```

## systemd 服务集成

创建 systemd service 文件 `/etc/systemd/system/cspawn-example.service`：

```ini
[Unit]
Description=Example cspawn container
After=network.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/cspawn \
    -r containerd \
    -i docker.io/library/ubuntu:22.04 \
    -e TERM=linux \
    -u root:root \
    -c / \
    /bin/bash -c "echo 'Container started' && sleep 5"
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
```

启用并启动服务：

```bash
systemctl enable cspawn-example.service
systemctl start cspawn-example.service
```

## 运行时实现

### Local Runtime

- 拉取镜像并解压到数据目录内的 rootfs

### Containerd Runtime

- 使用 containerd API 拉取镜像
- 自动提取镜像层到 `/var/lib/cspawn/rootfs/<image-hash>`
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

### 3. 绑定挂载

- 用户通过 `-b` 指定的所有绑定挂载
- 挂载顺序：先设备文件系统，再绑定挂载

## 隔离级别

`cspawn` 提供以下隔离：

- ✅ **文件系统隔离**：通过 `chroot`/`pivot_root`
- ✅ **进程隔离**：通过独立的 PID 命名空间
- ✅ **用户隔离**：通过 `setuid`/`setgid` 切换用户
- ❌ **网络隔离**：与宿主机共享网络栈
- ❌ **存储隔离**：存储仅通过绑定挂载方式提供

## 日志

`cspawn` 日志输出到标准错误和标准输出，可通过 systemd 的 `StandardOutput` 和 `StandardError` 配置重定向。

## 依赖项

- **containerd**：使用 containerd 运行时需要
- **Linux 内核**：需要支持命名空间和 chroot 相关功能
- **Go 1.21+**：构建依赖

## 配置存储

容器配置存储在 `/var/lib/cspawn/config/` 目录下，每个容器有一个 JSON 格式的配置备份。

## 故障排查

### 常见问题

1. **权限不足**：确保以 root 权限运行
2. **rootfs 不存在**：检查 `-d` 指定的路径
3. **containerd 连接失败**：确保 containerd 服务正在运行
4. **设备挂载失败**：检查内核是否支持相关文件系统

### 调试模式

```bash
# 开启详细日志（通过环境变量）
CSPAWN_DEBUG=1 cspawn -r local -d /var/lib/cspawn/rootfs/ubuntu /bin/bash
```

## 许可证

MIT License

## 贡献

欢迎提交 Issue 和 Pull Request。
