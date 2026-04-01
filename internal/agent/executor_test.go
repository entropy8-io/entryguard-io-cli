package agent

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestDiscoverScripts(t *testing.T) {
	dir := t.TempDir()

	// Create valid scripts with NN- prefix
	files := []string{
		"02-second.sh",
		"01-first.sh",
		"10-tenth.sh",
	}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("#!/bin/bash\necho ok"), 0755); err != nil {
			t.Fatal(err)
		}
	}

	// Create files that should be skipped (no NN- prefix)
	skipFiles := []string{
		"README.md",
		"helper.sh",
		".hidden",
	}
	for _, f := range skipFiles {
		if err := os.WriteFile(filepath.Join(dir, f), []byte("skip"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	// Create a subdirectory (should be skipped)
	if err := os.Mkdir(filepath.Join(dir, "01-subdir"), 0755); err != nil {
		t.Fatal(err)
	}

	scripts, err := DiscoverScripts(dir)
	if err != nil {
		t.Fatalf("DiscoverScripts failed: %v", err)
	}

	if len(scripts) != 3 {
		t.Fatalf("expected 3 scripts, got %d: %v", len(scripts), scripts)
	}

	// Check sorted order
	expectedOrder := []string{"01-first.sh", "02-second.sh", "10-tenth.sh"}
	for i, expected := range expectedOrder {
		if filepath.Base(scripts[i]) != expected {
			t.Errorf("script[%d]: expected %s, got %s", i, expected, filepath.Base(scripts[i]))
		}
	}
}

func TestDiscoverScripts_emptyDir(t *testing.T) {
	dir := t.TempDir()

	scripts, err := DiscoverScripts(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(scripts) != 0 {
		t.Fatalf("expected 0 scripts, got %d", len(scripts))
	}
}

func TestDiscoverScripts_nonExistentDir(t *testing.T) {
	_, err := DiscoverScripts("/nonexistent/path")
	if err == nil {
		t.Fatal("expected error for non-existent directory")
	}
}

func TestExecuteDir_allSucceed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	dir := t.TempDir()
	applyDir := filepath.Join(dir, "apply")
	if err := os.Mkdir(applyDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create two scripts that succeed
	script1 := "#!/bin/bash\necho \"applied $1\"\n"
	script2 := "#!/bin/bash\necho \"done $1\"\n"

	os.WriteFile(filepath.Join(applyDir, "01-first.sh"), []byte(script1), 0755)
	os.WriteFile(filepath.Join(applyDir, "02-second.sh"), []byte(script2), 0755)

	executor := NewExecutor("/bin/bash", 10*time.Second)
	results, allSuccess := executor.ExecuteDir(applyDir, "10.0.0.1/32", "test session", 0)

	if !allSuccess {
		t.Error("expected allSuccess to be true")
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	if results[0].ScriptName != "01-first.sh" {
		t.Errorf("expected first script name 01-first.sh, got %s", results[0].ScriptName)
	}
	if !results[0].Success {
		t.Errorf("expected first script to succeed: %s", results[0].Output)
	}

	if results[1].ScriptName != "02-second.sh" {
		t.Errorf("expected second script name 02-second.sh, got %s", results[1].ScriptName)
	}
	if !results[1].Success {
		t.Errorf("expected second script to succeed: %s", results[1].Output)
	}
}

func TestExecuteDir_partialFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	dir := t.TempDir()
	applyDir := filepath.Join(dir, "apply")
	if err := os.Mkdir(applyDir, 0755); err != nil {
		t.Fatal(err)
	}

	// First script succeeds, second fails, third succeeds
	os.WriteFile(filepath.Join(applyDir, "01-ok.sh"), []byte("#!/bin/bash\necho ok"), 0755)
	os.WriteFile(filepath.Join(applyDir, "02-fail.sh"), []byte("#!/bin/bash\nexit 1"), 0755)
	os.WriteFile(filepath.Join(applyDir, "03-ok.sh"), []byte("#!/bin/bash\necho ok"), 0755)

	executor := NewExecutor("/bin/bash", 10*time.Second)
	results, allSuccess := executor.ExecuteDir(applyDir, "10.0.0.1/32", "test", 0)

	if allSuccess {
		t.Error("expected allSuccess to be false")
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// All scripts should have run regardless of failures
	if !results[0].Success {
		t.Error("first script should have succeeded")
	}
	if results[1].Success {
		t.Error("second script should have failed")
	}
	if !results[2].Success {
		t.Error("third script should have succeeded (runs despite earlier failure)")
	}
}

func TestExecuteDir_noMatchingScripts(t *testing.T) {
	dir := t.TempDir()

	// Create files without NN- prefix
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("no scripts here"), 0644)

	executor := NewExecutor("/bin/bash", 10*time.Second)
	results, allSuccess := executor.ExecuteDir(dir, "10.0.0.1/32", "test", 0)

	if allSuccess {
		t.Error("expected allSuccess to be false when no scripts found")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 error result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("expected error result to be failure")
	}
}

func TestExecuteDir_customTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "01-slow.sh"), []byte("#!/bin/bash\nsleep 10"), 0755)

	executor := NewExecutor("/bin/bash", 30*time.Second)
	results, allSuccess := executor.ExecuteDir(dir, "10.0.0.1/32", "test", 1*time.Second)

	if allSuccess {
		t.Error("expected timeout failure")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Success {
		t.Error("expected script to fail due to timeout")
	}
}

func TestExecute_singleScript(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "test.sh")
	os.WriteFile(scriptPath, []byte("#!/bin/bash\necho \"applied $1 $2\""), 0755)

	executor := NewExecutor("/bin/bash", 10*time.Second)
	result := executor.Execute(scriptPath, "10.0.0.1/32", "test session")

	if !result.Success {
		t.Errorf("expected success, got: %s", result.Output)
	}
	if result.Output == "" {
		t.Error("expected non-empty output")
	}
}
