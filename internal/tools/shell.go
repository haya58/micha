package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"dis-llm-bot/internal/textutil"
)

// ExecShell runs a command in a disposable container with NO network.
// Prototype: uses `docker run --rm --network none` via CLI so no docker SDK dep.
// Falls back to direct local execution only when docker binary is missing AND
// the command is read-only-ish; otherwise returns an error.
func ExecShell(ctx context.Context, image, command string, timeoutSec int) (string, error) {
	if timeoutSec <= 0 || timeoutSec > 120 {
		timeoutSec = 30
	}
	if image == "" {
		image = "alpine:latest"
	}
	ctx2, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec+5)*time.Second)
	defer cancel()

	dockerBin, err := exec.LookPath("docker")
	if err == nil {
		args := []string{"run", "--rm", "--network", "none",
			"--cap-drop", "ALL", "--user", "65534:65534",
			"--memory", "256m", "--cpus", "0.5",
			image, "sh", "-c", command}
		cmd := exec.CommandContext(ctx2, dockerBin, args...)
		var buf bytes.Buffer
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		runErr := cmd.Run()
		out := textutil.Truncate(buf.String(), 8000)
		if len([]rune(buf.String())) > 8000 {
			out += "\n...[truncated]"
		}
		if runErr != nil {
			return fmt.Sprintf("exit error: %v\n%s", runErr, out), nil
		}
		if out == "" {
			out = "(no output)"
		}
		return out, nil
	}
	// No docker: strict fallback — allow only harmless read-only commands locally.
	return "", fmt.Errorf("docker not found; shell execution requires docker (no-network disposable container)")
}
