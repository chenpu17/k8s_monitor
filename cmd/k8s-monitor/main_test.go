package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/chenpu/k8s-monitor/internal/i18n"
	"github.com/spf13/pflag"
)

// Golden literals below are the original CLI texts before i18n; the English
// bundle values must match them byte for byte (regression hard requirement).

const goldenRootShort = "A CLI monitoring console for Kubernetes clusters"

const goldenRootLong = `k8s-monitor is a lightweight, read-only CLI monitoring console
for Kubernetes clusters. It provides real-time cluster health overview,
node status, workload monitoring, and quick diagnostics.

Run it without arguments (or with the "console" subcommand) to launch
the interactive TUI console.

Built with ❤️  using Bubble Tea and client-go.`

const goldenConsoleShort = "Start the interactive monitoring console"

const goldenConsoleLong = "Launch the interactive TUI console to monitor your Kubernetes cluster"

var goldenFlagUsages = map[string]string{
	"config":           "config file path (default: ./config/config.yaml)",
	"kubeconfig":       "path to kubeconfig file (default: $HOME/.kube/config)",
	"context":          "kubernetes context to use",
	"namespace":        "namespace to monitor (default: all namespaces)",
	"verbose":          "enable verbose logging",
	"locale":           "interface language (en, zh)",
	"refresh":          "refresh interval in seconds",
	"no-color":         "disable color output",
	"insecure-kubelet": "skip TLS verification for kubelet metrics (use in test environments)",
	"max-concurrent":   "maximum concurrent kubelet queries (default: 10)",
	"log-tail-lines":   "number of log lines to fetch (default: 200)",
	"npu-exporter":     "NPU-Exporter endpoint URL (e.g., http://npu-exporter.kube-system:8082)",
}

// cliMessageIDs is the exact set of cli.* message IDs both bundles must
// carry: 4 command texts, 12 flag usages, 4 error messages.
var cliMessageIDs = []string{
	"cli.root_short", "cli.root_long", "cli.console_short", "cli.console_long",
	"cli.flag_config", "cli.flag_kubeconfig", "cli.flag_context",
	"cli.flag_namespace", "cli.flag_verbose", "cli.flag_locale",
	"cli.flag_refresh", "cli.flag_no_color", "cli.flag_insecure_kubelet",
	"cli.flag_max_concurrent", "cli.flag_log_tail_lines", "cli.flag_npu_exporter",
	"cli.err_load_config", "cli.err_create_app", "cli.err_shutdown",
	"cli.err_application",
}

// writeLocaleConfig writes a minimal config file with the given ui.locale.
func writeLocaleConfig(t *testing.T, locale string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := fmt.Sprintf("ui:\n  locale: %s\n", locale)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config fixture: %v", err)
	}
	return path
}

// neutralizeLocaleEnv clears env variables viper's AutomaticEnv could use to
// override ui.locale, so config fixtures decide the outcome.
func neutralizeLocaleEnv(t *testing.T) {
	t.Helper()
	t.Setenv("K8S_MONITOR_UI.LOCALE", "")
	t.Setenv("K8S_MONITOR_UI_LOCALE", "")
}

func TestResolveLocale(t *testing.T) {
	neutralizeLocaleEnv(t)

	zhCfg := writeLocaleConfig(t, "zh")
	enCfg := writeLocaleConfig(t, "en")
	frCfg := writeLocaleConfig(t, "fr")
	brokenCfg := filepath.Join(t.TempDir(), "broken.yaml")
	if err := os.WriteFile(brokenCfg, []byte("ui: [unclosed\n"), 0o644); err != nil {
		t.Fatalf("write broken config fixture: %v", err)
	}

	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no args", nil, "en"},
		{"long flag separated", []string{"--locale", "zh"}, "zh"},
		{"short flag separated", []string{"-l", "zh"}, "zh"},
		{"long flag equals", []string{"--locale=zh"}, "zh"},
		{"short flag equals", []string{"-l=zh"}, "zh"},
		{"short flag attached value", []string{"-lzh"}, "zh"},
		{"explicit en", []string{"--locale", "en"}, "en"},
		{"after subcommand", []string{"console", "--locale", "zh"}, "zh"},
		{"interspersed flags", []string{"--no-color", "--locale", "zh", "--verbose"}, "zh"},
		{"unsupported value falls back to config", []string{"--locale", "fr", "--config", zhCfg}, "zh"},
		{"unsupported value without config", []string{"--locale", "fr"}, "en"},
		{"empty long flag value", []string{"--locale="}, "en"},
		{"empty long flag value falls back to config", []string{"--locale=", "--config", zhCfg}, "zh"},
		{"empty short flag value", []string{"-l="}, "en"},
		{"missing flag value at end", []string{"--locale"}, "en"},
		{"config separated", []string{"--config", zhCfg}, "zh"},
		{"config equals", []string{"--config=" + zhCfg}, "zh"},
		{"config after subcommand", []string{"console", "--config", zhCfg}, "zh"},
		{"config with en locale", []string{"--config", enCfg}, "en"},
		{"config with unsupported locale", []string{"--config", frCfg}, "en"},
		{"config load failure", []string{"--config", brokenCfg}, "en"},
		{"config file missing", []string{"--config", "/nonexistent/config.yaml"}, "en"},
		{"explicit flag beats config", []string{"--locale", "en", "--config", zhCfg}, "en"},
		{"last config occurrence wins", []string{"--config", enCfg, "--config", zhCfg}, "zh"},
		{"flag value not mistaken for flag", []string{"--namespace", "-l", "zh"}, "en"},
		{"locale flag as trailing value only", []string{"--namespace", "-l"}, "en"},
		{"value flag consumes its value", []string{"--namespace", "zh", "-l", "en"}, "en"},
		{"double dash stops parsing", []string{"--", "--locale", "zh"}, "en"},
		{"klog-style flag does not swallow next flag", []string{"-v", "3", "--locale", "zh"}, "zh"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveLocale(tc.args); got != tc.want {
				t.Errorf("resolveLocale(%q) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

func TestCLITextsEnglish(t *testing.T) {
	applyCLITexts(i18n.NewLocalizer("en"))

	if rootCmd.Short != goldenRootShort {
		t.Errorf("root Short mismatch:\n got %q\nwant %q", rootCmd.Short, goldenRootShort)
	}
	if rootCmd.Long != goldenRootLong {
		t.Errorf("root Long mismatch:\n got %q\nwant %q", rootCmd.Long, goldenRootLong)
	}
	if consoleCmd.Short != goldenConsoleShort {
		t.Errorf("console Short mismatch:\n got %q\nwant %q", consoleCmd.Short, goldenConsoleShort)
	}
	if consoleCmd.Long != goldenConsoleLong {
		t.Errorf("console Long mismatch:\n got %q\nwant %q", consoleCmd.Long, goldenConsoleLong)
	}

	seen := map[string]bool{}
	rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		want, ok := goldenFlagUsages[f.Name]
		if !ok {
			return // library flags (klog, Go std) keep their own texts
		}
		seen[f.Name] = true
		if f.Usage != want {
			t.Errorf("flag --%s usage mismatch:\n got %q\nwant %q", f.Name, f.Usage, want)
		}
	})
	for name := range goldenFlagUsages {
		if !seen[name] {
			t.Errorf("flag --%s not found on rootCmd.PersistentFlags()", name)
		}
	}
}

func TestCLIErrorMessagesEnglish(t *testing.T) {
	loc := i18n.NewLocalizer("en")
	sentinel := errors.New("inner failure")

	err := fmt.Errorf("%s: %w", loc.T("cli.err_load_config"), sentinel)
	if got, want := err.Error(), "failed to load config: inner failure"; got != want {
		t.Errorf("load config error = %q, want %q", got, want)
	}
	if !errors.Is(err, sentinel) {
		t.Error("load config error must wrap the cause with %w")
	}

	err = fmt.Errorf("%s: %w", loc.T("cli.err_create_app"), sentinel)
	if got, want := err.Error(), "failed to create application: inner failure"; got != want {
		t.Errorf("create app error = %q, want %q", got, want)
	}
	if !errors.Is(err, sentinel) {
		t.Error("create app error must wrap the cause with %w")
	}

	err = fmt.Errorf("%s: %w", loc.T("cli.err_application"), sentinel)
	if got, want := err.Error(), "application error: inner failure"; got != want {
		t.Errorf("application error = %q, want %q", got, want)
	}
	if !errors.Is(err, sentinel) {
		t.Error("application error must wrap the cause with %w")
	}

	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%s: %v\n", loc.T("cli.err_shutdown"), sentinel)
	if got, want := buf.String(), "Error during shutdown: inner failure\n"; got != want {
		t.Errorf("shutdown message = %q, want %q", got, want)
	}
}

// loadTOMLMessages reads a go-i18n TOML message file and flattens dotted
// table headers ([a.b] other = "...") to "a.b" keys, mirroring how go-i18n
// loads these bundles. A table whose keys are plural forms (other, one, ...)
// is a message; its "other" form is used.
func loadTOMLMessages(t *testing.T, path string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var raw map[string]interface{}
	if err := toml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	pluralForms := map[string]bool{
		"zero": true, "one": true, "two": true,
		"few": true, "many": true, "other": true,
	}
	msgs := map[string]string{}
	var flatten func(prefix string, m map[string]interface{})
	flatten = func(prefix string, m map[string]interface{}) {
		for k, v := range m {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			sub, ok := v.(map[string]interface{})
			if !ok {
				continue
			}
			isMessage := false
			for form := range sub {
				if pluralForms[form] {
					isMessage = true
					break
				}
			}
			if isMessage {
				if other, ok := sub["other"].(string); ok {
					msgs[key] = other
				}
				continue
			}
			flatten(key, sub)
		}
	}
	flatten("", raw)
	return msgs
}

func TestCLIBundleKeySets(t *testing.T) {
	want := map[string]bool{}
	for _, id := range cliMessageIDs {
		want[id] = true
	}
	for _, path := range []string{
		"../../internal/i18n/active.en.toml",
		"../../internal/i18n/active.zh.toml",
	} {
		got := map[string]bool{}
		for id := range loadTOMLMessages(t, path) {
			if strings.HasPrefix(id, "cli.") {
				got[id] = true
			}
		}
		if !reflect.DeepEqual(got, want) {
			var missing, extra []string
			for id := range want {
				if !got[id] {
					missing = append(missing, id)
				}
			}
			for id := range got {
				if !want[id] {
					extra = append(extra, id)
				}
			}
			t.Errorf("%s cli.* key set mismatch: missing %v, orphaned %v", path, missing, extra)
		}
	}
}

func TestCLITextsMatchBundle(t *testing.T) {
	for _, locale := range []string{"en", "zh"} {
		t.Run(locale, func(t *testing.T) {
			path := fmt.Sprintf("../../internal/i18n/active.%s.toml", locale)
			msgs := loadTOMLMessages(t, path)
			applyCLITexts(i18n.NewLocalizer(locale))

			checks := map[string]string{
				"cli.root_short":    rootCmd.Short,
				"cli.root_long":     rootCmd.Long,
				"cli.console_short": consoleCmd.Short,
				"cli.console_long":  consoleCmd.Long,
			}
			for id, got := range checks {
				want, ok := msgs[id]
				if !ok {
					t.Fatalf("message %s missing in %s", id, path)
				}
				if got != want {
					t.Errorf("%s: got %q, want %q", id, got, want)
				}
			}

			rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
				id, ok := flagUsageKeys[f.Name]
				if !ok {
					return
				}
				want, ok := msgs[id]
				if !ok {
					t.Errorf("message %s missing in %s", id, path)
					return
				}
				if f.Usage != want {
					t.Errorf("%s (flag --%s): got %q, want %q", id, f.Name, f.Usage, want)
				}
			})

			for _, id := range []string{
				"cli.err_load_config", "cli.err_create_app",
				"cli.err_shutdown", "cli.err_application",
			} {
				want, ok := msgs[id]
				if !ok {
					t.Errorf("message %s missing in %s", id, path)
					continue
				}
				if got := i18n.NewLocalizer(locale).T(id); got != want {
					t.Errorf("%s: got %q, want %q", id, got, want)
				}
			}

			if locale == "zh" && rootCmd.Short == goldenRootShort {
				t.Error("zh root Short equals the English text; zh bundle not applied")
			}
		})
	}
}

// TestFlagUsageKeyWiring pins the flagUsageKeys wiring against the other two
// collections. A deleted or renamed mapping entry used to pass every test:
// the en golden is masked by the identical literals still in init(), and the
// zh checks only visit flags that are still in the map, so a dropped flag
// silently kept its English usage in the Chinese help (batch-10 verify
// mutation finding).
func TestFlagUsageKeyWiring(t *testing.T) {
	if len(flagUsageKeys) != len(goldenFlagUsages) {
		t.Fatalf("flagUsageKeys has %d entries, goldenFlagUsages has %d: a flag mapping was added or removed without the other",
			len(flagUsageKeys), len(goldenFlagUsages))
	}
	cliIDs := make(map[string]bool, len(cliMessageIDs))
	for _, id := range cliMessageIDs {
		cliIDs[id] = true
	}
	for name, key := range flagUsageKeys {
		if _, ok := goldenFlagUsages[name]; !ok {
			t.Errorf("flagUsageKeys maps %q but the golden set has no such flag", name)
		}
		if !cliIDs[key] {
			t.Errorf("flagUsageKeys[%q] = %q, which is not part of cliMessageIDs", name, key)
		}
	}
}
