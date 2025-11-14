package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/yourusername/k8s-monitor/internal/app"
	"go.uber.org/zap"
	"k8s.io/klog/v2"
)

var (
	// Version will be set by build flags, default to timestamp
	Version = "dev-" + time.Now().Format("20060102-150405")
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

Built with ❤️  using Bubble Tea and client-go.`,
	Version: Version,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var consoleCmd = &cobra.Command{
	Use:   "console",
	Short: "Start the interactive monitoring console",
	Long:  `Launch the interactive TUI console to monitor your Kubernetes cluster`,
	RunE:  runConsole,
}

func init() {
	// Configure klog to suppress client-go logs in TUI mode
	// klog writes to stderr by default, which pollutes the TUI
	klog.InitFlags(nil)
	flag.Set("logtostderr", "false")       // Don't log to stderr
	flag.Set("alsologtostderr", "false")   // Don't also log to stderr
	flag.Set("stderrthreshold", "FATAL")   // Only FATAL errors to stderr
	flag.Set("v", "0")                     // Minimal verbosity

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

	// Console command flags
	consoleCmd.Flags().IntP("refresh", "r", 2, "refresh interval in seconds")
	consoleCmd.Flags().BoolP("no-color", "", false, "disable color output")
	consoleCmd.Flags().BoolP("insecure-kubelet", "", false, "skip TLS verification for kubelet metrics (use in test environments)")
	consoleCmd.Flags().IntP("max-concurrent", "m", 10, "maximum concurrent kubelet queries (default: 10)")
}

func runConsole(cmd *cobra.Command, args []string) error {
	// Load configuration
	config, err := app.LoadConfig(configFile)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
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

	// Override refresh interval from flag
	if refresh, _ := cmd.Flags().GetInt("refresh"); refresh > 0 {
		config.RefreshInterval = time.Duration(refresh) * time.Second
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

	// Create application instance
	application, err := app.New(config, Version)
	if err != nil {
		return fmt.Errorf("failed to create application: %w", err)
	}
	defer func() {
		if err := application.Shutdown(); err != nil {
			fmt.Fprintf(os.Stderr, "Error during shutdown: %v\n", err)
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
			return fmt.Errorf("application error: %w", err)
		}
	case sig := <-sigChan:
		zap.L().Info("Received signal, shutting down...", zap.String("signal", sig.String()))
	}

	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
