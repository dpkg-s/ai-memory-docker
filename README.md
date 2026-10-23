# ai-memory-docker

**`ai-memory` 记忆库的全部容器化定义**：MCP 服务端镜像 + Web 控制台镜像 + 一栈式编排。

容器相关的文件都在这里（`Dockerfile` / `Dockerfile.web` / `compose.yaml` / `.dockerignore`）。
Python 业务代码在子模块 `template/` 里，克隆时加 `--recurse-submodules` 会一起拉下来。

[![Release](https://img.shields.io/github/v/release/dpkg-s/ai-memory-docker?label=release&color=blue)](https://github.com/dpkg-s/ai-memory-docker/releases/latest)
[![Docker](https://img.shields.io/badge/docker-compose%20up%20--build-2496ED)](https://docs.docker.com/compose/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

---

## 一条命令起全套

```bash
git clone --recurse-submodules https://github.com/dpkg-s/ai-memory-docker.git
cd ai-memory-docker
docker compose up -d --build
```

| 服务 | 地址 | 说明 |
| --- | --- | --- |
| MCP 服务端 | <http://127.0.0.1:8700/mcp> | Streamable HTTP，25 个工具 |
| Web 控制台 | <http://127.0.0.1:8811> | 首次登录 `admin` / `password`，进去第一件事改口令 |

**`--recurse-submodules` 不能省**：控制台代码在二级 submodule `template/web-console/` 里，
只拉一级的话 `Dockerfile.web` 会在 `COPY` 处报 not found。已有克隆补一次：

```bash
git submodule update --init --recursive
```

只想跑其中一个：

```bash
docker compose up -d --build ai-memory        # 只起 MCP
docker compose up -d --build ai-memory-web    # 只起控制台（需 MCP 已在别处）
```

停掉但保留数据：

```bash
docker compose down        # 具名卷 vault / state / am-conf 保留
```

> 首次登录账号的「种子而非配置」语义、`docker inspect` 能看到明文口令的警告，
> 见下文 [首次登录的初始账号](#首次登录的初始账号)。

## 三个仓的关系

本产品拆成三仓，各自独立发布 —— **想要容器部署的话，停在这个仓就够了**。

| 仓 | 职责 | 适用场景 |
| --- | --- | --- |
| MCP 服务端 / 安装器 | [ai-memory-template](https://github.com/dpkg-s/ai-memory-template) | 不想用容器、想装到本机 |
| Web 控制台 | [ai-memory-web](https://github.com/dpkg-s/ai-memory-web) | 想改界面或后端 |
| **容器与编排**（本仓） | 两个镜像 + 一栈式 compose | 想部署、改端口、改卷、改镜像策略 |

控制台的容器定义也在本仓（`Dockerfile.web`），所以改这个仓就能调两个服务的构建方式，
不用去上游找 Dockerfile。

## 版本对照

三仓版本号**不同轨**，各自按自己的节奏升，不要靠数字大小推断兼容性。

| 组件 | 当前版本 | 对应的镜像 |
| --- | --- | --- |
| ai-memory-template | [![v](https://img.shields.io/github/v/release/dpkg-s/ai-memory-template?label=&color=blue)](https://github.com/dpkg-s/ai-memory-template/releases/latest) | `ai-memory:<该版本>` |
| ai-memory-web | [![v](https://img.shields.io/github/v/release/dpkg-s/ai-memory-web?label=&color=blue)](https://github.com/dpkg-s/ai-memory-web/releases/latest) | `ai-memory-web:<该版本>` |
| ai-memory-docker | [![v](https://img.shields.io/github/v/release/dpkg-s/ai-memory-docker?label=&color=blue)](https://github.com/dpkg-s/ai-memory-docker/releases/latest) | 与 `compose.yaml` 里两个 `image:` 标注一致 |

两个镜像的 tag 各自跟随上游版本，互不绑定 —— 可以只升级 MCP 服务端而让控制台留在旧版。

## 两个镜像

| 镜像 | 构建文件 | 端口 | 内容 |
| --- | --- | --- | --- |
| `ai-memory:<template 版本>` | `Dockerfile` | 8700（`/mcp`） | 只要 `template/*.py`，**不需要** web-console |
| `ai-memory-web:<web 版本>` | `Dockerfile.web` | 8811 | 只要 `template/web-console/` |

### 首次登录的初始账号

| 用户名 | 口令 |
| --- | --- |
| `admin` | `password` |

打开 <http://127.0.0.1:8811> 直接用它进，进去**第一件事是改口令**（「访问控制」页）。

> 🔴 **这是「首次启动的种子」，不是常驻配置。**
> 控制台只在 `/config/users.json` 为空时用它建初始管理员，落盘的是 scrypt 哈希，
> 磁盘上没有明文。一旦改过口令，`AI_MEMORY_WEB_USER/PASSWORD` 环境变量就再也不起作用
> —— 刻意的，否则每次重启都会把口令打回默认值。改不了就删掉 `am-conf` 卷重来。
> 口令以 ENV 形式进镜像层，`docker inspect` 能看到明文，别当保险箱。

## 镜像的行为约定

### MCP 服务端

镜像里**没有数据**，只有运行时：

| 路径 | 内容 | 是否持久 |
| --- | --- | --- |
| `/vault` | 记忆库（md 条目 + frontmatter） | 必须持久，事实源 |
| `/state` | 索引 db + 指标 json | 应该持久：索引可重建，累积指标不可补 |

- **`/state` 在镜像里预建并改属主**（`memory:memory`）—— Docker 首次挂载空命名卷时会连属主
  一起从镜像目录复制；镜像里没这个目录，挂载点会是 `root:root`，非 root 进程写不进去，而
  失败症状是「索引静默回落全库扫描」而不是报错。
- 非 root 运行，uid `10001`。
- 端口只绑 `127.0.0.1:8700`，局域网看不到 —— 这是既定红线，不是配置疏漏。要从别的主机访问，
  必须显式改 `AI_MEMORY_HTTP_ALLOWED_HOSTS` 并把 ports 改成对外地址；白名单缺失时进程会
  fail-fast 拒绝启动，属有意摩擦。
- 依赖清单来自子模块里的 `requirements.txt`（`mcp<2` 是硬约束：2.x 把 FastMCP 更名为
  MCPServer，装到 2.x 会 import 失败）。

### Web 控制台

- `/config` 是唯一需要写权限的目录（`config.json` / `users.json` / `tokens.json`），声明为卷。
- 初始账号的「只是种子」语义见上一节。
- 探活打 `/api/session`，**不是** `/api/health`：开启登录后后者要 401，容器会被反复判死。
- **容器间用服务名互访**：compose 里控制台的 `AI_MEMORY_WEB_MCP` 指向
  `http://ai-memory:8700/mcp`，走内部网络，不绕宿主机回环 —— 也因此不需要
  `host.docker.internal`，不需要额外改 MCP 的 Host 白名单（`ai-memory:*` 已在里面）。

#### ⚠️ 421 Invalid Host header（只有「控制台在容器、MCP 在宿主机」时才会撞）

MCP 的 streamable HTTP transport 默认只放行 `127.0.0.1` / `localhost` / `[::1]` 作为 `Host`
（防 DNS rebinding）。实测同一 IP:端口只换 Host 头：`127.0.0.1:8700` → 200，
`host.docker.internal:8700` → **421**。TCP 是通的，是 MCP 在校验 Host。

解法在 **MCP 侧**放行：

```bash
AI_MEMORY_HTTP_ALLOWED_HOSTS="127.0.0.1:*,localhost:*,host.docker.internal:8700" \
  python serve_http.py
```

> ⚠️ 该变量是**替换**语义，不是追加：只写 `host.docker.internal:8700` 会把宿主自己的回环通道
> 一起关掉，所以必须带上 `127.0.0.1:*` 与 `localhost:*`。

`--network host` 不是解药：在 Docker Desktop for Windows/macOS 下它走的是**虚拟机**的网络命名
空间，既到不了宿主机 `127.0.0.1`，外部也访问不到容器端口（实测拒连）。

**用这里的 compose 就不会撞这个问题** —— 两个服务都在同一网络里，走服务名。

## 升级

数据都在具名卷里，升镜像不影响它 —— 但别用 `docker compose down -v`，那会把卷一起删掉。

```bash
cd ai-memory-docker
git pull                                 # compose.yaml 里的 image tag 已更新
git submodule update --init --recursive  # 同步被构建的源码
docker compose up -d --build
```

**改过 `compose.yaml`**（端口、卷路径、环境变量）的话 `git pull` 可能产生冲突。保留自己的
配置，并按本仓 [Releases](https://github.com/dpkg-s/ai-memory-docker/releases) 中对应版本，
更新 `compose.yaml` 的两个 `image:` tag。

两个镜像 tag 互不绑定，可以只升其中一个 —— 把最后一条命令换成
`docker compose up -d --build ai-memory` 即可。**回滚**就是把 tag 改回旧版本再 `up -d --build`，
数据卷不受影响。

## 许可

[MIT](LICENSE)
