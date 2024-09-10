package sshexec

import (
	"context"
	"regexp"
	"strings"
)

type Executor interface {
	Run(ctx context.Context, cmd string) (string, error)
	Close() error
}

// Quote* functions are copied from github.com/alessio/shellescape package.
var pattern = regexp.MustCompile(`[^\w@%+=:,./-]`)

// Quote returns a shell-escaped version of the string s. The returned value
// is a string that can safely be used as one token in a shell command line.
func Quote(s string) string {
	if len(s) == 0 {
		return "''"
	}

	if pattern.MatchString(s) {
		return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
	}

	return s
}

// QuoteCommand returns a shell-escaped version of the command arguments.
// The returned value is a string that can safely be used as shell command arguments.
func QuoteCommand(args ...string) string {
	l := make([]string, len(args))

	for i, s := range args {
		l[i] = Quote(s)
	}

	return strings.Join(l, " ")
}
