package golog

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// Level type
type Level int

// Logging level supported
const (
	DEBUG Level = iota
	INFO
	WARN
	ERROR
)

func (l Level) String() string {
	return [...]string{
		"debug",
		"info",
		"warning",
		"error",
	}[l]
}

var lookupMap = map[string]Level{
	"debug": DEBUG,
	"info":  INFO,
	"warn":  WARN,
	"error": ERROR,
}

// GetLevel returns the input string's corresponding logging Level
func GetLevel(s string) Level {
	return lookupMap[s]
}

func (l Level) toLogrusLevel() logrus.Level {
	switch l {
	case DEBUG:
		return logrus.DebugLevel
	case INFO:
		return logrus.InfoLevel
	case WARN:
		return logrus.WarnLevel
	case ERROR:
		return logrus.ErrorLevel
	default:
		return logrus.InfoLevel
	}
}

// A list of field keys
const (
	TagKey        = "tag"
	ErrorKey      = "error"
	StacktraceKey = "stack_trace" // required by Stackdriver to do error reporting
)

// Logger struct holds the actual 3rd party logger we rely on,
// decouple the users of this package from the specific 3rd party logging lib we are using
type Logger struct {
	logger *logrus.Entry
}

// New creates a new logger
func New(l Level, o io.Writer) Logger {
	logger := logrus.New()
	logger.Formatter = &logrus.JSONFormatter{
		FieldMap: logrus.FieldMap{
			logrus.FieldKeyTime:  "timestamp",
			logrus.FieldKeyLevel: "severity",
			logrus.FieldKeyMsg:   "message",
		},
		TimestampFormat: time.RFC3339Nano,
	}

	logger.SetLevel(l.toLogrusLevel())
	logger.SetOutput(o)

	return Logger{
		logger: logrus.NewEntry(logger),
	}
}

// NewDefault creates a new logger with default level configured in env variable,
// if not set, default to debug
func NewDefault() Logger {
	return New(GetLevel(getEnv("LOGGING_LEVEL", "debug")), os.Stdout)
}

func getEnv(key string, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return strings.ToLower(value)
	}

	return fallback
}

func (l Logger) Debugln(msg string) {
	l.logger.Logln(logrus.DebugLevel, msg)
}

func (l Logger) Infoln(msg string) {
	l.logger.Logln(logrus.InfoLevel, msg)
}

func (l Logger) Warnln(msg string) {
	l.logger.Logln(logrus.WarnLevel, msg)
}

func (l Logger) Errorln(msg string) {
	l.logger.Logln(logrus.ErrorLevel, msg)
}

// WithFields returns a new logger with key value pairs added. Calling this method doesn't
// log anything. Caller has to call Debugln, Infoln, Warnln or Errorln to flush the key value
// pair into a log entry.
func (l Logger) WithFields(fields map[string]interface{}) Logger {
	if val, ok := fields[ErrorKey]; ok {
		fields[StacktraceKey] = fmt.Sprintf("%+v", val)
	}

	return Logger{
		logger: l.logger.WithFields(fields),
	}
}
