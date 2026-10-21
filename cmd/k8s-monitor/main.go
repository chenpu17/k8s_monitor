package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/chenpu/k8s-monitor/internal/app"
	"github.com/chenpu/k8s-monitor/internal/i18n"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"go.uber.org/zap"
	"k8s.io/klog/v2"
)

var (
	// Version will be set by build flags
	Version = "dev"
	// BuildTime will be set by build flags
	BuildTime = "unknown"

	// Global flags
	configFile string
	kubeconfig string
	context    string
	namespace  string
	verbose    bool
	locale     string
)

var rootCmd = &cobra.Command{
	Use:   "k8s-monitor",
	Short: "A CLI monitoring console for Kubernetes clusters",
	Long: `k8s-monitor is a lightweight, read-only CLI monitoring console
for Kubernetes clusters. It provides real-time cluster health overview,
node status, workload monitoring, and quick diagnostics.

Run it without arguments (or with the "console" subcommand) to launch
the interactive TUI console.

Built with ❤️  using Bubble Tea and client-go.`,
	Version: fmt.Sprintf("%s (built: %s)", Version, BuildTime),
	RunE:    runConsole, // default action: launch the interactive console
}

var consoleCmd = &cobra.Command{
	Use:   "console",
	Short: "Start the interactive monitoring console",
	Long:  `Launch the interactive TUI console to monitor your Kubernetes cluster`,
	RunE:  runConsole,
}

// cliLocalizer localizes CLI help texts and error messages. Cobra renders
// help/usage before RunE runs, so the texts must be localized before
// Execute; main() replaces this default (English) with the locale resolved
// from os.Args.
var cliLocalizer = i18n.NewLocalizer("en")

// flagUsageKeys maps the persistent flags declared in this file to their
// i18n message IDs. Library flags (klog, Go standard flags) are not listed
// and keep their original usage strings.
var flagUsageKeys = map[string]string{
	"config":           "cli.flag_config",
	"kubeconfig":       "cli.flag_kubeconfig",
	"context":          "cli.flag_context",
	"namespace":        "cli.flag_namespace",
	"verbose":          "cli.flag_verbose",
	"locale":           "cli.flag_locale",
	"refresh":          "cli.flag_refresh",
	"no-color":         "cli.flag_no_color",
	"insecure-kubelet": "cli.flag_insecure_kubelet",
	"max-concurrent":   "cli.flag_max_concurrent",
	"log-tail-lines":   "cli.flag_log_tail_lines",
	"npu-exporter":     "cli.flag_npu_exporter",
}

// applyCLITexts fills the command and flag texts from the given localizer.
func applyCLITexts(loc *i18n.Localizer) {
	rootCmd.Short = loc.T("cli.root_short")
	rootCmd.Long = loc.T("cli.root_long")
	consoleCmd.Short = loc.T("cli.console_short")
	consoleCmd.Long = loc.T("cli.console_long")
	rootCmd.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if key, ok := flagUsageKeys[f.Name]; ok {
			f.Usage = loc.T(key)
		}
	})
}

func init() {
	// Configure klog to suppress client-go logs in TUI mode
	// klog writes to stderr by default, which pollutes the TUI
	klog.InitFlags(nil)
	flag.Set("logtostderr", "false")     // Don't log to stderr
	flag.Set("alsologtostderr", "false") // Don't also log to stderr
	flag.Set("stderrthreshold", "FATAL") // Only FATAL errors to stderr
	flag.Set("v", "0")                   // Minimal verbosity

	// Add Go flags to pflag so Cobra can parse them
	// This avoids conflicts when global flags are placed before subcommands
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	// Add subcommands
	rootCmd.AddCommand(consoleCmd)

	// Global persistent flags
	rootCmd.PersistentFlags().StringVar(&configFile, "config", "", "config file path (default: ./config/config.yaml)")
	rootCmd.PersistentFlags().StringVarP(&kubeconfig, "kubeconfig", "k", "", "path to kubeconfig file (default: $HOME/.kube/config)")
	rootCmd.PersistentFlags().StringVarP(&context, "context", "c", "", "kubernetes context to use")
	rootCmd.PersistentFlags().StringVarP(&namespace, "namespace", "n", "", "namespace to monitor (default: all namespaces)")
	rootCmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "enable verbose logging")
	rootCmd.PersistentFlags().StringVarP(&locale, "locale", "l", "en", "interface language (en, zh)")

	// Console flags are persistent so they work both with and without the
	// explicit "console" subcommand (bare invocation defaults to console)
	rootCmd.PersistentFlags().IntP("refresh", "r", 2, "refresh interval in seconds")
	rootCmd.PersistentFlags().BoolP("no-color", "", false, "disable color output")
	rootCmd.PersistentFlags().BoolP("insecure-kubelet", "", false, "skip TLS verification for kubelet metrics (use in test environments)")
	rootCmd.PersistentFlags().IntP("max-concurrent", "m", 10, "maximum concurrent kubelet queries (default: 10)")
	rootCmd.PersistentFlags().IntP("log-tail-lines", "", 200, "number of log lines to fetch (default: 200)")
	rootCmd.PersistentFlags().StringP("npu-exporter", "", "", "NPU-Exporter endpoint URL (e.g., http://npu-exporter.kube-system:8082)")
}

// cliFlagsWithValue lists flag names (long and shorthand form) that take a
// value, so the value following them is not mistaken for another flag while
// scanning os.Args. Bool flags and unknown flags (e.g. klog ones) are not
// listed; misparsing them only degrades locale detection, which cobra still
// parses correctly afterwards.
var cliFlagsWithValue = map[string]bool{
	"config":     true,
	"kubeconfig": true, "k": true,
	"context": true, "c": true,
	"namespace": true, "n": true,
	"locale": true, "l": true,
	"refresh": true, "r": true,
	"max-concurrent": true, "m": true,
	"npu-exporter":   true,
	"log-tail-lines": true,
}

// isSupportedLocale reports whether the locale has CLI translations.
func isSupportedLocale(locale string) bool {
	return locale == "en" || locale == "zh"
}

// resolveLocale determines the language for CLI texts before cobra parses
// flags, by scanning args (pflag allows flags after subcommands) in order:
//  1. an explicit --locale/-l flag with a supported value (en, zh);
//  2. the ui.locale setting of the config file given via --config
//     (loading failures are silent here; runConsole reports real errors);
//  3. "en".
func resolveLocale(args []string) string {
	configSeen := false
	configPath := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break // pflag stops flag parsing here
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			continue // positional argument
		}
		name, value, hasValue := splitFlagArg(arg)
		if !hasValue && cliFlagsWithValue[name] && i+1 < len(args) {
			value = args[i+1]
			hasValue = true
			i++
		}
		switch name {
		case "locale", "l":
			if isSupportedLocale(value) {
				return value
			}
		case "config":
			configSeen = true
			if hasValue {
				configPath = value
			}
		}
	}
	if configSeen {
		if cfg, err := app.LoadConfig(configPath); err == nil && isSupportedLocale(cfg.Locale) {
			return cfg.Locale
		}
	}
	return "en"
}

// splitFlagArg splits an argument such as "--name=value", "-nvalue" or
// "-n=value" into flag name and attached value. The boolean result reports
// whether a value was attached to the argument itself.
func splitFlagArg(arg string) (name, value string, hasValue bool) {
	if strings.HasPrefix(arg, "--") {
		name = arg[2:]
	} else {
		name = arg[1:]
	}
	if name == "" || strings.HasPrefix(name, "-") {
		return "", "", false // bare "-" or excess dashes
	}
	if i := strings.Index(name, "="); i >= 0 {
		return name[:i], name[i+1:], true
	}
	if !strings.HasPrefix(arg, "--") && len(name) > 1 {
		return name[:1], name[1:], true // shorthand with attached value, e.g. -lzh
	}
	return name, "", false
}

func runConsole(cmd *cobra.Command, args []string) error {
	// Load configuration
	config, err := app.LoadConfig(configFile)
	if err != nil {
		return fmt.Errorf("%s: %w", cliLocalizer.T("cli.err_load_config"), err)
	}

	// Override config with command-line flags
	if kubeconfig != "" {
		config.Kubeconfig = kubeconfig
	}
	if context != "" {
		config.Context = context
	}
	if namespace != "" {
		config.Namespace = namespace
	}
	// Only override locale if user explicitly specified it
	if cmd.Flags().Changed("locale") {
		config.Locale = locale
	}
	if verbose {
		config.LogLevel = "debug"
	}

	// Override refresh interval flag only if user explicitly specified it
	if cmd.Flags().Changed("refresh") {
		if refresh, _ := cmd.Flags().GetInt("refresh"); refresh > 0 {
			config.RefreshInterval = time.Duration(refresh) * time.Second
		}
	}

	// Override no-color flag
	if noColor, _ := cmd.Flags().GetBool("no-color"); noColor {
		config.NoColor = true
		config.ColorMode = "never"
	}

	// Override insecure-kubelet flag
	if insecureKubelet, _ := cmd.Flags().GetBool("insecure-kubelet"); insecureKubelet {
		config.InsecureKubelet = true
	}

	// Override max-concurrent flag only if user explicitly specified it
	if cmd.Flags().Changed("max-concurrent") {
		if maxConcurrent, _ := cmd.Flags().GetInt("max-concurrent"); maxConcurrent > 0 {
			config.MaxConcurrent = maxConcurrent
		}
	}

	// Override log-tail-lines flag only if user explicitly specified it
	if cmd.Flags().Changed("log-tail-lines") {
		if logTailLines, _ := cmd.Flags().GetInt("log-tail-lines"); logTailLines > 0 {
			config.LogTailLines = logTailLines
		}
	}

	// Override npu-exporter endpoint flag only if user explicitly specified it
	if cmd.Flags().Changed("npu-exporter") {
		if npuExporter, _ := cmd.Flags().GetString("npu-exporter"); npuExporter != "" {
			config.NPUExporterEndpoint = npuExporter
		}
	}

	// Create application instance with full version info
	fullVersion := fmt.Sprintf("%s (built: %s)", Version, BuildTime)
	application, err := app.New(config, fullVersion)
	if err != nil {
		return fmt.Errorf("%s: %w", cliLocalizer.T("cli.err_create_app"), err)
	}
	defer func() {
		if err := application.Shutdown(); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", cliLocalizer.T("cli.err_shutdown"), err)
		}
	}()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Run application in goroutine
	errChan := make(chan error, 1)
	go func() {
		errChan <- application.Run()
	}()

	// Wait for either error or signal
	select {
	case err := <-errChan:
		if err != nil {
			return fmt.Errorf("%s: %w", cliLocalizer.T("cli.err_application"), err)
		}
	case sig := <-sigChan:
		zap.L().Info("Received signal, shutting down...", zap.String("signal", sig.String()))
	}

	return nil
}

func main() {
	cliLocalizer = i18n.NewLocalizer(resolveLocale(os.Args[1:]))
	applyCLITexts(cliLocalizer)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
