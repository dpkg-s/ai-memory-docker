package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sync_mode（2026-11-04）把「掐掉哪一侧」变成可切换的三态：
//
//	rw 双向 / ro 只接收（掐上行）/ wo 只上传（掐下行）
//
// 三条口径必须钉死，否则控制台那个下拉只是装饰：
//  1. 缺省 rw —— 旧配置没有这个键时，同步方向不许发生漂移；
//  2. ro 必须等价于旧的 readonly_sync_enabled=true（只挡上行）；
//  3. 拼错的取值必须**报错**，不能静默回落 rw —— 那会把「只写」悄悄变成双向。

func writeModeConfig(t *testing.T, extra string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := "api: \"http://test.example.com\"\n" + extra
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestDefaultSyncModeIsReadWrite(t *testing.T) {
	if got := Default().SyncMode; got != SyncModeReadWrite {
		t.Fatalf("default sync_mode = %q, want %q", got, SyncModeReadWrite)
	}
}

func TestLoadDefaultsToReadWriteWhenKeyAbsent(t *testing.T) {
	cfg, err := Load(writeModeConfig(t, ""))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SyncMode != SyncModeReadWrite {
		t.Errorf("sync_mode = %q, want rw", cfg.SyncMode)
	}
	if cfg.ReadOnlySyncEnabled {
		t.Error("readonly_sync_enabled must stay false under rw")
	}
	if !cfg.DownlinkApplyEnabled() {
		t.Error("rw must apply downloads")
	}
}

func TestLegacyReadonlyToggleMapsToRO(t *testing.T) {
	cfg, err := Load(writeModeConfig(t, "readonly_sync_enabled: true\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SyncMode != SyncModeReadOnly {
		t.Errorf("sync_mode = %q, want ro", cfg.SyncMode)
	}
	if !cfg.ReadOnlySyncEnabled {
		t.Error("legacy toggle must survive as readonly_sync_enabled")
	}
	if !cfg.DownlinkApplyEnabled() {
		t.Error("ro must still apply downloads; only the upload side is muted")
	}
}

func TestWriteOnlyMutesDownloadSideOnly(t *testing.T) {
	cfg, err := Load(writeModeConfig(t, "sync_mode: \"wo\"\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.ReadOnlySyncEnabled; got {
		t.Error("wo must keep the upload side enabled (readonly_sync_enabled=false)")
	}
	if cfg.DownlinkApplyEnabled() {
		t.Error("wo must mute the download side")
	}
}

func TestReadOnlyModeKeepsDownloadSide(t *testing.T) {
	cfg, err := Load(writeModeConfig(t, "sync_mode: \"ro\"\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.ReadOnlySyncEnabled {
		t.Error("ro must set readonly_sync_enabled so upload paths stay guarded")
	}
	if !cfg.DownlinkApplyEnabled() {
		t.Error("ro must not mute the download side")
	}
}

func TestSyncModeOverridesLegacyToggle(t *testing.T) {
	// sync_mode 是唯一真值：显式 rw + 旧键 true 时必须按 rw 走，
	// 否则「改了方向却还是只读」这种前后矛盾会让人查很久。
	cfg, err := Load(writeModeConfig(t, "sync_mode: \"rw\"\nreadonly_sync_enabled: true\n"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.SyncMode != SyncModeReadWrite || cfg.ReadOnlySyncEnabled {
		t.Errorf("got mode=%q readonly=%v, want rw/false", cfg.SyncMode, cfg.ReadOnlySyncEnabled)
	}
}

func TestUnknownSyncModeIsRejected(t *testing.T) {
	_, err := Load(writeModeConfig(t, "sync_mode: \"ow\"\n"))
	if err == nil {
		t.Fatal("expected Load to reject an unrecognized sync_mode")
	}
	for _, want := range []string{"sync_mode", "rw", "ro", "wo"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error should mention %q, got: %v", want, err)
		}
	}
}

func TestDownlinkApplyEnabledNilSafe(t *testing.T) {
	var c *Config
	if !c.DownlinkApplyEnabled() {
		t.Error("nil config must behave as rw (downloads applied)")
	}
}
