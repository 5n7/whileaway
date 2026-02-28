package ai

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

// ClaudeCodeClient calls the claude CLI as a subprocess.
type ClaudeCodeClient struct {
	model   string
	timeout time.Duration
}

// NewClaudeCodeClient creates a new ClaudeCodeClient.
func NewClaudeCodeClient(model string) *ClaudeCodeClient {
	return &ClaudeCodeClient{
		model:   model,
		timeout: 5 * time.Minute,
	}
}

// Validate checks that the claude CLI is available in PATH.
func (c *ClaudeCodeClient) Validate() error {
	if _, err := exec.LookPath("claude"); err != nil {
		return fmt.Errorf("claude CLI not found in PATH: %w", err)
	}
	return nil
}

// Complete calls the claude CLI and returns the response.
func (c *ClaudeCodeClient) Complete(ctx context.Context, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	args := []string{"--print", "--model", c.model, "--output-format", "text"}
	return c.runClaude(ctx, args, strings.NewReader(prompt))
}

func (c *ClaudeCodeClient) runClaude(ctx context.Context, args []string, stdin io.Reader) (string, error) {
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Stdin = stdin

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("claude CLI failed: %w\nstderr: %s", err, stderr.String())
	}
	return stdout.String(), nil
}
