package sync

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readEvents 读回事件文件的所有行（测试辅助）。
func readEvents(t *testing.T, path string) []SyncEvent {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var out []SyncEvent
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev SyncEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("bad json line %q: %v", line, err)
		}
		out = append(out, ev)
	}
	return out
}

func TestSyncEventLogAppendAndRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sync-events.jsonl")
	log := NewSyncEventLog(path)

	log.Append(SyncEvent{Kind: "note", Action: "download", Path: "工具/记忆库MCP.md", Size: 1200})
	log.Append(SyncEvent{Kind: "note", Action: "upload", Path: `Windows\坑.md`})
	log.Append(SyncEvent{Kind: "note", Action: "rename", Path: "新标题.md", Detail: "旧标题.md"})

	got := readEvents(t, path)
	if len(got) != 3 {
		t.Fatalf("want 3 events, got %d", len(got))
	}
	if got[0].Time <= 0 {
		t.Fatalf("t must be filled: %+v", got[0])
	}
	if got[0].Title != "记忆库MCP" {
		t.Fatalf("title from path: want 记忆库MCP, got %q", got[0].Title)
	}
	// 反斜杠路径必须归一：Windows 侧 watcher 给的是 \，控制台与前端按 / 处理。
	if got[1].Path != "Windows/坑.md" {
		t.Fatalf("path normalize: got %q", got[1].Path)
	}
	if got[1].Title != "坑" {
		t.Fatalf("title from windows path: got %q", got[1].Title)
	}
	if got[2].Detail != "旧标题.md" {
		t.Fatalf("rename detail lost: %+v", got[2])
	}
}

func TestSyncEventLogCompact(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sync-events.jsonl")
	log := &SyncEventLog{path: path, maxBytes: 200, keep: 3}

	for i := 0; i < 12; i++ {
		log.Append(SyncEvent{Kind: "note", Action: "modify", Path: "笔记" + string(rune('A'+i)) + ".md"})
	}
	got := readEvents(t, path)
	if len(got) != 3 {
		t.Fatalf("compact should keep 3, got %d", len(got))
	}
	// 保留的是**最近**三条（尾部），不是头部。
	if got[len(got)-1].Title != "笔记L" {
		t.Fatalf("last event should be newest, got %q", got[len(got)-1].Title)
	}
}

func TestSyncEventLogDisabled(t *testing.T) {
	var log *SyncEventLog // nil 形态
	log.Append(SyncEvent{Kind: "note", Action: "download", Path: "x.md"}) // 不得 panic

	if got := NewSyncEventLog("   "); got != nil {
		t.Fatalf("blank path must disable the log, got %+v", got)
	}
}

func TestEventLogPath(t *testing.T) {
	old, had := os.LookupEnv("FNS_SYNC_EVENTS")
	t.Cleanup(func() {
		if had {
			os.Setenv("FNS_SYNC_EVENTS", old)
		} else {
			os.Unsetenv("FNS_SYNC_EVENTS")
		}
	})

	os.Unsetenv("FNS_SYNC_EVENTS")
	got := eventLogPath(filepath.Join("C:", "state", "state.json"))
	if filepath.Base(got) != "sync-events.jsonl" {
		t.Fatalf("default path should live next to state.json, got %q", got)
	}
	// statePath 为空（部分单测）必须整体关闭，否则会往测试当前目录写文件。
	if got := eventLogPath(""); got != "" {
		t.Fatalf("empty statePath must disable, got %q", got)
	}
	os.Setenv("FNS_SYNC_EVENTS", "/custom/events.jsonl")
	if got := eventLogPath("/state/state.json"); got != "/custom/events.jsonl" {
		t.Fatalf("env override ignored: %q", got)
	}
	os.Setenv("FNS_SYNC_EVENTS", "off")
	if got := eventLogPath("/state/state.json"); got != "" {
		t.Fatalf("off must disable, got %q", got)
	}
}

func TestSyncServiceSyncEventNilSafe(t *testing.T) {
	// 零值 SyncService（events == nil）：每个接线点都会走这条路径，
	// 一旦这里 panic，同步主流程就被日志拖死。
	s := &SyncService{}
	s.syncEvent("note", "upload", "某笔记.md", 10, "")

	var nilSvc *SyncService
	nilSvc.syncEvent("note", "upload", "某笔记.md", 10, "")
}

func TestNoteTitleFromRel(t *testing.T) {
	cases := map[string]string{
		"":                          "",
		"标题.md":                     "标题",
		"目录/子目录/标题.md":              "标题",
		"目录/子目录/标题.MD":             "标题",
		`Windows\目录\标题.md`:          "标题",
		"无扩展名":                      "无扩展名",
	}
	for in, want := range cases {
		if got := noteTitleFromRel(in); got != want {
			t.Errorf("noteTitleFromRel(%q) = %q, want %q", in, got, want)
		}
	}
}
