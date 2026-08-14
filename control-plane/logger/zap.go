package logger

import (
	"fmt"
	"os"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var globalLogger *zap.Logger // console — compact in dev, JSON in prod
var structLogger *zap.Logger // JSON structured — แยกจาก console log
var hostname string

func init() {
	hostname, _ = os.Hostname()
}

func Init(serviceName string) {
	stdout := zapcore.AddSync(os.Stdout)
	logFormat := os.Getenv("LOG_FORMAT") // "console" (default) | "json"
	isJSON := logFormat == "json"

	base := []zap.Field{zap.String("hostname", hostname), zap.String("service", serviceName)}

	// console logger. Compact one-liner หรือ JSON ขึ้นกับ LOG_FORMAT
	consoleCore := zapcore.NewCore(buildConsoleEncoder(isJSON), stdout, zap.NewAtomicLevelAt(zap.DebugLevel))
	globalLogger = zap.New(consoleCore, zap.AddCaller(), zap.AddCallerSkip(1), zap.AddStacktrace(zapcore.ErrorLevel))

	// structured logger. เขียน JSON ลง stdout (คนละ encoder กับ console) ให้ collector
	// ข้างนอกเก็บต่อได้; access log แบบอ่านง่ายยังออกทาง globalLogger เหมือนเดิม
	if isJSON {
		structCore := zapcore.NewCore(buildJSONEncoder(), stdout, zap.NewAtomicLevelAt(zap.InfoLevel))
		structLogger = zap.New(structCore, zap.AddCaller(), zap.AddCallerSkip(1)).With(base...)
	} else {
		// console mode. Console logger พิมพ์ครบอยู่แล้ว ไม่ต้องซ้ำ
		structLogger = zap.NewNop()
	}
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

func Info(msg string) {
	if globalLogger == nil {
		fmt.Println("[INFO]", msg)
		return
	}
	globalLogger.Info(msg)
}

func Error(msg string) {
	if globalLogger == nil {
		fmt.Fprintln(os.Stderr, "[ERROR]", msg)
		return
	}
	globalLogger.Error(msg)
}

func Warn(msg string) {
	if globalLogger == nil {
		fmt.Fprintln(os.Stderr, "[WARN]", msg)
		return
	}
	globalLogger.Warn(msg)
}

// With returns a logger with extra fields
func With(fields ...zap.Field) *zap.Logger {
	if globalLogger == nil {
		return zap.NewNop()
	}
	return globalLogger.With(fields...)
}

// RequestLog writes full structured fields to structLogger (JSON)
func RequestLog(status int, fields ...zap.Field) {
	if structLogger == nil {
		return
	}
	switch {
	case status >= 500:
		structLogger.Error("request", fields...)
	case status >= 400:
		structLogger.Warn("request", fields...)
	default:
		structLogger.Info("request", fields...)
	}
}

// EventLog writes a structured domain/audit event (เช่น token issued, session terminated) ไป pipeline
// เดียวกับ RequestLog (JSON). ใช้ field "event" แยกจาก HTTP access log เพื่อทำ analytics ต่อได้
func EventLog(event string, fields ...zap.Field) {
	if structLogger == nil {
		return
	}
	structLogger.Info("event", append([]zap.Field{zap.String("event", event)}, fields...)...)
}
