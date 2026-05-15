package harness

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

const defaultStartupTimeout = 10 * time.Second

type ServiceDefinition struct {
	Name      string
	ConfigRel string
	AdminBase string
	GRPCAddr  string
}

type Runtime struct {
	projectRoot string
	logsDir     string

	mu       sync.Mutex
	services map[string]*exec.Cmd
}

func NewRuntime(projectRoot, logsDir string) *Runtime {
	return &Runtime{
		projectRoot: projectRoot,
		logsDir:     logsDir,
		services:    make(map[string]*exec.Cmd),
	}
}

func (r *Runtime) StartService(ctx context.Context, def ServiceDefinition) error {
	r.mu.Lock()
	if _, exists := r.services[def.Name]; exists {
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()

	if err := os.MkdirAll(r.logsDir, 0o755); err != nil {
		return fmt.Errorf("create logs directory %s: %w", r.logsDir, err)
	}

	logPath := filepath.Join(r.logsDir, def.Name+".log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open service log %s: %w", logPath, err)
	}

	configPath := filepath.Join(r.projectRoot, def.ConfigRel)
	cmd := exec.CommandContext(ctx, filepath.Join(r.projectRoot, "bin", def.Name), "-config", configPath)
	cmd.Dir = r.projectRoot
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("start %s: %w", def.Name, err)
	}

	r.mu.Lock()
	r.services[def.Name] = cmd
	r.mu.Unlock()

	if err := r.waitForHealthy(def.AdminBase + "/healthz"); err != nil {
		_ = r.StopService(context.Background(), def.Name, 2*time.Second)
		return fmt.Errorf("wait for %s healthz: %w", def.Name, err)
	}

	return nil
}

func (r *Runtime) StopService(ctx context.Context, name string, timeout time.Duration) error {
	r.mu.Lock()
	cmd, exists := r.services[name]
	if exists {
		delete(r.services, name)
	}
	r.mu.Unlock()
	if !exists {
		return nil
	}

	if cmd.Process == nil {
		return nil
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("send SIGTERM to %s: %w", name, err)
	}

	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("wait %s: %w", name, err)
		}
		return nil
	case <-ctx.Done():
		_ = cmd.Process.Kill()
		return fmt.Errorf("stop %s: %w", name, ctx.Err())
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return fmt.Errorf("stop %s: timeout", name)
	}
}

func (r *Runtime) StopAll(ctx context.Context, timeout time.Duration) error {
	r.mu.Lock()
	names := make([]string, 0, len(r.services))
	for name := range r.services {
		names = append(names, name)
	}
	r.mu.Unlock()

	var firstErr error
	for _, name := range names {
		if err := r.StopService(ctx, name, timeout); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (r *Runtime) waitForHealthy(healthURL string) error {
	deadline := time.Now().Add(defaultStartupTimeout)
	client := &http.Client{Timeout: 500 * time.Millisecond}

	for {
		req, reqErr := http.NewRequestWithContext(context.Background(), http.MethodGet, healthURL, nil)
		if reqErr != nil {
			return fmt.Errorf("build request: %w", reqErr)
		}
		resp, err := client.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}

		if time.Now().After(deadline) {
			return fmt.Errorf("service is not healthy at %s before timeout", healthURL)
		}

		time.Sleep(150 * time.Millisecond)
	}
}
