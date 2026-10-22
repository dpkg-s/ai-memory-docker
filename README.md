# ai-memory-docker

`ai-memory` 记忆库 MCP 服务端的容器镜像与编排定义。

本仓**只放容器化相关的东西**：`Dockerfile`、`compose.yaml`、`.dockerignore`。
Python 业务代码不在这里，而在 submodule `template/` 里 —— 见下面的依赖链。

## 依赖链

```
ai-memory-docker          ← 本仓：镜像与编排
  └── template/ (submodule)  →  dpkg-s/ai-memory-template   ← server.py 与各模块
        └── web-console/ (submodule)  →  dpkg-s/ai-memory-web  ← Web 控制台
```

三级链的划分理由：

| 仓 | 职责 | 变更节奏 |
| --- | --- | --- |
| `ai-memory-template` | MCP 服务端本体（`server.py` + 叶子模块）、安装器，以及随仓分发的安装器 exe | 业务迭代，最频繁 |
| `ai-memory-web` | Web 控制台（前端 + `serve_web.py` + `web_console.py`） | 界面迭代 |
| `ai-memory-docker` | 把上面两者装进容器 | 只随镜像策略变化 |

镜像里**只需要 MCP 服务端**，所以本仓刻意不检出 `template/` 的 `web-console`
子模块（`git submodule update --init` 不递归即可，默认行为正好）。

## 快速开始

```bash
git clone --recurse-submodules https://github.com/dpkg-s/ai-memory-docker.git
cd ai-memory-docker
docker compose up -d --build
```

连端点：`http://127.0.0.1:8700/mcp`（Streamable HTTP）。

停掉但保留数据：

```bash
docker compose down        # 具名卷 vault / state 保留
```

已经 clone 但忘了子模块：

```bash
git submodule update --init
```

## 镜像的行为约定

镜像里**没有数据**，只有运行时：

| 路径 | 内容 | 是否持久 |
| --- | --- | --- |
| `/vault` | 记忆库（md 条目 + frontmatter） | 必须持久，事实源 |
| `/state` | 索引 db + 指标 json | 应该持久：索引可重建，累积指标不可补 |

两者都声明成 `VOLUME`，避免 `docker run` 忘挂载时数据落进容器层。

`/state` 在镜像里**预先建好并改属主**（`memory:memory`）—— Docker 首次挂载空命名卷
时会连属主一起从镜像目录复制过去；镜像里没这个目录，挂载点会是 `root:root`，
非 root 进程写不进去，而失败症状是「索引静默回落全库扫描」而不是报错。

非 root 运行，uid `10001`（避开宿主常见 uid，减少 bind mount 的归属冲突）。

端口只绑 `127.0.0.1:8700`，局域网看不到 —— 这是 v3 的既定红线，不是配置疏漏。
要从别的主机访问，必须显式改 `AI_MEMORY_HTTP_ALLOWED_HOSTS` 并把 ports 改成
对外地址；白名单缺失时进程会 fail-fast 拒绝启动，属有意摩擦。

## 升级

镜像里没有业务逻辑，所以升级的实质是**移动 submodule 指针**：

```bash
cd template && git fetch && git checkout v3.1.1 && cd ..
git add template
# 同步 compose.yaml 的 image tag
git commit -m "chore: template 指向 v3.1.1"
docker compose up -d --build
```

只用某个已发布的 tag、不想跟 master：

```bash
git -C template checkout v3.1.0
```

## 版本号

与 `ai-memory-template` 的叠加规则不同轨 —— 本仓只跟随镜像策略变化：

| 变更类型 | 增量 |
| --- | --- |
| 修 Dockerfile / compose / 文档 | `+0.0.1` |
| 新增编排能力（新服务、新部署形态） | `+0.1` |

`compose.yaml` 的 `image:` tag 与本仓 tag 保持一致，便于反查「这个镜像跑的是哪份定义」。
