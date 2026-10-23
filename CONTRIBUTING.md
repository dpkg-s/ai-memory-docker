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

**本仓没有 CI**，tag 推送不会自动发布 —— Release 需要手工建：

```bash
gh release create v1.1.2 --title "v1.1.2 — <一句话>" --notes-file notes.md
```

正文只写使用者能感知的（镜像内容变没变、要不要改 compose、要不要重建卷）。纯指针上提
照实写「**本版无功能变更**，仅同步上游 3.1.3 / 1.11.4」。骨架见
`~/.workbuddy/skills/ai-memory-release-flow/references/release-body-template.md`。

## 发布前 checklist

- [ ] `git submodule status` 两级别都是干净状态（无 `-dirty`）
- [ ] `compose.yaml` 的两个 `image:` tag 与新版本一致
- [ ] Compose 未内置默认账号口令；部署凭据通过被忽略的 `.env` 提供
- [ ] Web 控制台端口默认只绑定回环地址；LAN/TLS/VPN 使用方式有明确说明
- [ ] `CHANGELOG.md` 顶部加条目
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
