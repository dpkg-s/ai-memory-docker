# Changelog

本项目的所有重要变更都记录在此文件。

格式遵循 [Keep a Changelog](https://keepachangelog.com/zh-CN/1.1.0/)，
版本号遵循 [语义化版本](https://semver.org/lang/zh-CN/)。

## 版本号规则

与两个上游仓（`ai-memory-template` / `ai-memory-web`）**不同轨** —— 本仓只管镜像与编排，
它们的业务迭代不触发本仓版本：

| 变更类型 | 增量 | 示例 |
| --- | --- | --- |
| 修 Dockerfile / compose / 文档 | `+0.0.1` | `1.1.0 → 1.1.1` |
| 新增编排能力（新镜像、新部署形态） | `+0.1` | `1.0.0 → 1.1.0` |

`compose.yaml` 里的 `image:` tag 与本仓 tag 对齐，便于反查「这个镜像跑的是哪份定义」。
镜像自身的版本跟随上游：`ai-memory:<template 版本>`、`ai-memory-web:<web 版本>`。

## [1.1.2] - 2026-10-27

> 主题：**镜像跟随上游 3.3.0 / 1.12.0**。上游这轮动了服务端的权限模型和控制台的访问控制，镜像内容因此必须重建 —— 不重建的话 `security` / `clearance` 两个新字段不会生效。

### Changed

- **`template` 子模块 → `168d7b7`** —— 服务端 `VERSION` 为 **3.3.0**。该提交比 `v3.3.0` 标签晚一步，唯一差别是把二级子模块指针前移到控制台的 v1.12.0：打 `v3.3.0` 标签时控制台尚未发版，指针只能停在 1.11.3。**指到提交而非标签是有意的** —— 若停在标签上，构建出来的控制台镜像自报 1.11.3，与 `image: ai-memory-web:1.12.0` 名实不符。
- **二级子模块 `web-console` → `v1.12.0`（`e1a9225`）** —— 控制台 `WEB_VERSION` 为 **1.12.0**。
- **`compose.yaml` 两个 `image:` tag 跟随上游** —— `ai-memory:3.3.0`、`ai-memory-web:1.12.0`。

### Note

- 升级只需重建镜像：`docker compose build`。三个数据卷（`vault` / `state` / `am-conf`）不受影响，无需迁移。
- 上游能力变化：服务端新增条目级 `security` / `clearance` 字段与端点分档（`/mcp`、`/mcp/member`、`/mcp/guest`）；控制台新增只读 GET 调用与访问日志卡片。对使用者而言可感知的动作仍然只有「重建」。

## [1.1.1] - 2026-10-22

> 主题：**初始账号摆在门口**。1.1.0 的 README 把 `admin` / `password` 埋在「快速开始」的一条要点里，实际看仓的人会直接滚到 `docker compose up` 就动手 —— 然后卡在登录页翻文档。

### Changed

- **README「快速开始」新增 `### 首次登录的初始账号` 小节** —— 账号口令做成表格，紧跟启动命令；「这是首次启动的种子、不是常驻配置」的语义与 `docker inspect` 能看到明文的警告一并前置。
- **删掉「Web 控制台」小节里重复的那条** —— 改为指向上面那一节，避免同一个事实两处维护。

### Note

- **`template` 子模块 → `v3.1.2`（`ba3a92b`）** —— 只是跟随上游的 CI / 文档改动，镜像内容不变，无需重新 `docker build`。

## [1.1.0] - 2026-10-22

> 主题：**容器定义只有一个家**。1.0.0 只管 MCP 服务端镜像，Web 控制台的 `Dockerfile` /
> `docker-compose.yml` 却留在 `ai-memory-web` 仓里 —— 于是「怎么跑容器」被劈成两半，
> 控制台的 Release 笔记里塞满了 `docker build`、421 陷阱、多架构坑这些本该属于本仓的内容。
> 这一版把控制台镜像与一栈式编排全部收进来，两个上游仓不再持有任何容器定义。

### Added

- **`Dockerfile.web` —— Web 控制台镜像** —— 从 `ai-memory-web` 仓迁入并改写构建上下文：原文件假定自己在控制台仓根（`COPY requirements.txt .`），现在要跨进二级 submodule，改为 `COPY template/web-console/requirements.txt` + `COPY template/web-console/ ./`。其余行为原样保留：
  - `python:3.13-slim` 基座，`pip install -r requirements.txt` 单独成层（改源码不重装 mcp）
  - 非 root 运行，uid `10001`
  - `/config` 为唯一可写目录并声明为卷
  - `EXPOSE 8811`
  - 默认账号 `admin` / `password` 写进镜像 ENV（**首次启动的种子**，见下）
- **`compose.yaml` 改为一栈式两服务** —— 原来只有一个 `ai-memory`，现在加 `ai-memory-web`，并且**用服务名互访**：控制台的 `AI_MEMORY_WEB_MCP` 指向 `http://ai-memory:8700/mcp`。走 compose 内部网络，不绕宿主机回环 ⇒ 不需要 `host.docker.internal`，也不会撞 421（`ai-memory:*` 已写进 MCP 侧 `AI_MEMORY_HTTP_ALLOWED_HOSTS`）。
- **`ai-memory` 服务补 `hostname: ai-memory`** —— Host 白名单里那条 `ai-memory:*` 靠它生效。
- **`.dockerignore` 重写** —— 两个镜像共用同一构建上下文，必须分别放行：`template/*.py` + `template/requirements.txt`（MCP 用）、`template/web-console/`（控制台用），并逐项排除 `template/web-console/` 下的 `.git` / `tests` / `*.tgz` / `Dockerfile` / `docker-compose.yml`。
- **`CHANGELOG.md`（本文件）**。

### Changed

- **README 重写** —— 从「一个镜像的说明」扩成「两个镜像 + 一栈式编排」，新增依赖链示意图、两个镜像的职责表、二级 submodule 升级法、421 的完整成因与放行方式（并注明「用本仓的 compose 就不会撞」）。
- **`compose.yaml` 镜像 tag 跟随上游** —— `ai-memory:3.1.1`、`ai-memory-web:1.11.2`。

### Note

- **默认口令是种子，不是常驻配置** —— 控制台只在 `/config/users.json` 为空时用它建初始管理员，落盘 scrypt 哈希，磁盘上没有明文。一旦在「访问控制」页改过口令，`AI_MEMORY_WEB_USER/PASSWORD` 就再也不起作用 —— 刻意的，否则每次重启都会把口令打回默认值。对外暴露前请先改掉；改不了就删掉 `am-conf` 卷重来。口令以 ENV 形式进镜像层，`docker inspect` 能看到，别当保险箱。
- **探活打 `/api/session`**（公开端点），**不是** `/api/health` —— 开启登录后后者要 401，拿它做 healthcheck 会让容器被反复判死；探针端口读 `AI_MEMORY_WEB_PORT`，改端口不会误判。
- **`--recurse-submodules` 不能省** —— 控制台代码在二级 submodule `template/web-console/` 里，只拉一级的话 `Dockerfile.web` 会在 `COPY` 处报 not found。
- **多架构坑** —— 若本机缓存的 `python:3.13-slim` 是另一种架构，构建会报 `exec /bin/sh: exec format error`；`--platform linux/amd64` 不足以让它重拉，需先 `docker pull --platform linux/amd64 python:3.13-slim`。

## [1.0.0] - 2026-10-22

> 主题：**把容器化定义从模板仓拆出来**。此前 `Dockerfile` / `docker-compose.yml` / `.dockerignore`
> 躺在 `ai-memory-template` 里，和业务代码混在一起；镜像策略一变就要动业务仓的提交历史。这一版独立成仓，只管 MCP 服务端镜像。

### Added

- **`Dockerfile` —— MCP 服务端镜像** —— 只要 `template/*.py` 与 `template/requirements.txt`（`mcp<2` 是硬约束：2.x 把 FastMCP 更名为 MCPServer，装到 2.x 会 import 失败），**不需要** `web-console`。
  - 非 root 运行，uid `10001`
  - `/state` 在镜像里**预建并改属主**（`memory:memory`）—— Docker 首次挂载空命名卷时会连属主一起从镜像目录复制；镜像里没这个目录，挂载点会是 `root:root`，非 root 进程写不进去，而失败症状是「索引静默回落全库扫描」而不是报错。
  - 端口只绑 `127.0.0.1:8700`，局域网看不到 —— 这是既定红线，不是配置疏漏。
  - 白名单缺失时进程 fail-fast 拒绝启动，属有意摩擦。
- **`compose.yaml`** —— `/vault` 与 `/state` 落具名卷，`docker compose down` 不删数据。
- **`.dockerignore`** / **`.gitattributes`** / **`README.md`**。

### Note

- **`AI_MEMORY_HTTP_ALLOWED_HOSTS` 是替换语义，不是追加** —— 只写 `host.docker.internal:8700` 会把宿主自己的回环通道一起关掉，必须带上 `127.0.0.1:*` 与 `localhost:*`。
- **`--network host` 不是访问宿主 MCP 的解药** —— 在 Docker Desktop for Windows/macOS 下它走的是**虚拟机**的网络命名空间，既到不了宿主机 `127.0.0.1`，外部也访问不到容器端口（实测拒连）。
- **421 Invalid Host header 的成因** —— MCP 的 streamable HTTP transport 默认只放行 `127.0.0.1` / `localhost` / `[::1]` 作为 `Host`（防 DNS rebinding）。实测同一 IP:端口只换 Host 头：`127.0.0.1:8700` → 200，`host.docker.internal:8700` → **421**。TCP 是通的，是 MCP 在校验 Host。解法在 MCP 侧放行，不在控制台侧。
