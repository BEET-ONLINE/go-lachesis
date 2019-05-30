package common

import (
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
)

// This can be used as the destination for a logger and it'll
// map them into calls to testing.T.Log, so that you only see
// the logging for failed tests.
type testLoggerAdapter struct {
	t      testing.TB
	prefix string
}

func (a *testLoggerAdapter) Write(d []byte) (int, error) {
	if d[len(d)-1] == '\n' {
		d = d[:len(d)-1]
	}
	if a.prefix != "" {
		l := a.prefix + ": " + string(d)
		a.t.Log(l)
		return len(l), nil
	}
	a.t.Log(string(d))
	return len(d), nil
}

// NewTestLogger constructor
func NewTestLogger(t testing.TB) *logrus.Logger {
	logger, _ := test.NewNullLogger()
	// todo problem data race by a function call t.Log (testing.TB.Log)
	// logger.Out = &testLoggerAdapter{t: t}
	logger.Level = logrus.DebugLevel
	return logger
}
