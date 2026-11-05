# syntax=docker/dockerfile:1
#
# FNS 同步客户端镜像（go-fast-note-sync）
#
# 为什么随包提供源码：上游（github.com/erichll/go-fast-note-sync）没有官方镜像，
# 而接 FNS 的人如果卡在「image: 填什么」这一步，整个多端分发就走不通。所以把
# 源码 + 入口包装一起放进本仓，`docker compose --profile fns up -d --build` 即可。
#
# 与上游 deploy/docker/Dockerfile 的差异：
#   1. 固定 uid/gid = 10001（上游 adduser -S 随机分配，与宿主 chown 10001 对不上）
#   2. 显式 GOPROXY（国内直连 proxy.golang.org 基本不通，会在 go build 卡死）
#   3. 基础镜像走 daocloud 镜像站（docker.io 在部分网络下不可达）
#   4. 镜像内自带 /config/config.yaml 兜底模板（不挂卷也能用环境变量跑起来）
#
# 构建上下文 = 本目录（需含 src/）。命令：
#   docker build -t ai-memory-fns-sync:local ./fns-sync

FROM docker.m.daocloud.io/library/golang:1.24.5-alpine AS builder
# go.mod 要求 go 1.24.5，低于此版本直接构建失败，不要降级
ARG GOPROXY=https://goproxy.cn,https://proxy.golang.org,direct
ENV GOPROXY=${GOPROXY}
WORKDIR /build
COPY src/ ./
RUN CGO_ENABLED=0 GOOS=linux go build \
      -ldflags="-w -s -X main.cliVersion=local" \
      -o go-fast-note-sync ./cmd/

FROM docker.m.daocloud.io/library/alpine:3.21
RUN apk add --no-cache ca-certificates tzdata \
 && addgroup -g 10001 appgroup \
 && adduser -D -u 10001 -G appgroup -h /home/appuser appuser \
 && mkdir -p /home/appuser /state /config \
 && chown 10001:10001 /home/appuser /state /config

COPY --from=builder --chown=10001:10001 /build/go-fast-note-sync /usr/local/bin/go-fast-note-sync

# 入口包装：启动时先向控制台拉配置（方案 C），拉不到才回退本镜像内的模板；
# 运行期比对 sha 变化即退出，由 restart 策略重载。全程不需要 docker.sock。
COPY --chown=10001:10001 entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

# 兜底模板：挂载 /config/config.yaml 可覆盖它；不挂也能靠 FNS_API/FNS_TOKEN 环境变量跑
COPY --chown=10001:10001 config.yaml.example /config/config.yaml

ENV HOME=/home/appuser
USER 10001:10001
WORKDIR /home/appuser

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
# ⚠️ 不写 --config：路径由 entrypoint 决定（远端 /state/config.yaml 优先），
#    写死在这里会让控制台下发的配置形同虚设。
CMD ["start"]
