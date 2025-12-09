package logger

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Config holds the configuration for the logger.
type Config struct {
	LogPath    string `json:"log_path"`
	Debug      bool   `json:"debug"`
	MaxSize    int    `json:"max_size"` // in megabytes
	MaxBackups int    `json:"max_backups"`
	MaxAge     int    `json:"max_age"` // in days
	Compress   bool   `json:"compress"`
	Console    bool   `json:"console"`
}

// Init initializes the global logger with zap.
func Init(cfg *Config) {
	if cfg == nil {
		cfg = &Config{
			LogPath: "ssh-manager.log",
			Debug:   false,
			Console: true,
		}
	}

	// Set defaults if zero values
	if cfg.LogPath == "" {
		cfg.LogPath = "ssh-manager.log"
	}
	if cfg.MaxSize == 0 {
		cfg.MaxSize = 10
	}
	if cfg.MaxBackups == 0 {
		cfg.MaxBackups = 3
	}
	if cfg.MaxAge == 0 {
		cfg.MaxAge = 28
	}

	// 1. Encoder Configuration
	// Console Encoder for terminal (colored, human readable)
	consoleEncoderConfig := zap.NewDevelopmentEncoderConfig()
	consoleEncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	consoleEncoder := zapcore.NewConsoleEncoder(consoleEncoderConfig)

	// File Encoder
	fileEncoderConfig := zap.NewProductionEncoderConfig()
	fileEncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	fileEncoder := zapcore.NewConsoleEncoder(fileEncoderConfig)

	// 2. Writer Configuration
	// Stdout
	consoleWriter := zapcore.Lock(os.Stdout)

	// File with rotation (Lumberjack)
	fileWriter := zapcore.AddSync(&lumberjack.Logger{
		Filename:   cfg.LogPath,
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   cfg.Compress,
	})

	// 3. Level Configuration
	level := zap.InfoLevel
	if cfg.Debug {
		level = zap.DebugLevel
	}

	// 4. Core construction
	var cores []zapcore.Core
	if cfg.Console {
		cores = append(cores, zapcore.NewCore(consoleEncoder, consoleWriter, level))
	}
	cores = append(cores, zapcore.NewCore(fileEncoder, fileWriter, level))

	core := zapcore.NewTee(cores...)

	// 5. Build Logger
	// AddCaller skips 1 caller level so it shows the actual call site
	// AddCaller is good.
	logger := zap.New(core, zap.AddCaller())

	// 6. Set Global
	zap.ReplaceGlobals(logger)

	zap.L().Info("Logger initialized",
		zap.String("path", cfg.LogPath),
		zap.Bool("debug", cfg.Debug),
		zap.Bool("console", cfg.Console),
	)
}
