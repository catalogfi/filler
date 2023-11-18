package process

import (
	"os"
	"path/filepath"

	"github.com/catalogfi/cobi/utils"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func NewFileLogger(uid string) *zap.Logger {
	loggerConfig := zap.NewProductionEncoderConfig()
	loggerConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	fileEncoder := zapcore.NewJSONEncoder(loggerConfig)
	logFile, _ := os.OpenFile(filepath.Join(utils.DefaultCobiLogs(), LogFile(uid)), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	writer := zapcore.AddSync(logFile)
	defaultLogLevel := zapcore.DebugLevel
	core := zapcore.NewTee(
		zapcore.NewCore(fileEncoder, writer, defaultLogLevel),
	)
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	return logger
}
