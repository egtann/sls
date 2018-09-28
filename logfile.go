package sls

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/errors"
)

// Logfile is a locking representation of a file on disk. It satisfies the
// io.Writer interface. The lock is needed because Linux only guarantees that
// writes are atomic up to a certain size, but SLS does not limit the size of
// a request.
type Logfile struct {
	fi      *os.File
	created time.Time
}

// Write to the Logfile. This is not threadsafe and must be called with a
// mutex lock.
func (l *Logfile) Write(byt []byte) (int, error) { return l.fi.Write(byt) }

// Close the file after all writes complete. Once closed the Logfile cannot be
// reused.
func (l *Logfile) Close() error { return l.fi.Close() }

// Name of the current logfile.
func (l *Logfile) Name() string { return l.fi.Name() }

// Old reports whether the logfile is older than 24 hours and needs to be
// rotated.
func (l *Logfile) Old() bool {
	return l.created.Before(time.Now().Add(-24 * time.Hour))
}

// NewLogfile creates or gets an existing logfile at a given directory.
func NewLogfile(dir string) (*Logfile, error) {
	if !strings.HasSuffix(dir, string(filepath.Separator)) {
		return nil, errors.New("logfile directory must end with filepath separator")
	}
	// Truncate sub-day time information to consistently rotate files after
	// 24 hours, even if the file exists in
	now := time.Now()
	now = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	filename := dir + now.Format("20060102") + ".log"
	fi, err := os.OpenFile(filename, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return nil, errors.Wrap(err, "open")
	}
	logfile := &Logfile{
		fi:      fi,
		created: now,
	}
	return logfile, nil
}
