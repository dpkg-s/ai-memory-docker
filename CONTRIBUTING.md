# Contributing

面向要**改镜像或编排、发版本**的人。只想把服务跑起来，看 [README.md](README.md) 就够了。

---

## 三仓拓扑

```
ai-memory-docker              ← 本仓：两个镜像与编排
  └── template/ (submodule)   →  dpkg-s/ai-memory-template    ← server.py 与各模块
        └── web-console/ (submodule) → dpkg-s/ai-memory-web   ← Web 控制台
```

本仓是**唯一的容器定义宿主**：`Dockerfile` 管 MCP 服务端，`Dockerfile.web` 管控制台。
两个上游仓都不持有 `Dockerfile` / `compose`，避免同一件事有两份说法。

划分理由：`ai-memory-template` 业务迭代最频繁，`ai-memory-web` 随界面迭代，本仓只随
镜像策略变化 —— 三仓版本号因此不同轨。

### 发布顺序是硬约束

**ai-memory-template 先 → ai-memory-web 后 → 本仓最后。**

本仓的镜像 tag 要跟随两者，且 web 仓的 Release 还要从 template `latest` 转挂安装器 exe。
本仓先发会造成「compose 指向一个上游还不存在的 tag」。

## 改动记录只写本仓

**谁改了东西，就写在谁自己的那个仓里。** 本仓只记录容器端（镜像与编排）的改动，另两端去看它们自己的记录。

| 端 | 仓库 | 谁能写进本仓的文档 |
| --- | --- | --- |
| 核心 | [ai-memory-template](https://github.com/dpkg-s/ai-memory-template) · [CONTRIBUTING](https://github.com/dpkg-s/ai-memory-template/blob/master/CONTRIBUTING.md) | MCP 服务端、索引/检索、测试、安装器 exe —— 归它自己写 |
| 外部端 | [ai-memory-web](https://github.com/dpkg-s/ai-memory-web) · [CONTRIBUTING](https://github.com/dpkg-s/ai-memory-web/blob/master/CONTRIBUTING.md) | 控制台页面 / CSS / 前端交互 —— 归它自己写 |
| **容器端（本仓）** | [ai-memory-docker](https://github.com/dpkg-s/ai-memory-docker) | `Dockerfile`、`Dockerfile.web`、`compose.yaml`、镜像 tag、两级 submodule 指针 —— 只有这些 |

执行细则：

1. `commit message`、`CHANGELOG.md`、`README.md`、PR 正文、Release 正文，**只描述本仓的改动**。
2. **不写**另两端的改动清单、对方的 commit SHA、对方的运行/验证过程、对方的发版动作。
   那些属于对方仓自己那份记录 —— 写在这里就是混写。
3. 本仓天然要涉及上游版本，写法是**指针 + 超链接**，例如：
   「`template/` → [v3.1.3](https://github.com/dpkg-s/ai-memory-template/releases/tag/v3.1.3)」，
   到此为止；上游这一版改了什么，去核心仓的
   [CHANGELOG](https://github.com/dpkg-s/ai-memory-template/blob/master/CHANGELOG.md) 看，不抄过来。
4. 一次改动横跨多端 ⇒ 三仓各一笔提交、各写一句，本仓只留本仓那一笔。
5. 指针上提是本仓的活、也归本仓记录（因为它登记在本仓的 gitlink 里），但**只记录指针走到了哪个 tag**，
   不记录上游内部实现。

提交前自查：`git diff --cached` 里出现的仓库名，只能是本仓（超链接除外）。

## 版本号规则

major 位固定不动，**只叠加**：

| 变更 | 位 |
| --- | --- |
| 改 Dockerfile / compose / 文档 | `+0.0.1` |
| 新增编排能力（新镜像、新部署形态） | `+0.1` |

## 上游发新版后：上提指针

镜像里没有业务逻辑，所以升级的实质是**移动 submodule 指针 + 改 compose 的 image tag**，
两件事必须同时做，否则 compose 引用的 tag 与实际构建的源码不是同一版。

```bash
# 1. 上提 MCP 服务端
cd template && git fetch --tags && git checkout v3.1.3 && cd ..
git add template

# 2. 同步 Dockerfile 引用的版本（如有）+ compose 的 image tag
#    compose.yaml 第 20 行：image: ai-memory:3.1.3

# 3. 只提控制台时改二级指针
cd template/web-console && git fetch --tags && git checkout v1.11.4 && cd ../..
git add template
#    compose.yaml 第 54 行：image: ai-memory-web:1.11.4

# 4. 先提交指针，再提交普通文件
git commit -m "chore: template 指向 v3.1.3"

# 5. 验证并构建
docker compose config >/dev/null && docker compose build

# 6. 打 tag 发布
git tag -a v1.1.2 -m "v1.1.2"
git push origin master --follow-tags
```

**tag 推送会自动建 Release**（`.github/workflows/release.yml`，2026-10-27 补上 —— 此前完全没有
workflow，v1.1.2 因此漏发过一次）。正文优先取 `.github/release-notes/<tag>.md`：用户视角、
随版本一起提交；该文件缺失时回退到 `CHANGELOG.md` 里对应版本的段落，再缺就写一句「本版无功能变更」。

`master` 有推送时也会兜底检查一次最新 tag（补历史漏发），另留 `workflow_dispatch` 供手工补发。
要改已发布版本的正文，手工执行：

```bash
gh release edit vX.Y.Z --notes-file .github/release-notes/vX.Y.Z.md
```

正文只写使用者能感知的（镜像内容变没变、要不要改 compose、要不要重建卷）。纯指针上提
照实写「**本版无功能变更**，仅同步上游 3.1.3 / 1.11.4」。骨架见
`~/.workbuddy/skills/ai-memory-release-flow/references/release-body-template.md`。

## 发布前 checklist

- [ ] `git submodule status` 两级别都是干净状态（无 `-dirty`）
- [ ] `compose.yaml` 的两个 `image:` tag 与新版本一致
- [ ] `CHANGELOG.md` 顶部加条目
- [ ] `.github/release-notes/<tag>.md` 已写（否则正文回退到 CHANGELOG 段落，读者会看到维护者视角的细节）
- [ ] `docker compose config` 通过（语法错误的 compose 会让用户第一步就失败）
- [ ] `docker compose build` 成功，两个镜像都能构建
- [ ] 起一遍并访问 <http://127.0.0.1:8811>，确认不是白屏

## 几条容易踩的坑

- **`git add -A` 会用子模块当前 HEAD 覆盖手工登记的 gitlink**。上提指针后必须
  `git add template`（指向新 commit），再 `git add` 普通文件。用 `-A` 会把指针和脏状态
  一起提交，或者覆盖掉你刚登记的位置。
- **本仓的 `template` 子模块里还有一级 `web-console`**：`git submodule update --init`
  不带 `--recursive` 只拉一级，`Dockerfile.web` 会在 `COPY` 处报 not found。
- **`/state` 必须在镜像里预建并改属主**：Docker 首次挂载空命名卷时会连属主一起从镜像目录
  复制。镜像里没这个目录，挂载点是 `root:root`，非 root 进程写不进去，而症状是「索引静默
  回落全库扫描」而不是报错。
- **探活打 `/api/session`，不是 `/api/health`**：开启登录后后者要 401，容器会被反复判死。
- **`AI_MEMORY_HTTP_ALLOWED_HOSTS` 是替换语义，不是追加**：只写新主机名会把宿主自己的回环
  通道一起关掉，必须带上 `127.0.0.1:*` 与 `localhost:*`。
- **`.dockerignore` 里不要排除 `template/`**：镜像的内容全靠它，排除掉会得到一个空服务。
