package agent

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Executor struct {
	shell   string
	timeout time.Duration
}

func NewExecutor(shell string, timeout time.Duration) *Executor {
	return &Executor{
		shell:   shell,
		timeout: timeout,
	}
}

// buildCommand creates the appropriate exec.Cmd based on the configured shell.
// PowerShell: -ExecutionPolicy Bypass -File <script> <args...>
// cmd.exe:    /C <script> <args...>
// bash/other: <script> <args...> (positional args)
func (e *Executor) buildCommand(ctx context.Context, scriptPath, cidr, description string) *exec.Cmd {
	shellBase := strings.ToLower(filepath.Base(e.shell))

	switch shellBase {
	case "powershell.exe", "pwsh.exe", "pwsh":
		return exec.CommandContext(ctx, e.shell,
			"-ExecutionPolicy", "Bypass", "-File", scriptPath, cidr, description)
	case "cmd.exe", "cmd":
		return exec.CommandContext(ctx, e.shell,
			"/C", scriptPath, cidr, description)
	default:
		return exec.CommandContext(ctx, e.shell, scriptPath, cidr, description)
	}
}

// Execute runs a script with CIDR as $1 and description as $2.
// Returns the combined stdout/stderr output and whether it succeeded (exit code 0).
func (e *Executor) Execute(scriptPath, cidr, description string) ExecutionResult {
	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := e.buildCommand(ctx, scriptPath, cidr, description)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start)

	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	// Trim to reasonable size for reporting
	if len(output) > 4096 {
		output = output[:4096] + "\n... (truncated)"
	}

	if ctx.Err() == context.DeadlineExceeded {
		return ExecutionResult{
			Success:  false,
			Output:   fmt.Sprintf("Script timed out after %s\n%s", e.timeout, output),
			Duration: duration,
		}
	}

	if err != nil {
		return ExecutionResult{
			Success:  false,
			Output:   fmt.Sprintf("Script failed: %v\n%s", err, output),
			Duration: duration,
		}
	}

	return ExecutionResult{
		Success:  true,
		Output:   output,
		Duration: duration,
	}
}
