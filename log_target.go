package sls

import "io"

// LogTarget is something that SLS will write logs to.
type LogTarget interface {
	Name() string
	io.WriteCloser
}
