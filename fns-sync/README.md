# fns-sync —— FNS 同步客户端

[Fast Note Sync Service](https://github.com/haierkeys/fast-note-sync-service) 的官方 Go 客户端
（[go-fast-note-sync](https://github.com/erichll/go-fast-note-sync)）的容器化包装。
作用是把本栈的 `/vault` 与远端的 FNS 服务端双向同步，让手机/平板上的 Obsidian 也能读到同一份记忆。

**它默认不启动**：不接 FNS 的人不需要它。启用见主 README 的「多端分发」一节：

```bash
docker compose --profile fns up -d --build
```

## 这个目录为什么存在

上游没有发布官方镜像。如果只给一句「image: 填你的 fns-sync 客户端镜像」，接 FNS 的人会直接卡死 ——
所以本仓把**源码（`src/`，go-fast-note-sync 上游快照）+ 构建文件**一起带上，不需要任何外部镜像。

源码约 750 KB，构建需要联网拉 Go 依赖（Dockerfile 里 GOPROXY 默认走 goproxy.cn）。

## 文件

| 文件 | 作用 |
|---|---|
| `Dockerfile` | 两阶段构建：golang 编译静态二进制 → alpine 运行，uid/gid 固定 10001 |
| `entrypoint.sh` | 启动前向控制台拉配置（拿不到回退模板）；运行期比对 sha，变了就退出重载 |
| `config.yaml.example` | 兜底配置模板，已 COPY 进镜像的 `/config/config.yaml` |
| `src/` | go-fast-note-sync 上游源码快照（离线可构建） |

## 配置的三条路径

1. **控制台接管（推荐）**：控制台「配置 → FNS 同步」填好 → 容器反向拉取渲染好的 YAML。
   需要两边共享同一个 `AI_MEMORY_WEB_INTERNAL_KEY`。
2. **环境变量**：compose 里给 `FNS_API` / `FNS_TOKEN`，用镜像内模板直接跑。
3. **挂载覆盖**：把自己改好的 `config.yaml` 挂到 `/config/config.yaml`。

## 避坑

- `client_type` 必须与令牌的 `c:` 维度一致，否则 315。不确定就在控制台点「测试连接」自动探测。
- `/vault` 必须可写，且**不能**挂 `:ro` —— 下行要落盘。
- SELinux 机器上如果用 bind mount 代替具名卷，记得加 `:z`（小写）；同一目录被多个容器共享时
  绝不能用 `:Z`，那会给每个容器分配独占 MCS 标签，后挂的把先挂的锁死。
- 同一个 vault 只能有一台上传源，两台同时 `rw` 会互相覆盖。
