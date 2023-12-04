package process

import (
	"os"
	"path/filepath"

	"github.com/catalogfi/cobi/utils"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// review :
//  1. give a bit explanation about the `uid` input, it's not very clear what it is without diving into the actual
//     inplementation.
//  2. Think about this way, if you want to create a file logger, what are the things you want to customise.
//     We want to let the user choose which file they want to log to.
//     Maybe the log level, cause it's common we use Debug in testing/development, but not in production.
//     So we can have the function interface like this
//     >  NewFileLogger(file os.*File, level zapcore.Level) *zap.Logger
//     And we can have another function to cover our specific usecase
//     > CobiAccountLogFile(uid string) (file os.*File, error)
func NewFileLogger(uid string) *zap.Logger {
	loggerConfig := zap.NewProductionEncoderConfig()
	loggerConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	fileEncoder := zapcore.NewJSONEncoder(loggerConfig)
	// review : don't ignore the error
	logFile, _ := os.OpenFile(filepath.Join(utils.DefaultCobiLogs(), LogFile(uid)), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	writer := zapcore.AddSync(logFile)
	defaultLogLevel := zapcore.DebugLevel
	core := zapcore.NewTee(
		zapcore.NewCore(fileEncoder, writer, defaultLogLevel),
	)
	logger := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zapcore.ErrorLevel))
	return logger
}
