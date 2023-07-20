//
// Copyright 2021-2022 TTBT Enterprises LLC
//
// This file is part of c2FmZQ (https://c2FmZQ.org/).
//
// c2FmZQ is free software: you can redistribute it and/or modify it under the
// terms of the GNU General Public License as published by the Free Software
// Foundation, either version 3 of the License, or (at your option) any later
// version.
//
// c2FmZQ is distributed in the hope that it will be useful, but WITHOUT ANY
// WARRANTY; without even the implied warranty of MERCHANTABILITY or FITNESS FOR
// A PARTICULAR PURPOSE. See the GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License along with
// c2FmZQ. If not, see <https://www.gnu.org/licenses/>.

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

var internalLogger = &Logger{skip: 1}

func Stack() string {
	buf := make([]byte, 4096)
	n := runtime.Stack(buf, false)
	return string(buf[:n])
}

func DefaultLogger() *Logger {
	return &Logger{}
}

type Logger struct {
	skip int
}

func (l *Logger) log(d int, level, s string) {
	fl := "unknown"
	if _, file, line, ok := runtime.Caller(d + l.skip); ok {
		fl = fmt.Sprintf("%s:%d", filepath.Join(filepath.Base(filepath.Dir(file)), filepath.Base(file)), line)
	}
	t := time.Now().UTC().Format("0102 150405.000")
	if Record != nil {
		Record(fmt.Sprintf("%s%s %s] %s", level, t, fl, s))
		return
	}
	mu.Lock()
	fmt.Fprintf(os.Stderr, "%s%s %s] %s\n", level, t, fl, s)
	mu.Unlock()
}

func Panic(args ...interface{}) {
	internalLogger.Panic(args...)
}

func (l *Logger) Panic(args ...interface{}) {
	m := fmt.Sprint(args...)
	l.log(2, "PANIC!", m)
	panic(m)
}

func Panicf(format string, args ...interface{}) {
	internalLogger.Panicf(format, args...)
}

func (l *Logger) Panicf(format string, args ...interface{}) {
	m := fmt.Sprintf(format, args...)
	l.log(2, "PANIC!", m)
	panic(m)
}

func Fatal(args ...interface{}) {
	internalLogger.Fatal(args...)
}

func (l *Logger) Fatal(args ...interface{}) {
	l.log(2, "F", fmt.Sprint(args...))
	os.Exit(1)
}

func Fatalf(format string, args ...interface{}) {
	internalLogger.Fatalf(format, args...)
}

func (l *Logger) Fatalf(format string, args ...interface{}) {
	l.log(2, "F", fmt.Sprintf(format, args...))
	os.Exit(1)
}

func Error(args ...interface{}) {
	internalLogger.Error(args...)
}

func (l *Logger) Error(args ...interface{}) {
	if Level >= ErrorLevel {
		l.log(2, "E", fmt.Sprint(args...))
	}
}

func Errorf(format string, args ...interface{}) {
	internalLogger.Errorf(format, args...)
}

func (l *Logger) Errorf(format string, args ...interface{}) {
	if Level >= ErrorLevel {
		l.log(2, "E", fmt.Sprintf(format, args...))
	}
}

func Info(args ...interface{}) {
	internalLogger.Info(args...)
}

func (l *Logger) Info(args ...interface{}) {
	if Level >= InfoLevel {
		l.log(2, "I", fmt.Sprint(args...))
	}
}

func Infof(format string, args ...interface{}) {
	internalLogger.Infof(format, args...)
}

func (l *Logger) Infof(format string, args ...interface{}) {
	if Level >= InfoLevel {
		l.log(2, "I", fmt.Sprintf(format, args...))
	}
}

func Debug(args ...interface{}) {
	internalLogger.Debug(args...)
}

func (l *Logger) Debug(args ...interface{}) {
	if Level >= DebugLevel {
		l.log(2, "D", fmt.Sprint(args...))
	}
}

func Debugf(format string, args ...interface{}) {
	internalLogger.Debugf(format, args...)
}

func (l *Logger) Debugf(format string, args ...interface{}) {
	if Level >= DebugLevel {
		l.log(2, "D", fmt.Sprintf(format, args...))
	}
}

func GoLogger() *logpkg.Logger {
	return logpkg.New(writer{}, "", 0)
}

type writer struct{}

func (writer) Write(b []byte) (n int, err error) {
	if Level >= InfoLevel {
		b = bytes.TrimSuffix(b, []byte{'\n'})
		// Depth set to work nicely with http/Server.ErrorLog.
		internalLogger.log(5, "L", string(b))
	}
	return len(b), nil
}
