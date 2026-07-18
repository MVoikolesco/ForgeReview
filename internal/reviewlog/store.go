package reviewlog

import (
	"context"
	"errors"
)

var ErrNotFound = errors.New("review log not found")

// Store holds short-lived review execution artifacts independently from the
// configuration database and the container filesystem.
type Store interface {
	CreateRun(context.Context, string) (string, error)
	Write(context.Context, string, string, string) error
	Append(context.Context, string, string, string) error
	Read(context.Context, string, string) (string, error)
	Exists(context.Context, string, string) (bool, error)
	Delete(context.Context, string, string) error
	Runs(context.Context) ([]string, error)
}
