package slidev

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ExportPDFFile uses Slidev's own PDF exporter, preserving text and vectors.
func ExportPDFFile(ctx context.Context, input string, opts RenderOptions) ([]byte, error) {
	cli := opts.SlidevPath
	if cli == "" {
		cli = os.Getenv("GSLIDEV_SLIDEV")
	}
	if cli == "" {
		cli = findSlidev(input)
	}
	if cli == "" {
		return nil, fmt.Errorf("slidev executable not found")
	}
	absolute, err := filepath.Abs(input)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "gslidev-pdf-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	output := filepath.Join(dir, "slides.pdf")
	args := []string{"export", filepath.Base(absolute), "--format", "pdf", "--output", output, "--per-slide", "--timeout", "60000", "--wait", "1000", "--with-clicks=false"}
	if opts.ChromePath != "" {
		args = append(args, "--executable-path", opts.ChromePath)
	}
	var cmd *exec.Cmd
	if strings.HasSuffix(cli, ".mjs") || strings.HasSuffix(cli, ".js") {
		node, err := exec.CommandContext(ctx, "node", "-p", "process.execPath").Output()
		if err != nil {
			return nil, fmt.Errorf("locate Node runtime: %w", err)
		}
		cmd = exec.CommandContext(ctx, strings.TrimSpace(string(node)), append([]string{cli}, args...)...)
	} else {
		cmd = exec.CommandContext(ctx, cli, args...)
	}
	cmd.Dir = filepath.Dir(absolute)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("slidev export: %w", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return nil, fmt.Errorf("Slidev export did not produce a PDF")
	}
	return data, nil
}
