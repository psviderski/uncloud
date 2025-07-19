package corroservice

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const (
	DefaultCommand = "corrosion"
	DefaultDataDir = "/var/lib/uncloud/corrosion"
)

// SubprocessService implements the Service interface by running the service as a subprocess.
type SubprocessService struct {
	Command string
	DataDir string

	cmd         *exec.Cmd
	running     bool
	mu          sync.Mutex
	cancelWatch context.CancelFunc
}

func DefaultSubprocessService() *SubprocessService {
	return &SubprocessService{
		Command: DefaultCommand,
		DataDir: DefaultDataDir,
	}
}

// TODO: maybe stop the process if this ctx is cancelled.
func (s *SubprocessService) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return nil
	}
	return s.startProcess(ctx)
}

func (s *SubprocessService) Restart(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		if err := s.stopProcess(); err != nil {
			return fmt.Errorf("stop process: %w", err)
		}
	}
	return s.startProcess(ctx)
}

func (s *SubprocessService) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *SubprocessService) startProcess(ctx context.Context) error {
	s.cmd = exec.Command(s.Command, "agent", "-c", filepath.Join(s.DataDir, "config.toml"))

	// Redirect stdout and stderr to the logger.
	stdout, err := s.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create stdout pipe: %w", err)
	}
	stderr, err := s.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("create stderr pipe: %w", err)
	}

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			slog.Info("[corrosion]: " + scanner.Text())
		}
		// TODO: remove
		slog.Info("######## corrosion redirect go routine end ########")
	}()

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			slog.Error("[corrosion]: " + scanner.Text())
		}
	}()

	if err = s.cmd.Start(); err != nil {
		return fmt.Errorf("start process: %w", err)
	}
	s.running = true

	// Watch for process exit to update running status.
	go func() {
		if err := s.cmd.Wait(); err != nil {
			slog.Error("corrosion process exited with error.", "code", s.cmd.ProcessState.ExitCode(), "err", err)
		}

		s.mu.Lock()
		s.running = false
		s.mu.Unlock()
	}()

	// TODO: figure out the waiting process
	// Wait for initialization
	// timer := time.NewTimer(2 * time.Second)
	// defer timer.Stop()

	//select {
	////case <-timer.C:
	////	s.running = true
	////	return nil
	//case <-watchCtx.Done():
	//	return fmt.Errorf("process failed to start")
	//case <-ctx.Done():
	//	s.stopProcess()
	//	return ctx.Err()
	//}
	return nil
}

func (s *SubprocessService) stopProcess() error {
	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}

	if err := s.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("send SIGTERM: %w", err)
	}

	// Wait up to 5 seconds for graceful shutdown before killing the process.
	done := make(chan error, 1)
	go func() {
		done <- s.cmd.Wait()
	}()

	select {
	case <-time.After(5 * time.Second):
		if err := s.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("kill process: %w", err)
		}
	case err := <-done:
		if err != nil {
			return fmt.Errorf("process exited with error: %w", err)
		}
	}

	return nil
}
