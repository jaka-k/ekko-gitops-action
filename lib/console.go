package lib

import (
	"fmt"
	"os"

	"github.com/fatih/color"
)

// Console output with a visual hierarchy:
//
//	Step / StepBold  — top-level pipeline steps, magenta (bold = jumps out)
//	Sub              — indented second-level detail, dark yellow
//	Info/Warn/Error  — plain messages behind a colored LEVEL: prefix
//	Success          — final green confirmation
//
// Runner logs render ANSI colors but aren't a TTY, so fatih/color would
// silently disable itself on GitHub Actions without the init override.
func init() {
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		color.NoColor = false
	}
}

var (
	stepC   = color.New(color.FgMagenta)
	stepEmC = color.New(color.FgMagenta, color.Bold)
	subC    = color.New(color.FgYellow)
	infoC   = color.New(color.FgCyan, color.Bold)
	warnC   = color.New(color.FgYellow, color.Bold)
	errorC  = color.New(color.FgRed, color.Bold)
	okC     = color.New(color.FgGreen, color.Bold)
)

// Step prints a top-level step headline.
func Step(format string, a ...any) {
	stepC.Printf(format+"\n", a...)
}

// StepBold prints a top-level line that should jump out (banners, results).
func StepBold(format string, a ...any) {
	stepEmC.Printf(format+"\n", a...)
}

// Sub prints second-level detail, indented under the current step.
func Sub(format string, a ...any) {
	subC.Printf("  "+format+"\n", a...)
}

// Info prints a neutral message behind a cyan INFO: prefix.
func Info(format string, a ...any) {
	fmt.Fprintf(color.Output, "%s %s\n", infoC.Sprint("INFO:"), fmt.Sprintf(format, a...))
}

// Warn prints a warning behind a yellow WARN: prefix.
func Warn(format string, a ...any) {
	fmt.Fprintf(color.Output, "%s %s\n", warnC.Sprint("WARN:"), fmt.Sprintf(format, a...))
}

// Error prints an error behind a red ERROR: prefix, to stderr.
func Error(format string, a ...any) {
	fmt.Fprintf(color.Error, "%s %s\n", errorC.Sprint("ERROR:"), fmt.Sprintf(format, a...))
}

// Success prints the green end-of-command confirmation.
func Success(format string, a ...any) {
	okC.Printf(format+"\n", a...)
}
