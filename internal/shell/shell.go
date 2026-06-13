package shell

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"majmun/internal/config/common"
	"majmun/internal/logging"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strings"
	"text/template"
	"unicode"

	"github.com/Masterminds/sprig/v3"
)

const (
	maxRenderIterations = 10
	bufferSize          = 64 * 1024
	// envPrefix namespaces the environment variables derived from runtime template
	// vars, e.g. {{ .Stream.PlaylistPath }} is also exposed as $MAJMUN_STREAM_PLAYLIST_PATH.
	envPrefix = "MAJMUN_"
)

// reservedVars are the top-level template namespaces injected at runtime: Stream
// (URL/SegmentPath/PlaylistPath) by streampool's segmenter, Playout (Input) and the
// Channel/Playlist metadata namespaces by the server's channel stream handler. Config
// validation rejects user-defined variables with these names.
var reservedVars = []string{
	"Stream",
	"Playout",
	"Channel",
	"Playlist",
}

// IsReservedVar reports whether name collides with a variable injected at runtime.
func IsReservedVar(name string) bool {
	return slices.Contains(reservedVars, name)
}

type Streamer struct {
	cmdTmpl  []*template.Template
	envVars  []string
	tmplVars map[string]any
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
		envVars:  slices.Concat(s.envVars, envFromVars(templateVars)),
		tmplVars: make(map[string]any),
	}

	maps.Copy(clone.tmplVars, s.tmplVars)
	maps.Copy(clone.tmplVars, templateVars)

	return clone
}

// envFromVars exposes runtime-injected template namespaces as process environment
// variables so commands can read them as $MAJMUN_STREAM_PLAYLIST_PATH in addition to
// {{ .Stream.PlaylistPath }}. Nested namespaces are flattened one level
// (e.g. Stream.PlaylistPath -> MAJMUN_STREAM_PLAYLIST_PATH); scalar values map to
// MAJMUN_<NAME>. Non-string and empty leaves are skipped.
func envFromVars(vars map[string]any) []string {
	var env []string
	for name, value := range vars {
		switch v := value.(type) {
		case map[string]any:
			for field, fieldValue := range v {
				if str, ok := fieldValue.(string); ok && str != "" {
					env = append(env, envPrefix+envName(name)+"_"+envName(field)+"="+str)
				}
			}
		case string:
			if v != "" {
				env = append(env, envPrefix+envName(name)+"="+v)
			}
		}
	}
	slices.Sort(env)
	return env
}

// envName converts a template variable name to an upper SNAKE_CASE environment
// variable segment, splitting camelCase boundaries (PlaylistPath -> PLAYLIST_PATH)
// while keeping acronyms intact (URL -> URL).
func envName(name string) string {
	var b strings.Builder
	runes := []rune(name)
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) {
			prev := runes[i-1]
			next := rune(0)
			if i+1 < len(runes) {
				next = runes[i+1]
			}
			if unicode.IsLower(prev) || (unicode.IsUpper(prev) && unicode.IsLower(next)) {
				b.WriteByte('_')
			}
		}
		b.WriteRune(unicode.ToUpper(r))
	}
	return b.String()
}

func (s *Streamer) Run(ctx context.Context) error {
	commandParts, err := s.renderCommand(s.tmplVars)
	if err != nil {
		return err
	}

	run := exec.CommandContext(ctx, commandParts[0], commandParts[1:]...)
	run.Env = s.envVars

	stderr, err := run.StderrPipe()
	if err != nil {
		return err
	}

	if err := run.Start(); err != nil {
		return err
	}

	go s.drainStderr(ctx, stderr)

	return run.Wait()
}

func (s *Streamer) RunWithStdout(ctx context.Context, w io.Writer) (int64, error) {
	commandParts, err := s.renderCommand(s.tmplVars)
	if err != nil {
		return 0, err
	}

	run := exec.CommandContext(ctx, commandParts[0], commandParts[1:]...)
	run.Env = s.envVars

	stdout, err := run.StdoutPipe()
	if err != nil {
		return 0, err
	}

	stderr, err := run.StderrPipe()
	if err != nil {
		return 0, err
	}

	if err := run.Start(); err != nil {
		return 0, err
	}

	go s.drainStderr(ctx, stderr)

	buf := make([]byte, bufferSize)
	var bytesWritten int64
	var loopErr error

	for loopErr == nil {
		n, readErr := stdout.Read(buf)
		if n > 0 {
			if _, writeErr := w.Write(buf[:n]); writeErr != nil {
				loopErr = writeErr
			}
			bytesWritten += int64(n)
		}
		if readErr != nil {
			if readErr != io.EOF {
				loopErr = readErr
			}
			break
		}
	}

	waitErr := run.Wait()
	if loopErr != nil {
		return bytesWritten, loopErr
	}
	if ctx.Err() != nil {
		return bytesWritten, ctx.Err()
	}
	return bytesWritten, waitErr
}

func (s *Streamer) drainStderr(ctx context.Context, stderr io.ReadCloser) {
	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		logging.Debug(ctx, "command output", "msg", scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		logging.Debug(ctx, "command output scan error", "error", err)
	}
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
