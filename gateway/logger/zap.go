package logger

import (
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var globalLogger *zap.Logger // console (dev: compact, prod: JSON)
var hostname string

func init() {
	hostname, _ = os.Hostname()
}

// Init เรียกครั้งเดียวใน main.go
func Init(serviceName string) {
	stdout := zapcore.AddSync(os.Stdout)
	logFormat := os.Getenv("LOG_FORMAT") // "console" (default) | "json"
	isJSON := logFormat == "json"
	// console logger. Compact one-liner หรือ JSON ขึ้นกับ LOG_FORMAT
	consoleEnc := buildConsoleEncoder(isJSON)
	consoleCore := zapcore.NewCore(consoleEnc, stdout, zap.NewAtomicLevelAt(zap.DebugLevel))
	globalLogger = zap.New(consoleCore, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel)).
		With(zap.String("hostname", hostname), zap.String("service", serviceName))
}

func buildJSONEncoder() zapcore.Encoder {
	cfg := zap.NewProductionEncoderConfig()
	cfg.TimeKey = "time"
	cfg.MessageKey = "message"
	cfg.EncodeLevel = zapcore.CapitalLevelEncoder
	cfg.EncodeTime = func(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
		enc.AppendString(t.UTC().Format("2006-01-02T15:04:05.000Z"))
	}
	return zapcore.NewJSONEncoder(cfg)
}

func buildConsoleEncoder(isJSON bool) zapcore.Encoder {
	if isJSON {
		return buildJSONEncoder()
	}
	cfg := zapcore.EncoderConfig{
		TimeKey:    "time",
		LevelKey:   "level",
		MessageKey: "msg",
		EncodeTime: zapcore.TimeEncoderOfLayout("15:04:05.000"),
		EncodeLevel: func(l zapcore.Level, enc zapcore.PrimitiveArrayEncoder) {
			colors := map[zapcore.Level]string{
				zapcore.DebugLevel: "\033[90mDEBUG\033[0m",
				zapcore.InfoLevel:  "\033[32mINFO\033[0m",
				zapcore.WarnLevel:  "\033[1;33mWARN\033[0m",
				zapcore.ErrorLevel: "\033[1;31mERROR\033[0m",
			}
			if c, ok := colors[l]; ok {
				enc.AppendString(c)
			} else {
				enc.AppendString(l.CapitalString())
			}
		},
	}
	return zapcore.NewConsoleEncoder(cfg)
}

// Info. Backward compat กับ logger.Info(msg string)
func Info(msg string) {
	if globalLogger == nil {
		fmt.Println("[INFO]", msg)
		return
	}
	globalLogger.Info(msg)
}

// Error. Backward compat กับ logger.Error(msg string)
func Error(msg string) {
	if globalLogger == nil {
		fmt.Fprintln(os.Stderr, "[ERROR]", msg)
		return
	}
	globalLogger.Error(msg)
}

// ProxyLog เขียน structured log สำหรับทุก proxy request
// - console: compact one-liner  "200 GET /path  133ms  trace=xxx  user=yyy"
func ProxyLog(f ProxyLogFields) {
	if globalLogger == nil {
		return
	}

	// compact console message. The full structured record goes to Redis Stream
	// separately via BuildProxyLogJSON, so nothing is duplicated here.
	compact := fmt.Sprintf("%d %s %s  %dms  trace=%s", f.StatusCode, f.Method, f.Path, f.TotalMs, f.TraceID)
	if f.UserID != "" {
		compact += "  user=" + f.UserID
	}
	if f.ApiKeyID != "" {
		compact += "  key=" + f.ApiKeyID
	}

	if f.StatusCode >= 500 {
		globalLogger.Error(compact)
		return
	}
	globalLogger.Info(compact)
}
