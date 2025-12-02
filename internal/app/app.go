package app

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/yourusername/k8s-monitor/internal/cache"
	"github.com/yourusername/k8s-monitor/internal/datasource"
	"github.com/yourusername/k8s-monitor/internal/model"
	"github.com/yourusername/k8s-monitor/internal/ui"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// App represents the main application
type App struct {
	ctx        context.Context
	logger     *zap.Logger
	config     *Config
	version    string
	dataSource *datasource.AggregatedDataSource
	cache      *cache.TTLCache
	refresher  *cache.Refresher
}

// New creates a new App instance
func New(config *Config, version string) (*App, error) {
	// Initialize logger
	logger, err := initLogger(config.LogLevel, config.LogFile)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize logger: %w", err)
	}

	return &App{
		ctx:     context.Background(),
		logger:  logger,
		config:  config,
		version: version,
	}, nil
}

// Run starts the application
func (a *App) Run() error {
	a.logger.Info("Starting k8s-monitor application",
		zap.String("version", a.version),
		zap.String("kubeconfig", a.config.Kubeconfig),
		zap.String("context", a.config.Context),
		zap.String("namespace", a.config.Namespace),
		zap.Duration("refresh_interval", a.config.RefreshInterval),
	)

	// Log configuration details
	a.logger.Debug("Application configuration loaded",
		zap.Duration("cache_ttl", a.config.CacheTTL),
		zap.Int("max_concurrent", a.config.MaxConcurrent),
		zap.String("log_level", a.config.LogLevel),
		zap.String("log_file", a.config.LogFile),
	)

	// Initialize data sources
	if err := a.initDataSources(); err != nil {
		return fmt.Errorf("failed to initialize data sources: %w", err)
	}

	// Start background refresh
	if err := a.refresher.Start(); err != nil {
		return fmt.Errorf("failed to start refresher: %w", err)
	}

	// Start Bubble Tea UI
	if err := a.startUI(); err != nil {
		return fmt.Errorf("failed to start UI: %w", err)
	}

	a.logger.Info("Application started successfully")
	return nil
}

// startUI starts the Bubble Tea UI
func (a *App) startUI() error {
	a.logger.Info("Starting UI", zap.String("locale", a.config.Locale))

	uiModel := ui.NewModel(a, a.logger, a.config.RefreshInterval, a.config.Locale, a.version, a.config.LogTailLines)
	p := tea.NewProgram(uiModel, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("UI error: %w", err)
	}

	return nil
}

// initDataSources initializes all data sources
func (a *App) initDataSources() error {
	a.logger.Info("Initializing data sources")

	// Create API Server client
	apiServer, err := datasource.NewAPIServerClient(a.config.Kubeconfig, a.config.Context, a.logger)
	if err != nil {
		return fmt.Errorf("failed to create API Server client: %w", err)
	}

	// Create kubelet client (using proxy mode)
	kubeletClient, err := datasource.NewKubeletClient(apiServer.GetConfig(), true, a.config.InsecureKubelet, a.logger)
	if err != nil {
		a.logger.Warn("Failed to create kubelet client, continuing without metrics",
			zap.Error(err),
		)
		kubeletClient = nil
	}

	// Create aggregated data source
	a.dataSource = datasource.NewAggregatedDataSource(apiServer, kubeletClient, a.logger, a.config.MaxConcurrent)

	// Create Volcano client (optional - will work without it)
	volcanoClient, err := datasource.NewVolcanoClient(apiServer.GetConfig(), a.logger)
	if err != nil {
		a.logger.Warn("Failed to create Volcano client, Volcano features disabled",
			zap.Error(err),
		)
	} else {
		a.dataSource.SetVolcanoClient(volcanoClient)
	}

	// Create NPU-Exporter client (optional - for Huawei Ascend NPU metrics)
	npuExporterClient, err := datasource.NewNPUExporterClient(apiServer.GetConfig(), a.logger)
	if err != nil {
		a.logger.Warn("Failed to create NPU-Exporter client, NPU metrics from exporter disabled",
			zap.Error(err),
		)
	} else {
		// Set custom endpoint if configured
		if a.config.NPUExporterEndpoint != "" {
			npuExporterClient.SetEndpoint(a.config.NPUExporterEndpoint)
		}
		a.dataSource.SetNPUExporterClient(npuExporterClient)
	}

	// Create cache
	a.cache = cache.NewTTLCache(a.config.CacheTTL, a.logger)

	// Create refresher
	a.refresher = cache.NewRefresher(
		a.dataSource,
		a.cache,
		a.config.RefreshInterval,
		a.config.Namespace,
		a.logger,
	)

	a.logger.Info("Data sources initialized successfully")
	return nil
}

// GetClusterData retrieves cluster data (from cache or fresh)
func (a *App) GetClusterData() (*model.ClusterData, error) {
	// Try cache first
	if data, ok := a.cache.Get(a.ctx); ok {
		return data, nil
	}

	// Cache miss, fetch fresh data
	a.logger.Debug("Cache miss, fetching fresh data")
	return a.dataSource.GetClusterData(a.ctx, a.config.Namespace)
}

// GetPodLogs retrieves logs for a specific pod and container
func (a *App) GetPodLogs(ctx context.Context, namespace, podName, containerName string, tailLines int64) (string, error) {
	if a.dataSource == nil {
		return "", fmt.Errorf("data source not initialized")
	}
	return a.dataSource.GetPodLogs(ctx, namespace, podName, containerName, tailLines)
}

// ForceRefresh triggers an immediate data refresh
func (a *App) ForceRefresh() error {
	if a.refresher == nil {
		return fmt.Errorf("refresher not initialized")
	}
	return a.refresher.RefreshNow()
}

// Shutdown gracefully stops the application
func (a *App) Shutdown() error {
	a.logger.Info("Shutting down application...")

	// Stop refresher
	if a.refresher != nil {
		if err := a.refresher.Stop(); err != nil {
			a.logger.Error("Failed to stop refresher", zap.Error(err))
		}
	}

	// Close data sources
	if a.dataSource != nil {
		if err := a.dataSource.Close(); err != nil {
			a.logger.Error("Failed to close data source", zap.Error(err))
		}
	}

	// Sync only flushes buffered log entries, ignore stderr sync errors
	_ = a.logger.Sync()
	return nil
}

// initLogger initializes the zap logger with file rotation support
func initLogger(levelStr, logFile string) (*zap.Logger, error) {
	// Parse log level
	level := zapcore.InfoLevel
	switch levelStr {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	default:
		level = zapcore.InfoLevel
	}

	// Configure encoder
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	// Create core with multiple outputs
	var cores []zapcore.Core

	// File output with rotation (required for TUI apps)
	if logFile == "" {
		logFile = "/tmp/k8s-monitor.log" // Default log file
	}

	fileEncoder := zapcore.NewJSONEncoder(encoderConfig)
	fileWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    100, // MB
		MaxBackups: 3,
		MaxAge:     7, // days
		Compress:   true,
	})
	fileCore := zapcore.NewCore(fileEncoder, fileWriter, level)
	cores = append(cores, fileCore)

	// NOTE: Do NOT output to stderr/stdout in TUI mode
	// Bubble Tea requires full control of terminal output

	// Combine cores
	core := zapcore.NewTee(cores...)
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))

	// Set as global logger
	zap.ReplaceGlobals(logger)

	return logger, nil
}
