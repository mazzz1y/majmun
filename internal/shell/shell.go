package shell

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"majmun/internal/config/common"
	"majmun/internal/logging"
	"os"
	"os/exec"
	"text/template"

	"github.com/Masterminds/sprig/v3"
)

const (
	maxRenderIterations = 10
	bufferSize          = 64 * 1024
)

type Streamer struct {
	cmdTmpl  []*template.Template
	envVars  []string
	tmplVars map[string]any
}

type startedCmd struct {
	proc   *exec.Cmd
	stdout io.ReadCloser
}

func NewShellStreamer(command []string, envVars []common.NameValue, tmplVars []common.NameValue) (*Streamer, error) {
	cmdTmpl := make([]*template.Template, 0, len(command))

	for _, cmdPart := range command {
		tmpl, err := template.
			New("").
			Funcs(sprig.FuncMap()).
			Parse(cmdPart)

		if err != nil {
			return nil, fmt.Errorf("parse template: %w", err)
		}
		cmdTmpl = append(cmdTmpl, tmpl)
	}

	environ := os.Environ()
	for _, envVar := range envVars {
		environ = append(environ, envVar.Name+"="+envVar.Value)
	}

	tmplVarMap := make(map[string]any, len(tmplVars))
	for _, tmplVar := range tmplVars {
		tmplVarMap[tmplVar.Name] = tmplVar.Value
	}

	return &Streamer{
		cmdTmpl:  cmdTmpl,
		envVars:  environ,
		tmplVars: tmplVarMap,
	}, nil
}

func (s *Streamer) WithTemplateVars(templateVars map[string]any) *Streamer {
	clone := &Streamer{
		cmdTmpl:  s.cmdTmpl,
		envVars:  s.envVars,
		tmplVars: make(map[string]any),
	}

	if s.tmplVars != nil {
		for k, v := range s.tmplVars {
			clone.tmplVars[k] = v
		}
	}

	for k, v := range templateVars {
		clone.tmplVars[k] = v
	}

	return clone
}

func (s *Streamer) RunWithStdout(ctx context.Context, w io.Writer) (int64, error) {
	cmd, err := s.startCommand(ctx, nil)
	if err != nil {
		return 0, err
	}

	buf := make([]byte, bufferSize)
	bytesWritten := int64(0)

	for {
		if ctx.Err() != nil {
			return bytesWritten, nil
		}

		n, readErr := cmd.stdout.Read(buf)
		if readErr != nil {
			return bytesWritten, nil
		}

		if n > 0 {
			bytesWritten += int64(n)
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				return bytesWritten, nil
			}
		}
	}
}

func (s *Streamer) RunWithStdin(ctx context.Context, stdin io.Reader) error {
	cmd, err := s.startCommand(ctx, stdin)
	if err != nil {
		return err
	}
	return cmd.proc.Wait()
}

func (s *Streamer) startCommand(ctx context.Context, stdin io.Reader) (*startedCmd, error) {
	commandParts, err := s.renderCommand(s.tmplVars)
	if err != nil {
		return nil, err
	}

	run := exec.CommandContext(ctx, commandParts[0], commandParts[1:]...)
	run.Env = s.envVars
	run.Stdin = stdin

	var stdout io.ReadCloser
	if stdin == nil {
		stdout, err = run.StdoutPipe()
		if err != nil {
			return nil, err
		}
	}

	stderr, err := run.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := run.Start(); err != nil {
		return nil, err
	}

	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			logging.Debug(ctx, "command output", "msg", scanner.Text())
		}
	}()

	return &startedCmd{proc: run, stdout: stdout}, nil
}

func (s *Streamer) renderCommand(tmplVars map[string]any) ([]string, error) {
	cmdLen := len(s.cmdTmpl)

	if cmdLen == 1 {
		result, err := renderTemplate(s.cmdTmpl[0], tmplVars)
		if err != nil {
			return nil, err
		}
		return []string{"sh", "-c", result}, nil
	}

	command := make([]string, cmdLen)
	for i, tmpl := range s.cmdTmpl {
		result, err := renderTemplate(tmpl, tmplVars)
		if err != nil {
			return nil, err
		}
		command[i] = result
	}

	return command, nil
}

func renderTemplate(tmpl *template.Template, tmplVars map[string]any) (string, error) {
	buf := &bytes.Buffer{}
	var prevResult string

	iter := 0
	for iter < maxRenderIterations {
		buf.Reset()
		if err := tmpl.Execute(buf, tmplVars); err != nil {
			return "", fmt.Errorf("render: %w", err)
		}
		newResult := buf.String()
		if prevResult == newResult {
			break
		}
		prevResult = newResult
		iter++
	}

	return prevResult, nil
}
