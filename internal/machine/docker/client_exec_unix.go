//go:build !windows

package docker

import (
	"os"
	"os/signal"

	"golang.org/x/sys/unix"
)

// notifyResizeSignal registers ch to receive terminal window resize (SIGWINCH) signals.
func notifyResizeSignal(ch chan<- os.Signal) {
	signal.Notify(ch, unix.SIGWINCH)
}
