package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
)

// DefaultClientType is the default client identifier sent to the server via the
// WebSocket URL `client=` parameter and the ClientInfo `type` field.
const DefaultClientType = "GoFastNoteSync"

// Sync-direction modes (2026-11-04).
//
// A mode selects which side is **muted**, not who is trusted:
//
//	rw - both directions apply (historical behaviour, the default)
//	ro - upload side muted: local edits never reach the server; downloads apply
//	wo - download side muted: server-pushed notes are never written to the vault
//
// ro is exactly the meaning the pre-existing readonly_sync_enabled toggle always
// had (it only guards the upload paths), so it stays supported as a legacy alias;
// wo is new and has no equivalent flag.
const (
	SyncModeReadWrite = "rw"
	SyncModeReadOnly  = "ro"
	SyncModeWriteOnly = "wo"
)

type Config struct {
	API      string `yaml:"api"       mapstructure:"api"`
	APIToken string `yaml:"api_token"  mapstructure:"api_token"`

	Vault     string `yaml:"vault"      mapstructure:"vault"`
	VaultPath string `yaml:"vault_path" mapstructure:"vault_path"`

	// ClientType identifies the client to the server. Defaults to DefaultClientType
	// ("GoFastNoteSync"). Override if the server token scope restricts `c:<value>`.
	ClientType string `yaml:"client_type" mapstructure:"client_type"`

	SyncEnabled       bool `yaml:"sync_enabled"        mapstructure:"sync_enabled"`
	ConfigSyncEnabled bool `yaml:"config_sync_enabled" mapstructure:"config_sync_enabled"`

	OfflineDeleteSyncEnabled bool   `yaml:"offline_delete_sync_enabled" mapstructure:"offline_delete_sync_enabled"`
	ReadOnlySyncEnabled      bool   `yaml:"readonly_sync_enabled"       mapstructure:"readonly_sync_enabled"`
	ManualSyncEnabled        bool   `yaml:"manual_sync_enabled"         mapstructure:"manual_sync_enabled"`
	OfflineSyncStrategy      string `yaml:"offline_sync_strategy"       mapstructure:"offline_sync_strategy"`
	SyncUpdateDelay          int    `yaml:"sync_update_delay"           mapstructure:"sync_update_delay"`
	BinarySyncLimitEnabled   bool   `yaml:"binary_sync_limit_enabled"   mapstructure:"binary_sync_limit_enabled"`

	// SyncMode is the authoritative direction switch: rw / ro / wo. ReadOnlySyncEnabled
	// is reconciled from it on load, so the upload-side guards keep reading one field.
	SyncMode string `yaml:"sync_mode" mapstructure:"sync_mode"`

	ConcurrencyControlEnabled bool `yaml:"concurrency_control_enabled" mapstructure:"concurrency_control_enabled"`
	MaxConcurrentUploads      int  `yaml:"max_concurrent_uploads"      mapstructure:"max_concurrent_uploads"`

	SyncExcludeFolders    []string `yaml:"sync_exclude_folders"     mapstructure:"sync_exclude_folders"`
	SyncExcludeExtensions []string `yaml:"sync_exclude_extensions"  mapstructure:"sync_exclude_extensions"`
	SyncExcludeWhitelist  []string `yaml:"sync_exclude_whitelist"   mapstructure:"sync_exclude_whitelist"`
	ConfigSyncOtherDirs   []string `yaml:"config_sync_other_dirs"   mapstructure:"config_sync_other_dirs"`

	StartupDelay        int    `yaml:"startup_delay"         mapstructure:"startup_delay"`
	AutoRedirectEnabled bool   `yaml:"auto_redirect_enabled" mapstructure:"auto_redirect_enabled"`
	StateFile           string `yaml:"state_file"            mapstructure:"state_file"`

	// SyncTimeoutSeconds bounds how long a sync round waits for all
	// *SyncEnd handlers + acks before force-completing. Defaults to 60s in
	// the daemon (`internal/sync.runCheckSyncCompletion`); set higher when
	// the vault is large enough that a fresh client cannot consume the
	// server's initial-sync payload within the default window. 0 keeps the
	// 60s default.
	SyncTimeoutSeconds int `yaml:"sync_timeout_seconds" mapstructure:"sync_timeout_seconds"`
}

func Default() *Config {
	return &Config{
		API:                      "",
		APIToken:                 "",
		Vault:                    "",
		VaultPath:                "",
		ClientType:               DefaultClientType,
		SyncEnabled:              true,
		ConfigSyncEnabled:        true,
		OfflineDeleteSyncEnabled: false,
		ReadOnlySyncEnabled:      false,
		SyncMode:                 SyncModeReadWrite,
		ManualSyncEnabled:        false,
		// Empty matches official client 2.4.0 / service 3.6.0 default behaviour.
		OfflineSyncStrategy:       "",
		SyncUpdateDelay:           500,
		BinarySyncLimitEnabled:    true,
		ConcurrencyControlEnabled: true,
		MaxConcurrentUploads:      3,
		SyncExcludeFolders:        []string{},
		SyncExcludeExtensions:     []string{},
		SyncExcludeWhitelist:      []string{},
		ConfigSyncOtherDirs:       []string{},
		StartupDelay:              0,
		AutoRedirectEnabled:       true,
		StateFile:                 "",
		SyncTimeoutSeconds:        0,
	}
}

func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "config.yaml"
	}
	return filepath.Join(home, ".config", "go-fast-note-sync", "config.yaml")
}

func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	expanded, err := expandEnv(string(raw))
	if err != nil {
		return nil, fmt.Errorf("expand env in %s: %w", path, err)
	}

	v := viper.New()
	v.SetConfigType("yaml")
	setDefaults(v)

	if err := v.ReadConfig(bytes.NewReader([]byte(expanded))); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	strategy, err := normalizeOfflineSyncStrategy(cfg.OfflineSyncStrategy)
	if err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	cfg.OfflineSyncStrategy = strategy

	mode, err := normalizeSyncMode(cfg.SyncMode, cfg.ReadOnlySyncEnabled)
	if err != nil {
		return nil, fmt.Errorf("config %s: %w", path, err)
	}
	cfg.SyncMode = mode
	// Derived, never independent: every upload-side guard in the sync package reads
	// ReadOnlySyncEnabled, so folding the mode into it here keeps those guards
	// correct without touching them.
	cfg.ReadOnlySyncEnabled = mode == SyncModeReadOnly
	return &cfg, nil
}

// normalizeSyncMode validates sync_mode and folds the legacy readonly toggle into
// it. Unknown values are **rejected** rather than defaulted: a typo like "ow"
// would otherwise silently mean rw and quietly re-enable the download side, which
// is the one direction an operator picks wo specifically to stop.
func normalizeSyncMode(mode string, legacyReadOnly bool) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	switch normalized {
	case "":
		if legacyReadOnly {
			return SyncModeReadOnly, nil
		}
		return SyncModeReadWrite, nil
	case SyncModeReadWrite, SyncModeReadOnly, SyncModeWriteOnly:
		return normalized, nil
	}
	return "", fmt.Errorf("sync_mode %q is not recognized; valid values are rw, ro, wo", mode)
}

// DownlinkApplyEnabled reports whether server-pushed changes may be written into
// the vault. rw and ro both apply them; only wo (upload-only) suppresses them.
// A nil config means "no restriction" so callers never have to nil-check.
func (c *Config) DownlinkApplyEnabled() bool {
	return c == nil || c.SyncMode != SyncModeWriteOnly
}

// normalizeOfflineSyncStrategy validates the ClientInfo offlineSyncStrategy
// value. The service recognizes only "", manualMerge, ignoreTimeMerge, and
// newTimeMerge. Unrecognized values disable conflict handling silently on the
// server, so they are rejected here. Legacy "auto" is normalized to empty for
// upgrade compatibility with older generated configs.
func normalizeOfflineSyncStrategy(strategy string) (string, error) {
	if strategy == "auto" {
		return "", nil
	}
	switch strategy {
	case "", "manualMerge", "ignoreTimeMerge", "newTimeMerge":
		return strategy, nil
	}
	return "", fmt.Errorf("offline_sync_strategy %q is not recognized by the sync service; "+
		"valid values are empty, manualMerge, ignoreTimeMerge, newTimeMerge",
		strategy)
}

// envRefRegexp matches ${VAR} references where VAR is a POSIX-shell-like identifier.
// Only the braced form is expanded; bare $VAR is left alone to avoid surprises with
// tokens or URLs that legitimately contain '$'.
var envRefRegexp = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// expandEnv replaces every ${VAR} occurrence with os.Getenv(VAR).
// Returns an error listing any referenced variables that are unset, so typos
// like ${SYNC_TONK} fail loudly rather than silently producing an empty value.
func expandEnv(s string) (string, error) {
	var missing []string
	seen := make(map[string]struct{})
	out := envRefRegexp.ReplaceAllStringFunc(s, func(match string) string {
		name := match[2 : len(match)-1]
		val, ok := os.LookupEnv(name)
		if !ok {
			if _, dup := seen[name]; !dup {
				seen[name] = struct{}{}
				missing = append(missing, name)
			}
			return match
		}
		return val
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("undefined env var(s): %s", strings.Join(missing, ", "))
	}
	return out, nil
}

func WriteDefault(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create config file: %w", err)
	}
	defer f.Close()

	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	if err := enc.Encode(Default()); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return enc.Close()
}

func setDefaults(v *viper.Viper) {
	d := Default()
	v.SetDefault("client_type", d.ClientType)
	v.SetDefault("sync_enabled", d.SyncEnabled)
	v.SetDefault("config_sync_enabled", d.ConfigSyncEnabled)
	v.SetDefault("offline_delete_sync_enabled", d.OfflineDeleteSyncEnabled)
	v.SetDefault("readonly_sync_enabled", d.ReadOnlySyncEnabled)
	// 🔴 刻意**不给** sync_mode 设默认值（2026-11-05 修）。
	//
	// 设过默认值的话，配置文件里没写 sync_mode 时 viper 也会把它填成 "rw"，
	// 于是 normalizeSyncMode 的 `case ""`（把旧键 readonly_sync_enabled=true
	// 折叠成 ro 的那一段）永远走不到 —— 老配置升上来会静默变成双向同步，
	// 而「只读」恰恰是运营方当初特意打开开关的原因。
	// 缺省值仍由 normalizeSyncMode 补齐（返回 rw），Default() 也照样是 rw，
	// 差别只在「配置文件到底有没有明说这一项」。
	v.SetDefault("manual_sync_enabled", d.ManualSyncEnabled)
	v.SetDefault("offline_sync_strategy", d.OfflineSyncStrategy)
	v.SetDefault("sync_update_delay", d.SyncUpdateDelay)
	v.SetDefault("binary_sync_limit_enabled", d.BinarySyncLimitEnabled)
	v.SetDefault("concurrency_control_enabled", d.ConcurrencyControlEnabled)
	v.SetDefault("max_concurrent_uploads", d.MaxConcurrentUploads)
	v.SetDefault("startup_delay", d.StartupDelay)
	v.SetDefault("auto_redirect_enabled", d.AutoRedirectEnabled)
	v.SetDefault("sync_timeout_seconds", d.SyncTimeoutSeconds)
}
