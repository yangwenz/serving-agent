package utils

import (
	"github.com/rs/zerolog"
	"time"
)

// See: https://cloud.google.com/logging/docs/reference/v2/rest/v2/LogEntry#LogSeverity
var logLevelSeverity = map[zerolog.Level]string{
	zerolog.DebugLevel: "DEBUG",
	zerolog.InfoLevel:  "INFO",
	zerolog.WarnLevel:  "WARNING",
	zerolog.ErrorLevel: "ERROR",
	zerolog.PanicLevel: "CRITICAL",
	zerolog.FatalLevel: "CRITICAL",
}

func InitZerolog() {
	logLevel := zerolog.InfoLevel
	zerolog.SetGlobalLevel(logLevel)

	zerolog.LevelFieldName = "severity"
	zerolog.LevelFieldMarshalFunc = func(l zerolog.Level) string {
		return logLevelSeverity[l]
	}
	zerolog.TimestampFieldName = "time"
	zerolog.TimeFieldFormat = time.RFC3339Nano
}
