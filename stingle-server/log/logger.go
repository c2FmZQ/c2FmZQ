package log

import (
	"fmt"
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
)

func log(l, s string) {
	fl := "unknown"
	if _, file, line, ok := runtime.Caller(2); ok {
		fl = fmt.Sprintf("%s:%d", filepath.Join(filepath.Base(filepath.Dir(file)), filepath.Base(file)), line)
	}
	t := time.Now().UTC().Format("20060102-150405.000")
	mu.Lock()
	fmt.Fprintf(os.Stderr, "%s%s %s] %s\n", l, t, fl, s)
	mu.Unlock()
}

func Panic(args ...interface{}) {
	m := fmt.Sprint(args...)
	log("PANIC!", m)
	panic(m)
}

func Panicf(format string, args ...interface{}) {
	m := fmt.Sprintf(format, args...)
	log("PANIC!", m)
	panic(m)
}

func Fatal(args ...interface{}) {
	log("F", fmt.Sprint(args...))
	os.Exit(1)
}

func Fatalf(format string, args ...interface{}) {
	log("F", fmt.Sprintf(format, args...))
	os.Exit(1)
}

func Error(args ...interface{}) {
	if Level >= ErrorLevel {
		log("E", fmt.Sprint(args...))
	}
}

func Errorf(format string, args ...interface{}) {
	if Level >= ErrorLevel {
		log("E", fmt.Sprintf(format, args...))
	}
}

func Info(args ...interface{}) {
	if Level >= InfoLevel {
		log("I", fmt.Sprint(args...))
	}
}

func Infof(format string, args ...interface{}) {
	if Level >= InfoLevel {
		log("I", fmt.Sprintf(format, args...))
	}
}

func Debug(args ...interface{}) {
	if Level >= DebugLevel {
		log("D", fmt.Sprint(args...))
	}
}

func Debugf(format string, args ...interface{}) {
	if Level >= DebugLevel {
		log("D", fmt.Sprintf(format, args...))
	}
}
