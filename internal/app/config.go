package app

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
)

// Config holds the application configuration
type Config struct {
	// Cluster configuration
	Kubeconfig string `mapstructure:"kubeconfig"`
	Context    string `mapstructure:"context"`
	Namespace  string `mapstructure:"namespace"`

	// Refresh configuration
	RefreshInterval time.Duration `mapstructure:"refresh_interval"`
	Timeout         time.Duration `mapstructure:"timeout"`
	MaxConcurrent   int           `mapstructure:"max_concurrent"`

	// Cache configuration
	CacheTTL        time.Duration `mapstructure:"cache_ttl"`
	MaxCacheEntries int           `mapstructure:"max_cache_entries"`

	// UI configuration
	ColorMode    string `mapstructure:"color_mode"`
	DefaultView  string `mapstructure:"default_view"`
	MaxRows      int    `mapstructure:"max_rows"`
	NoColor      bool   `mapstructure:"no_color"`
	Locale       string `mapstructure:"locale"`
	LogTailLines int    `mapstructure:"log_tail_lines"`

	// Kubelet configuration
	InsecureKubelet bool `mapstructure:"insecure_kubelet"`

	// NPU-Exporter configuration
	NPUExporterEndpoint string `mapstructure:"npu_exporter_endpoint"`

	// Logging configuration
	LogLevel string `mapstructure:"log_level"`
	LogFile  string `mapstructure:"log_file"`
}

// LocaleFromConfig returns the ui.locale setting of the configuration the
// console will load: the file at configFile when it is set, otherwise the first
// one the default search path finds (./config, $HOME/.k8s-monitor,
// /etc/k8s-monitor) — the same paths LoadConfig uses, so the CLI texts and the
// TUI agree on the language.
//
// It is meant for the CLI help path, which must not fail on a broken
// configuration: a missing file, an unparsable one or a missing key all return
// ("", nil). The caller decides whether the value is a supported locale.
func LocaleFromConfig(configFile string) (string, error) {
	// A fresh Viper instance keeps this read out of the global state LoadConfig
	// populates, so probing for the help language cannot leak a config file
	// into the console's own load later in the same process.
	v := viper.New()
	if configFile != "" {
		// Like LoadConfig, let Viper infer the type from the file extension:
		// forcing yaml here would reject e.g. a TOML config the console itself
		// reads fine, and split the help language from the TUI language again.
		v.SetConfigFile(configFile)
	} else {
		v.SetConfigType("yaml")
		v.SetConfigName("config")
		v.AddConfigPath("./config")
		v.AddConfigPath("$HOME/.k8s-monitor")
		v.AddConfigPath("/etc/k8s-monitor")
	}

	if err := v.ReadInConfig(); err != nil {
		if _, notFound := err.(viper.ConfigFileNotFoundError); notFound || errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return v.GetString("ui.locale"), nil
}

// LoadConfig loads configuration from file and environment
func LoadConfig(configFile string) (*Config, error) {
	// Defaults – nested keys align with config/default.yaml
	viper.SetDefault("cluster.kubeconfig", "")
	viper.SetDefault("cluster.context", "")
	viper.SetDefault("cluster.namespace", "")

	viper.SetDefault("refresh.interval", "2s")
	viper.SetDefault("refresh.timeout", "30s")
	viper.SetDefault("refresh.max_concurrent", 10)

	viper.SetDefault("cache.ttl", "60s")
	viper.SetDefault("cache.max_entries", 1000)

	viper.SetDefault("ui.color_mode", "auto")
	viper.SetDefault("ui.default_view", "overview")
	viper.SetDefault("ui.max_rows", 100)
	viper.SetDefault("ui.no_color", false)
	viper.SetDefault("ui.locale", "en")
	viper.SetDefault("ui.log_tail_lines", 200)

	viper.SetDefault("kubelet.insecure", false)

	viper.SetDefault("npu_exporter.endpoint", "")

	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.file", "/tmp/k8s-monitor.log")

	// Home kubeconfig default
	if home, err := os.UserHomeDir(); err == nil {
		viper.SetDefault("cluster.kubeconfig", filepath.Join(home, ".kube", "config"))
	}

	if configFile != "" {
		viper.SetConfigFile(configFile)
	} else {
		viper.SetConfigName("config")
		viper.SetConfigType("yaml")
		viper.AddConfigPath("./config")
		viper.AddConfigPath("$HOME/.k8s-monitor")
		viper.AddConfigPath("/etc/k8s-monitor")
	}

	viper.SetEnvPrefix("K8S_MONITOR")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config: %w", err)
		}
	}

	cfg := &Config{
		Kubeconfig:          viper.GetString("cluster.kubeconfig"),
		Context:             viper.GetString("cluster.context"),
		Namespace:           viper.GetString("cluster.namespace"),
		RefreshInterval:     viper.GetDuration("refresh.interval"),
		Timeout:             viper.GetDuration("refresh.timeout"),
		MaxConcurrent:       viper.GetInt("refresh.max_concurrent"),
		CacheTTL:            viper.GetDuration("cache.ttl"),
		MaxCacheEntries:     viper.GetInt("cache.max_entries"),
		ColorMode:           viper.GetString("ui.color_mode"),
		DefaultView:         viper.GetString("ui.default_view"),
		MaxRows:             viper.GetInt("ui.max_rows"),
		NoColor:             viper.GetBool("ui.no_color"),
		Locale:              viper.GetString("ui.locale"),
		LogTailLines:        viper.GetInt("ui.log_tail_lines"),
		InsecureKubelet:     viper.GetBool("kubelet.insecure"),
		NPUExporterEndpoint: viper.GetString("npu_exporter.endpoint"),
		LogLevel:            viper.GetString("logging.level"),
		LogFile:             viper.GetString("logging.file"),
	}

	// Normalise zero values in case configuration omitted units or left blank
	if cfg.RefreshInterval <= 0 {
		cfg.RefreshInterval = 2 * time.Second
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 30 * time.Second
	}
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 10
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = 60 * time.Second
	}
	if cfg.MaxCacheEntries <= 0 {
		cfg.MaxCacheEntries = 1000
	}
	if cfg.ColorMode == "" {
		cfg.ColorMode = "auto"
	}
	if cfg.DefaultView == "" {
		cfg.DefaultView = "overview"
	}
	if cfg.MaxRows <= 0 {
		cfg.MaxRows = 100
	}
	if cfg.LogTailLines <= 0 {
		cfg.LogTailLines = 200
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	if cfg.LogFile == "" {
		cfg.LogFile = "/tmp/k8s-monitor.log"
	}

	return cfg, nil
}
