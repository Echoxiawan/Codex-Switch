package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

// RunCodexLogin 执行 `codex login` 命令，返回 stdout/stderr/退出码。
func RunCodexLogin(ctx context.Context) (string, string, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "codex", "login")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return stdout.String(), stderr.String(), exitCode, fmt.Errorf("codex login 超时")
		}
		if errors.Is(err, exec.ErrNotFound) {
			return stdout.String(), stderr.String(), exitCode, fmt.Errorf("未找到 codex 命令，请确认已安装并配置 PATH")
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		return stdout.String(), stderr.String(), exitCode, err
	}
	return stdout.String(), stderr.String(), exitCode, nil
}
