package log

import (
	"bytes"
	"fmt"
	logpkg "log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"
)

const (
	ErrorLevel = 1
	InfoLevel  = 2
	DebugLevel = 3
)

var (
	Level int = 0
	mu    sync.Mutex
	// If Record is not nil, it will be used to send log messages instead
	// of Stderr.
	Record func(...interface{})
)

func Stack() string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

func log(d int, l, s string) {
	fl := "unknown"
	if _, file, line, ok := runtime.Caller(d); ok {
		fl = fmt.Sprintf("%s:%d", filepath.Join(filepath.Base(filepath.Dir(file)), filepath.Base(file)), line)
	}
	t := time.Now().UTC().Format("0102 150405.000")
	if Record != nil {
		Record(fmt.Sprintf("%s%s %s] %s", l, t, fl, s))
		return
	}
	mu.Lock()
	fmt.Fprintf(os.Stderr, "%s%s %s] %s\n", l, t, fl, s)
	mu.Unlock()
}

func Panic(args ...interface{}) {
	m := fmt.Sprint(args...)
	log(2, "PANIC!", m)
	panic(m)
}

func Panicf(format string, args ...interface{}) {
	m := fmt.Sprintf(format, args...)
	log(2, "PANIC!", m)
	panic(m)
}

func Fatal(args ...interface{}) {
	log(2, "F", fmt.Sprint(args...))
	os.Exit(1)
}

func Fatalf(format string, args ...interface{}) {
	log(2, "F", fmt.Sprintf(format, args...))
	os.Exit(1)
}

func Error(args ...interface{}) {
	if Level >= ErrorLevel {
		log(2, "E", fmt.Sprint(args...))
	}
}

func Errorf(format string, args ...interface{}) {
	if Level >= ErrorLevel {
		log(2, "E", fmt.Sprintf(format, args...))
	}
}

func Info(args ...interface{}) {
	if Level >= InfoLevel {
		log(2, "I", fmt.Sprint(args...))
	}
}

func Infof(format string, args ...interface{}) {
	if Level >= InfoLevel {
		log(2, "I", fmt.Sprintf(format, args...))
	}
}

func Debug(args ...interface{}) {
	if Level >= DebugLevel {
		log(2, "D", fmt.Sprint(args...))
	}
}

func Debugf(format string, args ...interface{}) {
	if Level >= DebugLevel {
		log(2, "D", fmt.Sprintf(format, args...))
	}
}

func Logger() *logpkg.Logger {
	return logpkg.New(writer{}, "", 0)
}

type writer struct{}

func (writer) Write(b []byte) (n int, err error) {
	if Level >= InfoLevel {
		b = bytes.TrimSuffix(b, []byte{'\n'})
		// Depth set to work nicely with http/Server.ErrorLog.
		log(5, "L", string(b))
	}
	return len(b), nil
}
