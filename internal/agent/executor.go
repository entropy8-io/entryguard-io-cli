package agent

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var scriptPrefixRegex = regexp.MustCompile(`^\d+-`)

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

// DiscoverScripts finds scripts in a directory that match the NN- naming convention,
// sorted by numeric prefix. Returns the full paths.
func DiscoverScripts(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read script directory %s: %w", dir, err)
	}

	var scripts []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !scriptPrefixRegex.MatchString(name) {
			continue
		}
		scripts = append(scripts, filepath.Join(dir, name))
	}

	sort.Slice(scripts, func(i, j int) bool {
		return filepath.Base(scripts[i]) < filepath.Base(scripts[j])
	})

	return scripts, nil
}

// ExecuteDir discovers and runs all scripts in a directory sequentially.
// All scripts run regardless of individual failures.
// Returns per-script results and an overall success flag.
func (e *Executor) ExecuteDir(dir, cidr, description string, timeout time.Duration) ([]ScriptResult, bool) {
	scripts, err := DiscoverScripts(dir)
	if err != nil {
		return []ScriptResult{{
			ScriptName: dir,
			Success:    false,
			Output:     err.Error(),
			DurationMs: 0,
		}}, false
	}

	if len(scripts) == 0 {
		return []ScriptResult{{
			ScriptName: dir,
			Success:    false,
			Output:     "No scripts found matching NN- naming convention",
			DurationMs: 0,
		}}, false
	}

	// Use per-script timeout if provided, otherwise use executor default
	scriptTimeout := e.timeout
	if timeout > 0 {
		scriptTimeout = timeout
	}

	var results []ScriptResult
	allSuccess := true

	for _, scriptPath := range scripts {
		start := time.Now()

		ctx, cancel := context.WithTimeout(context.Background(), scriptTimeout)
		cmd := e.buildCommand(ctx, scriptPath, cidr, description)

		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		runErr := cmd.Run()
		duration := time.Since(start)
		cancel()

		output := stdout.String()
		if stderr.Len() > 0 {
			if output != "" {
				output += "\n"
			}
			output += stderr.String()
		}
		if len(output) > 4096 {
			output = output[:4096] + "\n... (truncated)"
		}

		scriptName := filepath.Base(scriptPath)
		sr := ScriptResult{
			ScriptName: scriptName,
			DurationMs: duration.Milliseconds(),
		}

		if ctx.Err() == context.DeadlineExceeded {
			sr.Success = false
			sr.Output = fmt.Sprintf("Script timed out after %s\n%s", scriptTimeout, output)
			allSuccess = false
		} else if runErr != nil {
			sr.Success = false
			sr.Output = fmt.Sprintf("Script failed: %v\n%s", runErr, output)
			allSuccess = false
		} else {
			sr.Success = true
			sr.Output = output
		}

		results = append(results, sr)
	}

	return results, allSuccess
}
