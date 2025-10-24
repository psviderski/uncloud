package api

// ExecConfig contains configuration for executing a command in a container.
type ExecConfig struct {
	// Cmd is the command to run in the container.
	Cmd []string
	// AttachStdin attaches the stdin stream to the exec session.
	AttachStdin bool
	// AttachStdout attaches the stdout stream to the exec session.
	AttachStdout bool
	// AttachStderr attaches the stderr stream to the exec session.
	AttachStderr bool
	// Tty allocates a pseudo-TTY for the exec session.
	Tty bool
	// Detach runs the command in the background without attaching to streams.
	Detach bool
	// User specifies the user to run the command as.
	User string
	// Privileged runs the command in privileged mode.
	Privileged bool
	// WorkingDir sets the working directory for the command.
	WorkingDir string
	// Env sets environment variables for the command.
	Env []string
}
