package middleware

import (
	"fmt"
	"github.com/oryca/oryca/control-plane/logger"
	"github.com/oryca/oryca/control-plane/model"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

var skipLogPaths = map[string]bool{
	"/health": true,
	"/":       true,
}

func AccessLog() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()

			if skipLogPaths[req.URL.Path] {
				return next(c)
			}

			traceID := resolveTraceID(req.Header.Get("X-Trace-ID"))
			c.Set("traceId", traceID)
			c.Response().Header().Set("X-Trace-ID", traceID)

			start := time.Now()
			err := next(c)

			writeAccessLog(c, traceID, time.Since(start).Milliseconds())
			return err
		}
	}
}

func resolveTraceID(headerVal string) string {
	if headerVal != "" {
		return headerVal
	}
	return uuid.NewString()
}

func writeAccessLog(c echo.Context, traceID string, ms int64) {
	req := c.Request()
	status := c.Response().Status

	userID := ""
	if u, ok := c.Get("user").(*model.User); ok && u != nil {
		userID = u.ID.Hex()
	}

	compact := fmt.Sprintf("%d %s %s  %dms  trace=%s", status, req.Method, req.URL.Path, ms, traceID)
	if userID != "" {
		compact += "  user=" + userID
	}
	logCompact(status, compact)

	fields := []zap.Field{
		zap.String("traceId", traceID),
		zap.String("method", req.Method),
		zap.String("path", req.URL.Path),
		zap.Int("status", status),
		zap.Int64("durationMs", ms),
		zap.String("ip", c.RealIP()),
		zap.String("userAgent", req.UserAgent()),
		zap.Int64("responseSize", c.Response().Size),
	}
	if userID != "" {
		fields = append(fields, zap.String("userId", userID))
	}
	logger.RequestLog(status, fields...)
}

func logCompact(status int, line string) {
	switch {
	case status >= 500:
		logger.Error(line)
	case status >= 400:
		logger.Warn(line)
	default:
		logger.Info(line)
	}
}
