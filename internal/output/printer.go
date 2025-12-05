package output

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"os/exec"
	"strings"
	"sync"

	"github.com/codalotl/goagentbench/internal/ansi"
)

type Printer struct {
	out                io.Writer
	appStyle           ansi.Style
	commandStyle       ansi.Style
	commandOutputStyle ansi.Style
	last               outputKind
}

type outputKind int

const (
	outputNone outputKind = iota
	outputApp
	outputCommand
)

// NewPrinter creates a Printer that writes to out using the formatting rules from SPEC.md.
func NewPrinter(out io.Writer) *Printer {
	if out == nil {
		out = io.Discard
	}
	profile, err := ansi.GetColorProfile()
	if err != nil {
		profile = ansi.ColorProfileUncolored
	}
	darkBackground := isDarkBackground()
	commandColor, commandOutputColor := selectColors(profile, darkBackground)

	return &Printer{
		out: out,
		appStyle: ansi.Style{
			Bold:          ansi.StyleSetOn,
			Italic:        ansi.StyleSetOff,
			Underline:     ansi.StyleSetOff,
			Overline:      ansi.StyleSetOff,
			StrikeThrough: ansi.StyleSetOff,
			Reverse:       ansi.StyleSetOff,
			Background:    ansi.NoColor{},
		},
		commandStyle: ansi.Style{
			Foreground:    commandColor,
			Background:    ansi.NoColor{},
			Bold:          ansi.StyleSetOff,
			Italic:        ansi.StyleSetOff,
			Underline:     ansi.StyleSetOff,
			Overline:      ansi.StyleSetOff,
			StrikeThrough: ansi.StyleSetOff,
			Reverse:       ansi.StyleSetOff,
		},
		commandOutputStyle: ansi.Style{
			Foreground:    commandOutputColor,
			Background:    ansi.NoColor{},
			Bold:          ansi.StyleSetOff,
			Italic:        ansi.StyleSetOn,
			Underline:     ansi.StyleSetOff,
			Overline:      ansi.StyleSetOff,
			StrikeThrough: ansi.StyleSetOff,
			Reverse:       ansi.StyleSetOff,
		},
		last: outputNone,
	}
}

// App writes bold application output.
func (p *Printer) App(text string) error {
	if text == "" {
		return nil
	}
	if err := p.ensureGapBeforeApp(); err != nil {
		return err
	}
	if err := p.writeStyled(p.appStyle, ensureTrailingNewline(text)); err != nil {
		return err
	}
	p.last = outputApp
	return nil
}

func (p *Printer) Appf(format string, args ...any) error {
	return p.App(fmt.Sprintf(format, args...))
}

// RunCommand prints the command invocation and then runs it, reformatting stdout+stderr per SPEC.md.
// Returns the command's combined output.
func (p *Printer) RunCommand(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	if err := p.ensureGapBeforeCommand(); err != nil {
		return nil, err
	}
	commandLine := formatCommand(name, args)
	if err := p.writeStyled(p.commandStyle, ensureTrailingNewline(commandLine)); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	output, cmdErr := cmd.CombinedOutput()
	if len(output) > 0 {
		if err := p.writeStyled(p.commandOutputStyle, ensureTrailingNewline(string(output))); err != nil {
			return output, err
		}
	}
	p.last = outputCommand
	return output, cmdErr
}

// RunCommandStreaming streams stdout/stderr through the printer as it arrives, while capturing the combined output.
func (p *Printer) RunCommandStreaming(ctx context.Context, dir, name string, args ...string) ([]byte, error) {
	if err := p.ensureGapBeforeCommand(); err != nil {
		return nil, err
	}
	commandLine := formatCommand(name, args)
	if err := p.writeStyled(p.commandStyle, ensureTrailingNewline(commandLine)); err != nil {
		return nil, err
	}

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	writer := &styledWriter{
		style: p.commandOutputStyle,
		out:   p.out,
	}
	copyStream := func(r io.Reader) error {
		_, err := io.Copy(writer, io.TeeReader(r, &buf))
		return err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	errCh := make(chan error, 2)
	go func() { errCh <- copyStream(stdout) }()
	go func() { errCh <- copyStream(stderr) }()

	var copyErr error
	for i := 0; i < 2; i++ {
		if err := <-errCh; err != nil && copyErr == nil {
			copyErr = err
		}
	}

	waitErr := cmd.Wait()
	p.last = outputCommand

	if waitErr != nil {
		return []byte(buf.String()), waitErr
	}
	return buf.Bytes(), copyErr
}

func (p *Printer) ensureGapBeforeCommand() error {
	switch p.last {
	case outputApp, outputCommand:
		_, err := io.WriteString(p.out, "\n")
		return err
	default:
		return nil
	}
}

func (p *Printer) ensureGapBeforeApp() error {
	if p.last != outputCommand {
		return nil
	}
	_, err := io.WriteString(p.out, "\n")
	return err
}

func (p *Printer) writeStyled(style ansi.Style, text string) error {
	if text == "" {
		return nil
	}
	_, err := io.WriteString(p.out, style.Apply(text))
	return err
}

type styledWriter struct {
	style ansi.Style
	out   io.Writer
	mu    sync.Mutex
}

func (w *styledWriter) Write(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err := w.out.Write([]byte(w.style.Apply(string(p))))
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func ensureTrailingNewline(text string) string {
	if strings.HasSuffix(text, "\n") {
		return text
	}
	return text + "\n"
}

func formatCommand(name string, args []string) string {
	parts := make([]string, 0, len(args)+1)
	parts = append(parts, quoteArg(name))
	for _, arg := range args {
		parts = append(parts, quoteArg(arg))
	}
	return strings.Join(parts, " ")
}

func quoteArg(arg string) string {
	if arg == "" {
		return "''"
	}
	if !strings.ContainsAny(arg, " \t\n'\"\\$&|;<>*?[]{}()") {
		return arg
	}
	return "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
}

func selectColors(profile ansi.ColorProfile, darkBackground bool) (ansi.Color, ansi.Color) {
	var commandColor ansi.Color
	var outputColor ansi.Color
	if darkBackground {
		commandColor = ansi.ANSI256Color(110)
		outputColor = ansi.ANSI256Color(252)
	} else {
		commandColor = ansi.ANSI256Color(25)
		outputColor = ansi.ANSI256Color(238)
	}
	return profile.Convert(commandColor), profile.Convert(outputColor)
}

func isDarkBackground() bool {
	fg, bg := ansi.DefaultFBBGColor()
	if bg != nil {
		if _, ok := bg.(ansi.NoColor); !ok {
			return luminance(bg) < 0.5
		}
	}
	if fg != nil {
		if _, ok := fg.(ansi.NoColor); !ok {
			return luminance(fg) >= 0.5
		}
	}
	return true
}

func luminance(c ansi.Color) float64 {
	if c == nil {
		return 0
	}
	r, g, b := c.RGB8()
	return channelLuma(r)*0.2126 + channelLuma(g)*0.7152 + channelLuma(b)*0.0722
}

func channelLuma(v uint8) float64 {
	normalized := float64(v) / 255.0
	if normalized <= 0.03928 {
		return normalized / 12.92
	}
	return math.Pow((normalized+0.055)/1.055, 2.4)
}
