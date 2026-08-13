package plugin

import (
	"io"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// testConsoleLogger builds a console logger for tests. verbosity controls the
// level: 0 = warn, 1 = info, 2+ = debug. quiet limits output to errors.
func testConsoleLogger(stderr io.Writer, verbosity int, quiet bool) *zap.Logger {
	if stderr == nil {
		stderr = io.Discard
	}
	level := zap.WarnLevel
	switch {
	case quiet:
		level = zap.ErrorLevel
	case verbosity >= 2:
		level = zap.DebugLevel
	case verbosity == 1:
		level = zap.InfoLevel
	}
	encoder := zapcore.NewConsoleEncoder(zap.NewDevelopmentEncoderConfig())
	return zap.New(zapcore.NewCore(encoder, zapcore.AddSync(stderr), level))
}
