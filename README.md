# ai-memory-docker

**`ai-memory` 记忆库的全部容器化定义**：MCP 服务端镜像 + Web 控制台镜像 + 一栈式编排。

容器相关的文件都在这里（`Dockerfile` / `Dockerfile.web` / `compose.yaml` / `.dockerignore`）。
Python 业务代码在子模块 `template/` 里，克隆时加 `--recurse-submodules` 会一起拉下来。

[![Release](https://img.shields.io/github/v/release/dpkg-s/ai-memory-docker?label=release&color=blue)](https://github.com/dpkg-s/ai-memory-docker/releases/latest)
[![Docker](https://img.shields.io/badge/docker-compose%20up%20--build-2496ED)](https://docs.docker.com/compose/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

---

## 两条命令起全套

只想跑、不改代码的话**不需要 clone** —— 镜像在公开的 GHCR 上，compose 会自己拉：

```bash
mkdir ai-memory && cd ai-memory
curl -O https://raw.githubusercontent.com/dpkg-s/ai-memory-docker/master/compose.yaml
docker compose up -d
```

镜像同时提供 `linux/amd64` 与 `linux/arm64`，compose 按当前机器自动选，不用手动指定。

| 服务 | 地址 | 说明 |
| --- | --- | --- |
| MCP 服务端 | <http://127.0.0.1:8700/mcp> | Streamable HTTP，26 个工具 |
| Web 控制台 | <http://127.0.0.1:8811> | 首次登录 `admin` / `password`，进去第一件事改口令 |

### 想改代码：clone 后自己构建

```bash
git clone --recurse-submodules https://github.com/dpkg-s/ai-memory-docker.git
cd ai-memory-docker
docker compose up -d --build
```

**`--build` 这时必须加**。compose 里 `pull_policy: missing` 的语义是「本地没有才拉」，而本地那份
是上游镜像 —— 不加 `--build`，你改的代码不会进容器，跑的还是 GHCR 上那个。

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

### 没有构建环境：一个镜像包就够

目标机器不通外网、或者一次完整构建太慢时，不必 `git clone` —— Release 附件里有一个装好两个镜像的 `tar`：

```bash
# 从本仓 Release 下载 ai-memory-docker-<版本>.tar、同名 .sha256、compose.yaml
sha256sum -c ai-memory-docker-<版本>.tar.sha256
docker load -i ai-memory-docker-<版本>.tar
docker compose up -d --no-build
```

一个包里是两个镜像（`ai-memory:<核心仓版本>` 与 `ai-memory-web:<控制台仓版本>`），**基础层只存一份**，
所以它比「两个镜像各自导出」小得多。导入后 tag 与 `compose.yaml` 的 `image:` 一致，可直接起栈。
**`--no-build` 不能省** —— 不加它会去找源码重新构建，而离线包所在目录里并没有源码。

包名带的是**本仓的版本号**（如 `v1.1.2`），不是里面两个镜像各自的版本 —— 别拿它去比对 `image:` 的 tag，
那两个数字见 [版本对照](#版本对照)。

## Linux / SELinux：换 bind mount 前必读

默认用**具名卷**（`vault:/vault`），标签由容器引擎自己管 —— SELinux 机器上也不需要你做任何事。

但如果想直接指向一个已有的记忆库目录（`compose.yaml` 里那段被注释掉的 bind mount），
在 **RHEL / AlmaLinux / Fedora / CentOS** 这类 `getenforce` 返回 `Enforcing` 的机器上，**必须加 `:z`**：

```yaml
    volumes:
      - /srv/ai-memory:/vault:z
```

不加后缀时容器启动即 `Permission denied`，而宿主上 `ls -ld` 与属主看着全都正常 —— 拒绝来自 SELinux
而不是文件权限，`chmod` / `chown` 都救不回来。

### 别把 `:Z`（大写）用在共享目录上

卷后缀的大小写是两件完全不同的事：

| 后缀 | SELinux 效果 | 适用场合 |
| --- | --- | --- |
| `:z` | 打**共享**标签 `container_file_t:s0`，所有容器都能访问 | 同一目录被多个容器挂载 ✅ |
| `:Z` | 打**独占**标签 `container_file_t:s0:cX,cY`，只有该容器能访问 | 目录只给一个容器用 |

两个容器挂同一目录时，先挂的拿到 `c20,c288`，后挂的一用 `:Z` 就把整个目录重标成 `c169,c195`，
**先挂的那个立刻读不到**，而两边的 `podman ps` / `docker ps` 全显示正常。

> 真实案例：给本栈加 fns-sync 时误用了 `:Z`，MCP 当场 `Permission denied: '/vault/memory.toml'`，
> 进容器 `ls -ld /vault` 却一切正常。唯一的判据是比对标签，而不是权限位：
>
> ```bash
> ls -Zd /srv/ai-memory
> podman inspect <容器名> --format '{{.ProcessLabel}}'
> ```
>
> 目录与两个容器的 MCS 类别（`s0:cX,cY`）不一致，就是这个坑。

用默认的具名卷可以完全绕开这一整套。除非有明确理由（已经是现成数据目录、要挂本机块设备），否则别换。

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

## 可选：接 FNS 做多端分发

记忆库是本地目录，只有开着那台机器时才看得到。想让手机、平板上的 Obsidian 读到同一份内容，
可以在本栈旁边加一个 [Fast Note Sync Service](https://github.com/haierkeys/fast-note-sync-service)
（FNS）同步客户端。**同步客户端已经随本仓提供**（`fns-sync/`，含源码与构建文件），但默认不启动 ——
不接 FNS 的人不需要它。服务端（FNS 本体）仍要你自己有一个，本仓只负责「这台机器怎么跟它同步」。

```
上行：AI 客户端 → ai-memory 写 /vault → fns-sync 监听 → 上传 FNS 服务端 → 各端 Obsidian
下行：FNS 服务端 → fns-sync 写 /vault → ai-memory 索引后即可被 AI 读到
```

**两个方向都要 /vault 可写**。只挂 `:ro` 时上行照常，但服务端认为「客户端没有的笔记」会下发
`NoteSyncModify`，写入必然 `read-only file system` —— 表现就是「FNS 里明明有 400 多篇，容器里
一篇都不落地」。只要你要「别的设备/网页端新建的笔记也能进这台机器」，就必须给写权限。

### 启用内置的 fns-sync

**不需要自己找镜像**：`fns-sync/` 里已经带了 go-fast-note-sync 的源码、Dockerfile 与入口包装
（上游没有官方镜像，这也是为什么以前这里只能写「填你的镜像」）。一条命令启用：

```bash
# 首次会编译 Go 二进制，约需 1–3 分钟；之后走构建缓存
docker compose --profile fns up -d --build
```

它在 compose 里带 `profiles: ["fns"]`，所以普通的 `up -d` 不会拉起它。起来之后 `/vault` 与远端
双向同步：AI 写入的笔记会推上去，别的设备新建的会落下来、被索引后即可被 AI 读到。

配置有两条路，**选一条**：

#### ① 控制台接管（推荐）

在控制台「配置 → FNS 同步」里填服务地址、令牌、vault 名，容器每 15 秒向控制台要一次渲染好的
`config.yaml`，改了自动重载。需要两边共享同一个内部密钥：

```bash
# .env
INTERNAL_KEY=<自己生成一串随机值>
FNS_CONFIG_REMOTE=http://ai-memory-web:8811/internal/fns/config.yaml
```

并把同一个 `INTERNAL_KEY` 作为 `AI_MEMORY_WEB_INTERNAL_KEY` 传给控制台容器 —— 最省事的做法是写进
`compose.override.yaml`（该文件已被 `.gitignore`，不会误提交）：

```yaml
services:
  ai-memory-web:
    environment:
      AI_MEMORY_WEB_INTERNAL_KEY: ${INTERNAL_KEY}
```

#### ② 环境变量（最简）

不配控制台，直接给两个变量，用镜像内自带的模板跑：

```bash
# .env
FNS_API=https://your-fns-host:58889
FNS_TOKEN=eyJhbGciOi...
```

此时 `FNS_CONFIG_REMOTE` 留空，容器不做任何拉取，启动即用模板。

> 两种方式下 `client_type` 都必须与令牌的 `c:` 维度一致（`GoFastNoteSync` / `ObsidianPlugin` /
> `mobile` …）。不一致的握手会直接 `315`（无权限），而不是报「连不上」。拿不准就在控制台点一次
> 「测试连接」—— 它会逐个候选试出真正命中的那个。

### 启用后要留意的三条红线

- **`/vault` 必须是本地块设备**，不能换成 SMB / NFS 挂载 —— SQLite 的 WAL 与文件锁在网络文件系统上
  行为不确定。
- **8700 仍然只绑回环**。`fns-sync` 走 compose 内部网络拿配置，不需要把 MCP 端口开出去。
- **写者从「一个」变成「两个」，要选冲突策略**。开了下行之后，AI 与 FNS 服务端都能改同一个
  `.md`。客户端默认 `offline_sync_strategy: ""`（与服务端官方客户端一致）：内容不一致时以
  mtime 新的一方为准；同区域同时改则由服务端合并并**另存一份冲突文件**，正文不丢。
  想更保守可改 `manualMerge`（命中冲突就拦下来交给人工），代价是会卡住自动同步。
  另外别让两台机器共用同一个 vault 名各自当写者 —— 那才是真正会互相覆盖的场景。

### 两种令牌，用途不同

接 FNS 会碰到两类令牌，别混：

| 令牌 | 出现位置 | 用途 |
| --- | --- | --- |
| API 令牌 | 请求头 `Authorization: Bearer <JWT>` | 同步客户端与脚本调用。**用户级**，一个令牌可操作多个 vault（vault 由客户端指定），吊销后返回 `code=308` |
| 分享令牌 | URL 路径里 `/share/{id}/{token}` | 把单篇笔记以只读链接发给别人看，免登录 |

分享令牌那串约 60 字符的值才是钥匙 —— 前面的数字 `id` 是笔记自增主键，可枚举但没令牌无效。
它可设密码与有效期，服务端会记录访问次数与最后访问时间。**链接一旦发出就等于把内容交出去了**，
没有二次校验，所以涉密笔记要么不分享，要么必带有效期 + 密码。

### 同步方向（只读 / 只写 / 读写）

控制台「配置 → FNS 同步 → 同步方向」三选一，直接决定容器里 fns-sync 被允许走哪一侧：

| 选项 | `sync_mode` | 上行（本地 → 服务端） | 下行（服务端 → 本地） | 典型场合 |
| --- | --- | --- | --- | --- |
| 读写（双向） | `rw` | ✅ | ✅ | 默认；这台机器既产生内容也接收内容 |
| 只读（只接收） | `ro` | ❌ | ✅ | 只想看别的设备写的东西，本机 AI 的改写不许外流 |
| 只写（只上传） | `wo` | ✅ | ❌ | 本机是唯一写者，不允许任何外部内容回灌 |

三个细节值得先知道：

- **语义是「掐掉哪一侧」，不是「谁有权限」**。`ro` 掐上行、`wo` 掐下行，两者不重叠。
- **`wo` 需要客户端镜像支持 `sync_mode`**（本次改动引入，镜像必须重新构建才会生效）。
  旧镜像不认识这个键，会按读写跑 —— 所以升级顺序是「先换镜像，再改方向」。
- **`ro` 用旧键也能生效**：控制台对 `ro` 会同时下发 `readonly_sync_enabled: true`，
  老镜像里它只挡上行，语义恰好一致。

> 别用「把 `/vault` 挂成 `:ro`」来实现只写：那只是让写入失败，服务器该推还是推，
> 日志里一行 `read-only file system` 也不报错，属于既不生效也查不出原因的做法。

### 排除规则

镜像式同步把「服务端不存在的路径」当作已删除，vault 里除了笔记不应放别的东西。控制台下发的
配置默认排除 `.git`、`.obsidian`（其 `data.json` 含明文 JWT）、`.trash` 与代码副本目录，
扩展名层面排除 `.db`、`.json`、`.tmp`、`.lock`、`.syncbak`。要加规则就在控制台的 `exclude` 里填。

> 具体字段、体检码与下发机制见 [ai-memory-web](https://github.com/dpkg-s/ai-memory-web) 的
> 「配置页 · FNS 同步」一节。

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
