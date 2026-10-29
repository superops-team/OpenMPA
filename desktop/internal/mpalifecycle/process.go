package mpalifecycle

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/superops-team/OpenMPA/desktop/internal/dto"
)

type response struct {
	ID    string          `json:"id"`
	OK    bool            `json:"ok"`
	Value json.RawMessage `json:"value"`
	Error *struct {
		Code       string                   `json:"code"`
		HTTPStatus int                      `json:"httpStatus"`
		Diagnostic *dto.OperationDiagnostic `json:"diagnostic,omitempty"`
	} `json:"error"`
}
type engineProcess struct {
	command *exec.Cmd
	stdin   io.WriteCloser
	mu      sync.Mutex
	writing sync.Mutex
	waiting map[string]chan response
	done    chan struct{}
	next    atomic.Uint64
}

func launchEngine(ctx context.Context, python, script string, initialization any) (*engineProcess, *dto.SafeError) {
	command := exec.Command(python, "-I", script)
	configureProcess(command)
	command.Env = []string{"HOME=" + os.Getenv("HOME"), "PATH=/usr/bin:/bin", "LANG=en_US.UTF-8", "PYTHONUNBUFFERED=1"}
	command.Stderr = io.Discard
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, engineFailure()
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		stdin.Close()
		return nil, engineFailure()
	}
	p := &engineProcess{command: command, stdin: stdin, waiting: map[string]chan response{}, done: make(chan struct{})}
	ready := make(chan response, 1)
	p.waiting["init"] = ready
	if err = command.Start(); err != nil {
		stdin.Close()
		stdout.Close()
		return nil, engineFailure()
	}
	go func() {
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 65536), 1<<20)
		for scanner.Scan() {
			var result response
			if json.Unmarshal(scanner.Bytes(), &result) != nil {
				continue
			}
			p.mu.Lock()
			channel := p.waiting[result.ID]
			delete(p.waiting, result.ID)
			p.mu.Unlock()
			if channel != nil {
				channel <- result
			}
		}
		command.Wait()
		close(p.done)
	}()
	if err = json.NewEncoder(stdin).Encode(initialization); err != nil {
		p.close()
		return nil, engineFailure()
	}
	select {
	case result := <-ready:
		if result.OK {
			return p, nil
		}
	case <-ctx.Done():
	case <-p.done:
	}
	p.close()
	return nil, engineFailure()
}
func (p *engineProcess) call(ctx context.Context, operation string, body, output any) *dto.SafeError {
	id := fmt.Sprint(p.next.Add(1))
	channel := make(chan response, 1)
	p.mu.Lock()
	p.waiting[id] = channel
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.waiting, id); p.mu.Unlock() }()
	p.writing.Lock()
	err := json.NewEncoder(p.stdin).Encode(map[string]any{"id": id, "operation": operation, "body": body})
	p.writing.Unlock()
	if err != nil {
		return engineFailure()
	}
	select {
	case result := <-channel:
		if !result.OK {
			if result.Error != nil {
				failure := dto.Failure(result.Error.Code, "platform", "verifyOnly", result.Error.HTTPStatus)
				failure.Diagnostic = result.Error.Diagnostic
				if result.Error.Diagnostic != nil {
					failure.RequestID = result.Error.Diagnostic.RequestID
				}
				return failure
			}
			return engineFailure()
		}
		if json.Unmarshal(result.Value, output) != nil {
			return dto.Failure("invalidResponse", "desktop", "readOnly", 0)
		}
		return nil
	case <-ctx.Done():
		return dto.Failure("outcomeUnknown", "platform", "verifyOnly", 0)
	case <-p.done:
		return dto.Failure("outcomeUnknown", "platform", "verifyOnly", 0)
	}
}
func (p *engineProcess) close() {
	p.writing.Lock()
	_ = json.NewEncoder(p.stdin).Encode(map[string]any{"operation": "shutdown"})
	p.stdin.Close()
	p.writing.Unlock()
	select {
	case <-p.done:
	case <-time.After(8 * time.Second):
		killProcess(p.command)
		<-p.done
	}
}
func engineFailure() *dto.SafeError {
	return dto.Failure("lifecycleUnavailable", "desktop", "readOnly", 0)
}
