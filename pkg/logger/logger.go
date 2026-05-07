package logger

import (
	"errors"
	"os"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"golang.org/x/term"
)

// Init configures the global zerolog logger.
//
// The only required input is the log level (debug / info / warn / error).
// Output format is auto-detected:
//   - TTY (make run, local terminal) → pretty coloured text
//   - non-TTY (container, CI)        → JSON, one object per line
//
// At debug level every entry also carries the caller file:line.
func Init(level string) error {
	if level == "" {
		return errors.New("log level must not be empty")
	}

	lvl, err := zerolog.ParseLevel(level)
	if err != nil {
		return err
	}
	zerolog.SetGlobalLevel(lvl)

	var base zerolog.Logger
	if term.IsTerminal(int(os.Stdout.Fd())) {
		base = zerolog.New(zerolog.ConsoleWriter{Out: os.Stdout}).With().Timestamp().Logger()
	} else {
		base = zerolog.New(os.Stdout).With().Timestamp().Logger()
	}

	if lvl <= zerolog.DebugLevel {
		base = base.With().Caller().Logger()
	}

	log.Logger = base
	return nil
}
