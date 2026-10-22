# ai-memory 容器镜像（v3 · Streamable HTTP）
#
# ⚠️ 本仓**不含业务逻辑** —— 运行时模块（server.py 与各叶子模块）来自 submodule
#   `template/`，即 dpkg-s/ai-memory-template。所以：
#     1) 克隆必须带子模块：`git clone --recursive` 或 `git submodule update --init`
#     2) 升级镜像的实质是**移动 submodule 指针**再重新 build，不是在本仓改 Python
#
# 依赖链：ai-memory-docker → (submodule) ai-memory-template → (submodule) ai-memory-web
#   镜像里只需要 MCP 服务端，故**刻意不拉 template 的 web-console 子模块**
#   （web 是独立仓，要控制台就单独起一个对着 8700 的连接）。
#
# 镜像里没有数据：记忆库（vault）与运行时状态（索引 db + 指标）一律从卷挂载进来。
# 因此升级只需换镜像 tag，数据不动。
#
# 构建：
#     docker build -t ai-memory:3.1.0 .
# 运行（只绑回环，宿主机局域网看不到该端口）：
#     docker run -d --name ai-memory -p 127.0.0.1:8700:8700 \
#         -v ai-memory-vault:/vault -v ai-memory-state:/state ai-memory:3.1.0
# 或直接用 compose.yaml。

FROM python:3.13-slim

ENV PYTHONUNBUFFERED=1 \
    PYTHONDONTWRITEBYTECODE=1 \
    PIP_DISABLE_PIP_VERSION_CHECK=1 \
    PIP_NO_CACHE_DIR=1

WORKDIR /app

# 依赖只有 mcp 一个。上界 <2 是硬约束：mcp 2.x 把 FastMCP 更名为 MCPServer，
# 装到 2.x 会 import 失败（与 template 仓 install.sh / install.ps1 的约束一致）。
# requirements.txt 取 submodule 里那份 —— 不在本仓复制第二份清单，避免两处漂移。
COPY template/requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

# 运行时模块 = template 仓根目录的 *.py（server.py + 叶子模块 + HTTP 传输层与入口）。
# 刻意用通配而非逐项列举：template 里已有显式清单，不再复制第二份，新增模块无需改本文件。
COPY template/*.py ./

# 非 root 运行。/vault 是记忆库卷的挂载点，/state 是运行时状态（索引 db + 指标）的
# 挂载点。
# 不用 --system：系统账号 uid 上限是 999，配 10001 会发警告（虽不致命）。
# 10001 而非 1000 是为了避开宿主常见用户 uid，减少 bind mount 场景的归属冲突。
#
# /state **必须在镜像里先建好并改属主**：Docker 首次挂载空命名卷时会连内容**带属主**
# 从镜像目录复制过去。镜像里没这个目录的话，挂载点会被建成 root:root，非 root 进程
# 写不了 —— 而失败症状是「索引静默回落全库扫描」，不是报错。
RUN useradd --create-home --home-dir /home/memory --uid 10001 memory \
 && mkdir -p /vault /state \
 && chown -R memory:memory /vault /app /state

USER memory

# 容器内必须监听 0.0.0.0（否则宿主机连不上）⇒ 触发 transport_http 的 fail-fast 规则，
# 必须同时给出 Host 白名单，否则进程拒绝启动。这里给的是「回环 + 容器名」的安全默认：
# 从宿主机经 127.0.0.1:8700 访问（Host: localhost:8700）可通过；改成局域网地址访问时
# 必须显式覆盖 AI_MEMORY_HTTP_ALLOWED_HOSTS —— 这是有意的摩擦，不是疏漏。
ENV AI_MEMORY_DIR=/vault \
    AI_MEMORY_STATE_DIR=/state \
    AI_MEMORY_TRANSPORT=http \
    AI_MEMORY_HTTP_HOST=0.0.0.0 \
    AI_MEMORY_HTTP_PORT=8700 \
    AI_MEMORY_HTTP_PATH=/mcp \
    AI_MEMORY_HTTP_ALLOWED_HOSTS=localhost:*,127.0.0.1:*,ai-memory:*

EXPOSE 8700
# /vault = 记忆库（事实源，必须持久）；/state = 索引 db + 指标 json。索引是纯派生
# 缓存（可重建），指标是累积趋势（不可补），两者都不该落在镜像层。声明成卷，
# 避免 run 时忘挂导致数据落在容器层。
VOLUME ["/vault", "/state"]

# 存活探针：只验监听端口在。MCP 端点是 POST + 会话语义，GET 不返回 2xx，
# 因此不去请求 /mcp（那会把「健康」误判成「故障」）。
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD python -c "import os,socket,sys;s=socket.socket();s.settimeout(3);sys.exit(1 if s.connect_ex(('127.0.0.1',int(os.environ.get('AI_MEMORY_HTTP_PORT','8700')))) else 0)"

CMD ["python", "serve_http.py"]
