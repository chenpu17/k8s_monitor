package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/chenpu/k8s-monitor/internal/app"
	"github.com/chenpu/k8s-monitor/internal/i18n"
)

// TestResolveLocaleDefaultConfigPath is the W4-5 regression guard: the help
// language was only taken from a config file named via --config, while the TUI
// finds the config in the default paths (./config, $HOME/.k8s-monitor,
// /etc/k8s-monitor). A zh config in a default location therefore gave a Chinese
// console with English help.
func TestResolveLocaleDefaultConfigPath(t *testing.T) {
	neutralizeLocaleEnv(t)

	t.Run("home default location", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		dir := filepath.Join(home, ".k8s-monitor")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create config dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("ui:\n  locale: zh\n"), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}

		if got := resolveLocale(nil); got != "zh" {
			t.Errorf("resolveLocale(nil) = %q with a zh config in the default location, want zh", got)
		}
		if got := resolveLocale([]string{"--help"}); got != "zh" {
			t.Errorf("resolveLocale(--help) = %q with a zh config in the default location, want zh", got)
		}
	})

	t.Run("explicit flag still wins", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		dir := filepath.Join(home, ".k8s-monitor")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create config dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("ui:\n  locale: zh\n"), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}

		if got := resolveLocale([]string{"--locale", "en"}); got != "en" {
			t.Errorf("resolveLocale(--locale en) = %q, want the explicit en", got)
		}
	})

	t.Run("broken default config fails soft", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		dir := filepath.Join(home, ".k8s-monitor")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create config dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("ui: [unclosed\n"), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}

		if got := resolveLocale(nil); got != "en" {
			t.Errorf("resolveLocale(nil) = %q with a broken default config, want the en fallback", got)
		}
	})

	t.Run("unsupported value in default config", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)

		dir := filepath.Join(home, ".k8s-monitor")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create config dir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("ui:\n  locale: fr\n"), 0o644); err != nil {
			t.Fatalf("write config: %v", err)
		}

		if got := resolveLocale(nil); got != "en" {
			t.Errorf("resolveLocale(nil) = %q with an unsupported default locale, want en", got)
		}
	})
}

// TestHelpRendersInDefaultConfigLocale drives the help text end to end: the
// localizer applyCLITexts uses must be the one the default-location config
// selects.
func TestHelpRendersInDefaultConfigLocale(t *testing.T) {
	neutralizeLocaleEnv(t)

	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".k8s-monitor")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("ui:\n  locale: zh\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	locale := resolveLocale([]string{"--help"})
	if locale != "zh" {
		t.Fatalf("resolveLocale(--help) = %q, want zh", locale)
	}
	if got := i18n.NewLocalizer(locale).T("cli.root_short"); got == goldenRootShort {
		t.Errorf("the zh localizer resolved the English root_short: %q", got)
	}

	// Restore the package-level localizer for the other tests in this package.
	t.Cleanup(func() { applyCLITexts(i18n.NewLocalizer("en")) })
}

// TestLocaleFromConfigIsFailSoft pins the helper the help path relies on: a
// missing file and a broken file both yield an empty locale and no panic, so
// --help can never be blocked by a bad config.
func TestLocaleFromConfigIsFailSoft(t *testing.T) {
	t.Run("missing file", func(t *testing.T) {
		locale, err := app.LocaleFromConfig(filepath.Join(t.TempDir(), "nope.yaml"))
		if err != nil {
			t.Errorf("LocaleFromConfig(missing) returned error %v, want nil", err)
		}
		if locale != "" {
			t.Errorf("LocaleFromConfig(missing) = %q, want empty", locale)
		}
	})

	t.Run("broken file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "broken.yaml")
		if err := os.WriteFile(path, []byte("ui: [unclosed\n"), 0o644); err != nil {
			t.Fatalf("write broken config: %v", err)
		}
		locale, err := app.LocaleFromConfig(path)
		if err == nil {
			t.Errorf("LocaleFromConfig(broken) returned no error; the caller needs the error to fall back")
		}
		if locale != "" {
			t.Errorf("LocaleFromConfig(broken) = %q, want empty", locale)
		}
	})

	t.Run("no config anywhere", func(t *testing.T) {
		t.Setenv("HOME", t.TempDir())
		locale, err := app.LocaleFromConfig("")
		if err != nil {
			t.Errorf("LocaleFromConfig(\"\") returned error %v with no config present, want nil", err)
		}
		if locale != "" {
			t.Errorf("LocaleFromConfig(\"\") = %q with no config present, want empty", locale)
		}
	})

	// An explicit non-YAML config must be parsed by its extension, exactly as
	// LoadConfig does: forcing the yaml parser here made --config config.toml
	// read as broken for the help language while the console loaded it fine,
	// splitting the help from the TUI language again.
	t.Run("explicit toml file is honored by extension", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.toml")
		if err := os.WriteFile(path, []byte("[ui]\nlocale = \"zh\"\n"), 0o644); err != nil {
			t.Fatalf("write toml config: %v", err)
		}
		locale, err := app.LocaleFromConfig(path)
		if err != nil {
			t.Fatalf("LocaleFromConfig(toml) returned error %v, want nil", err)
		}
		if locale != "zh" {
			t.Errorf("LocaleFromConfig(toml) = %q, want zh", locale)
		}
	})
}
