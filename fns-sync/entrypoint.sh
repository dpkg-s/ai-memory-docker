#!/bin/sh
# fns-sync 入口包装（2026-10-23 起升级为「配置反向拉取」，方案 C）
#
# 背景：go-fast-note-sync 只在进程启动时读一次 config.yaml，没有 SIGHUP reload。
# 以前控制台改完配置毫无反应 —— 因为两者各读各的（控制台 config.json / 容器
# config.yaml）。现在改成**容器主动向控制台要配置**，控制台是唯一真源：
#
#   ① 远端（优先）：GET ${FNS_CONFIG_REMOTE}，带 X-Internal-Key，
#      拿到的是一份渲染好的完整 YAML ⇒ 落 ${FNS_CONFIG_RUNTIME}（可写卷）再启动。
#   ② 兜底：远端拉不到（控制台没起 / 没配内部密钥 / 配置不全返 503）时，
#      用 ${FNS_CONFIG}，即镜像里那一份模板（compose 以 :ro 挂进来）。
#
# 运行期：每 POLL 秒拉一次远端，内容 sha256 与当前不同 ⇒ 退出，由 restart 策略
# 重载。这样「控制台改完 → 容器自己重启生效」，全程不需要 docker.sock
# （= 不给控制台容器控制权，等于省掉一个 socket-proxy 提权面）。
#
# 环境变量：
#   FNS_CONFIG_REMOTE     控制台配置端点；空 = 关闭远端（纯兜底模式）
#   FNS_CONFIG_KEY        控制台 X-Internal-Key
#   FNS_CONFIG            兜底模板路径，默认 /config/config.yaml
#   FNS_CONFIG_RUNTIME    远端配置落点，默认 /state/config.yaml
#   FNS_CONFIG_POLL       轮询秒数，默认 15
#   FNS_CONFIG_FETCH_TRIES / FNS_CONFIG_FETCH_WAIT  启动期重试次数/间隔（默认 20 × 3s）

set -u

REMOTE=${FNS_CONFIG_REMOTE:-}
KEY=${FNS_CONFIG_KEY:-}
FALLBACK=${FNS_CONFIG:-/config/config.yaml}
RUNTIME=${FNS_CONFIG_RUNTIME:-/state/config.yaml}
POLL=${FNS_CONFIG_POLL:-15}
TRIES=${FNS_CONFIG_FETCH_TRIES:-20}
WAIT=${FNS_CONFIG_FETCH_WAIT:-3}
CFG=""
PID=""

log() { echo "[entrypoint] $*"; }
mtime() { stat -c %Y "$1" 2>/dev/null || echo 0; }
sha() { sha256sum "$1" 2>/dev/null | cut -d' ' -f1; }

# fetch <dest>：成功返回 0。先写 .part 再 mv，避免半截文件被当成配置读走。
fetch() {
    [ -n "$REMOTE" ] || return 1
    if ! wget -q -T 6 -O "$1.part" --header="X-Internal-Key: $KEY" "$REMOTE" 2>/dev/null; then
        rm -f "$1.part"
        return 1
    fi
    # 401/503 时 wget 通常直接非 0 退出；这一步是双保险 —— 万一它把 JSON 错误体
    # 存了下来，首行不会是注释。宁可用兜底模板，也不能把错误 JSON 喂给客户端。
    if [ ! -s "$1.part" ] || ! head -1 "$1.part" | grep -q '^#'; then
        rm -f "$1.part"
        return 1
    fi
    mv "$1.part" "$1" || { rm -f "$1.part"; return 1; }
    return 0
}

# docker stop 的 SIGTERM 要转发给真正的子进程，别留孤儿
stop() {
    [ -n "$PID" ] && kill -TERM "$PID" 2>/dev/null || true
}
trap stop TERM INT

# ---- 启动期：先尽力拿远端，拿不到才回退 ----
if [ -n "$REMOTE" ]; then
    i=1
    while [ "$i" -le "$TRIES" ]; do
        if fetch "$RUNTIME"; then
            log "远端配置已就绪：$REMOTE（第 $i 次尝试，sha=$(sha "$RUNTIME" | cut -c1-12)）"
            CFG=$RUNTIME
            break
        fi
        if [ "$i" -lt "$TRIES" ]; then
            log "远端配置不可用（$i/$TRIES），${WAIT}s 后重试"
        else
            log "远端配置不可用（$i/$TRIES），已用完重试次数"
        fi
        i=$((i + 1))
        sleep "$WAIT"
    done
    if [ -z "$CFG" ]; then
        log "放弃远端，回退兜底模板：$FALLBACK"
        CFG=$FALLBACK
    fi
else
    log "未配置 FNS_CONFIG_REMOTE，使用本地模板：$FALLBACK"
    CFG=$FALLBACK
fi

if [ ! -f "$CFG" ]; then
    log "配置不存在：$CFG —— 拒绝启动（快速失败优于带空配置握手）"
    exit 1
fi

LAST=$(mtime "$CFG")
log "config=$CFG mtime=$LAST poll=${POLL}s"

go-fast-note-sync "$@" --config "$CFG" &
PID=$!

while kill -0 "$PID" 2>/dev/null; do
    # sleep 放后台再 wait，这样 trap 能在 3 秒内响应而不是卡满一轮
    sleep "$POLL" &
    SLEEP_PID=$!
    wait "$SLEEP_PID" 2>/dev/null || true

    # ① 控制台配置变了？
    if [ -n "$REMOTE" ]; then
        if fetch "$RUNTIME.probe"; then
            NEW=$(sha "$RUNTIME.probe")
            CUR=$(sha "$RUNTIME")
            if [ -n "$NEW" ] && [ "$NEW" != "$CUR" ]; then
                log "控制台配置已变更（${CUR:-空} -> $NEW），退出以便重载"
                rm -f "$RUNTIME.probe"
                stop
                wait "$PID" 2>/dev/null || true
                exit 0
            fi
            rm -f "$RUNTIME.probe"
        fi
    fi

    # ② 兜底模式下仍然认「本地模板 mtime 变化」（人工改模板的场景）
    if [ "$CFG" = "$FALLBACK" ]; then
        NOW=$(mtime "$CFG")
        if [ "$NOW" != "$LAST" ]; then
            log "本地模板已变更（$LAST -> $NOW），退出以便重载"
            stop
            wait "$PID" 2>/dev/null || true
            exit 0
        fi
    fi
done

wait "$PID" 2>/dev/null
exit $?
