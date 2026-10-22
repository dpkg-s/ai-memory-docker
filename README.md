# ai-memory-docker

`ai-memory` 记忆库的**全部容器化定义**：MCP 服务端镜像 + Web 控制台镜像 + 一栈式编排。

本仓只放容器相关的东西（`Dockerfile` / `Dockerfile.web` / `compose.yaml` / `.dockerignore`）。
Python 业务代码不在这里，而在 submodule 里 —— 见下面的依赖链。

## 依赖链

```
ai-memory-docker              ← 本仓：两个镜像与编排
  └── template/ (submodule)   →  dpkg-s/ai-memory-template    ← server.py 与各模块
        └── web-console/ (submodule) → dpkg-s/ai-memory-web   ← Web 控制台
```

三级链的划分理由：

| 仓 | 职责 | 变更节奏 |
| --- | --- | --- |
| `ai-memory-template` | MCP 服务端本体（`server.py` + 叶子模块）、安装器，以及随仓分发的安装器 exe | 业务迭代，最频繁 |
| `ai-memory-web` | Web 控制台（前端 + `serve_web.py` + `web_console.py`） | 界面迭代 |
| `ai-memory-docker` | 把上面两者装进容器 | 只随镜像策略变化 |

**容器定义只在两个地方**：MCP 服务端的容器化定义在本仓 `Dockerfile`，控制台的在本仓
`Dockerfile.web` —— 两个上游仓都不再持有 `Dockerfile` / `compose`，避免同一件事有两份说法。

## 两个镜像

| 镜像 | 构建文件 | 端口 | 内容 |
| --- | --- | --- | --- |
| `ai-memory:<template 版本>` | `Dockerfile` | 8700（`/mcp`） | 只要 `template/*.py`，**不需要** web-console |
| `ai-memory-web:<web 版本>` | `Dockerfile.web` | 8811 | 只要 `template/web-console/` |

两者 TAG 各自跟随上游版本，互不绑定。

## 快速开始

```bash
git clone --recurse-submodules https://github.com/dpkg-s/ai-memory-docker.git
cd ai-memory-docker
docker compose up -d --build
```

- MCP 端点：`http://127.0.0.1:8700/mcp`（Streamable HTTP）
- 控制台：`http://127.0.0.1:8811`，默认 `admin` / `password`（**首次登录后请改**）

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
- 依赖清单取 submodule 里那份 `requirements.txt`（`mcp<2` 是硬约束：2.x 把 FastMCP 更名为
  MCPServer，装到 2.x 会 import 失败），不在本仓复制第二份。

### Web 控制台

- `/config` 是唯一需要写权限的目录（`config.json` / `users.json` / `tokens.json`），声明为卷。
- 默认账号 `admin` / `password` **只是首次启动的种子**：`users.json` 为空时才建初始管理员，
  存 scrypt 哈希、磁盘上没有明文。改过口令后环境变量即失效 —— 刻意的，否则每次重启会打回默认值。
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

**用本仓的 compose 就不会撞这个问题** —— 两个服务都在同一网络里，走服务名。

## 升级

镜像里没有业务逻辑，所以升级的实质是**移动 submodule 指针**：

```bash
cd template && git fetch && git checkout v3.1.1 && cd ..
git add template
# 同步 compose.yaml 里的 image tag
git commit -m "chore: template 指向 v3.1.1"
docker compose up -d --build
```

控制台单独升级（web 仓自身也是 template 的子模块，改的是**二级指针**）：

```bash
cd template/web-console && git fetch && git checkout v1.11.2 && cd ../..
git add template
git commit -m "chore: web-console 指向 v1.11.2"
docker compose up -d --build ai-memory-web
```

## 版本号

与两个上游仓不同轨，只跟随镜像策略变化：

| 变更类型 | 增量 |
| --- | --- |
| 修 Dockerfile / compose / 文档 | `+0.0.1` |
| 新增编排能力（新镜像、新部署形态） | `+0.1` |

`compose.yaml` 的 `image:` tag 与本仓 tag 保持一致，便于反查「这个镜像跑的是哪份定义」。
