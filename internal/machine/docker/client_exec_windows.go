package docker

import "os"

// notifyResizeSignal is a no-op on Windows, which does not deliver terminal
// resize notifications via signals. The initial window size is still sent.
func notifyResizeSignal(ch chan<- os.Signal) {}
