package agent

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
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

// Execute runs a script with CIDR as $1 and description as $2.
// Returns the combined stdout/stderr output and whether it succeeded (exit code 0).
func (e *Executor) Execute(scriptPath, cidr, description string) ExecutionResult {
	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.shell, scriptPath, cidr, description)

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
