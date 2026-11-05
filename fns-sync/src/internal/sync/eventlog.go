package sync

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	stdsync "sync"
	"time"
)

/* 同步事件日志（2026-11-05，「记忆库 Web 控制台 → 日志页」的数据面）。
 *
 * 为什么在这里落盘，而不是靠控制台去解析 stdout
 * --------------------------------------------------
 * 控制台容器拿不到本容器的 stdout（没有 docker.sock，也刻意不给），
 * 而 `log.Printf` 的文本是「给人 grep 的」：字段靠正则切、时间只有前缀、
 * 一改日志文案就静默错位。所以这里落 **JSONL**：一行一个事件，
 * 追加即持久，控制台读时直接 `json.loads` 得结构。
 *
 * 三条硬约束（与 web_audit.py 同一取向）
 * --------------------------------------
 * 1. **append-only**：只追加，不回头改写历史行。
 * 2. **不落内容**：只写「哪个路径、什么动作、多大」，正文/哈希一律不写 ——
 *    日志是旁证，不该在别处复制一份更疏于保护的数据。
 * 3. **有界**：超过 maxBytes 压到最近 keep 条。同步是长期高频事件，
 *    无界增长迟早把磁盘和「读最近 N 条」的延迟一起拖垮。
 *
 * 路径：默认与 state.json 同目录（容器里是 /state 卷），可用
 * `FNS_SYNC_EVENTS` 覆盖；`FNS_SYNC_EVENTS=off|none|-` 关闭。
 * 关闭开关是给「不想多一个文件」与排障对拍用的，不是常规形态。
 */

const (
	// 1 MiB ≈ 一万余条事件（单条 80~120 字节），够回看数周；
	// 再大没有收益：控制台只取最近几百条（界面）与全量（导出）都在这之内。
	defaultEventMaxBytes = 1 << 20
	// 压缩后保留条数：与 web_audit 的 keep_entries 同量级，
	// 保证「导出全量」不会因为压缩而突然少一个数量级。
	defaultEventKeep = 2000
)

// SyncEvent 是写进事件日志的一行。
//
// 字段刻意收窄：控制台要回答的是「什么时候、动了哪条笔记/文件、什么方向」，
// 其余（大小、备注）是帮助判断的旁证。
type SyncEvent struct {
	Time   int64  `json:"t"`               // unix 毫秒（与 Go 侧一致；控制台转秒）
	Kind   string `json:"kind"`            // note / file / setting / folder
	Action string `json:"action"`          // upload / download / modify / delete / rename
	Path   string `json:"path"`            // vault 相对路径（正斜杠，可直接反推标题）
	Title  string `json:"title,omitempty"` // 笔记标题（path 的 stem），方便前端直接打开
	Size   int64  `json:"size,omitempty"`  // 字节数；未知为 0
	Detail string `json:"detail,omitempty"`
}

// SyncEventLog 一条有界、追加式的事件流。
type SyncEventLog struct {
	path     string
	maxBytes int64
	keep     int
	mu       stdsync.Mutex
}

// NewSyncEventLog 按路径建事件流。path 为空返回 nil —— 调用方据此整体跳过，
// 免得每处接线都写一遍 `if enabled`。
func NewSyncEventLog(path string) *SyncEventLog {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	return &SyncEventLog{path: path, maxBytes: defaultEventMaxBytes, keep: defaultEventKeep}
}

// Path 返回事件文件路径（空表示未启用）。控制台侧只做只读挂载，不需要这条，
// 但测试与运维脚本要能自证「写到了哪」。
func (l *SyncEventLog) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// Append 追加一条。**永不 panic、永不返回错误**：同步是业务，日志是旁证，
// 写盘失败（磁盘满 / 卷只读）不该让同步跟着失败。代价是「日志有洞」，
// 收益是「同步不因日志而停」—— 这个取舍在本场景是明确的。
func (l *SyncEventLog) Append(ev SyncEvent) {
	if l == nil {
		return
	}
	if ev.Time <= 0 {
		ev.Time = time.Now().UnixMilli()
	}
	if ev.Path != "" {
		ev.Path = strings.ReplaceAll(ev.Path, "\\", "/")
	}
	if ev.Title == "" && ev.Path != "" {
		ev.Title = noteTitleFromRel(ev.Path)
	}
	line, err := json.Marshal(ev)
	if err != nil {
		log.Printf("[events] marshal: %v", err)
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		log.Printf("[events] mkdir %q: %v", filepath.Dir(l.path), err)
		return
	}
	fh, err := os.OpenFile(l.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("[events] open %q: %v", l.path, err)
		return
	}
	_, werr := fh.Write(append(line, '\n'))
	cerr := fh.Close()
	if werr != nil {
		log.Printf("[events] write %q: %v", l.path, werr)
		return
	}
	if cerr != nil {
		log.Printf("[events] close %q: %v", l.path, cerr)
		return
	}
	if info, err := os.Stat(l.path); err == nil && info.Size() > l.maxBytes {
		l.compact()
	}
}

// compact 压缩到最近 keep 条。调用方须已持锁。
//
// 用「读全部 → 留尾部 → 原子替换」而不是滚动分文件：本场景日增量是几十到
// 几百条量级，分文件只会制造一堆小文件反而更难查；真到了按天归档的量级，
// 换成 `sync-events-YYYYMMDD.jsonl` 是一行改动。
func (l *SyncEventLog) compact() {
	raw, err := os.ReadFile(l.path)
	if err != nil {
		log.Printf("[events] compact read %q: %v", l.path, err)
		return
	}
	lines := make([]string, 0, 64)
	for _, line := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) <= l.keep {
		return
	}
	kept := lines[len(lines)-l.keep:]
	if err := atomicWriteFile(l.path, []byte(strings.Join(kept, "\n")+"\n")); err != nil {
		log.Printf("[events] compact write %q: %v", l.path, err)
	}
}

// noteTitleFromRel 把 vault 相对路径换成条目标题（文件 stem）。
//
// 与核心仓的落点规则一致：frontmatter 里只存叶子名，目录只由路径表达，
// 所以「path 的 stem」就是控制台 `memory_read(title=…)` 能打开的那个名字。
func noteTitleFromRel(rel string) string {
	rel = strings.TrimSpace(strings.ReplaceAll(rel, "\\", "/"))
	if rel == "" {
		return ""
	}
	base := rel
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	if strings.HasSuffix(strings.ToLower(base), ".md") {
		base = base[:len(base)-3]
	}
	return base
}

// eventLogPathFromState 由 state.json 的位置推事件日志的位置。
//
// 为什么跟着 state 走：两者都是「本容器自己的运行事实」，落在同一个可写卷
// （/state）里，重建容器不丢；并且控制台只需要多挂这一个卷就能读。
// statePath 为空（部分单测）时返回空 ⇒ 整体关闭，避免往测试当前目录写文件。
func eventLogPathFromState(statePath string) string {
	statePath = strings.TrimSpace(statePath)
	if statePath == "" {
		return ""
	}
	dir := filepath.Dir(statePath)
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "sync-events.jsonl")
}

// eventLogPath 解析最终路径：环境变量优先，其次随 state.json 走。
// 返回空 = 不启用。
func eventLogPath(statePath string) string {
	raw := strings.TrimSpace(os.Getenv("FNS_SYNC_EVENTS"))
	switch strings.ToLower(raw) {
	case "off", "none", "-", "0", "false":
		return ""
	case "":
		return eventLogPathFromState(statePath)
	default:
		return raw
	}
}

// syncEvent 是 SyncService 侧的便捷入口：s 或 events 未就绪时静默跳过。
//
// 每个接线点只调这一行，避免在 handler 里散落 `if s.events != nil` 判断 ——
// 那种写法漏一处就少一类事件，而且很难被发现。
func (s *SyncService) syncEvent(kind, action, rel string, size int64, detail string) {
	if s == nil || s.events == nil {
		return
	}
	s.events.Append(SyncEvent{
		Kind:   kind,
		Action: action,
		Path:   rel,
		Size:   size,
		Detail: detail,
	})
}
