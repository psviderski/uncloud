package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/ipfs/go-log/v2"
)

// ipfsLogger is an slog logger that implements the IPFS go-log StandardLogger interface.
type ipfsLogger struct {
	log slog.Logger
}

func newIPFSLogger(l *slog.Logger) *ipfsLogger {
	return &ipfsLogger{log: *l}
}

func (l *ipfsLogger) Debug(args ...any) {
	l.log.Debug(fmt.Sprint(args...))
}

func (l *ipfsLogger) Debugf(format string, args ...any) {
	l.log.Debug(fmt.Sprintf(format, args...))
}

func (l *ipfsLogger) Error(args ...any) {
	l.log.Error(fmt.Sprint(args...))
}

func (l *ipfsLogger) Errorf(format string, args ...any) {
	l.log.Error(fmt.Sprintf(format, args...))
}

func (l *ipfsLogger) Fatal(args ...any) {
	l.log.Error(fmt.Sprint(args...))
	os.Exit(1)
}

func (l *ipfsLogger) Fatalf(format string, args ...any) {
	l.log.Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}

func (l *ipfsLogger) Info(args ...any) {
	l.log.Info(fmt.Sprint(args...))
}

func (l *ipfsLogger) Infof(format string, args ...any) {
	l.log.Info(fmt.Sprintf(format, args...))
}

func (l *ipfsLogger) Panic(args ...any) {
	msg := fmt.Sprint(args...)
	l.log.Error(msg)
	panic(msg)
}

func (l *ipfsLogger) Panicf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	l.log.Error(msg)
	panic(msg)
}

func (l *ipfsLogger) Warn(args ...any) {
	l.log.Warn(fmt.Sprint(args...))
}

func (l *ipfsLogger) Warnf(format string, args ...any) {
	l.log.Warn(fmt.Sprintf(format, args...))
}

var _ log.StandardLogger = (*ipfsLogger)(nil)
