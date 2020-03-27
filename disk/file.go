package disk

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.sr.ht/~egtann/sls"
)

// File is a locking representation of a file on disk. It satisfies the
// LogTarget interface. The lock is needed because Linux only guarantees that
// writes are atomic up to a certain size, but SLS does not limit the size of a
// request.
type File struct {
	fi      *os.File
	dir     string
	created time.Time
	log     sls.Logger

	// mu protects changes to the logfile when rotating or writing to it.
	mu *sync.Mutex
}

// New returns a *disk.File that rotates over time and automatically removes
// old entries.
func New(
	log sls.Logger,
	dir string,
	dur time.Duration,
) (*File, error) {
	now := time.Now().UTC()
	filename := filepath.Join(dir, name(now)+".log")
	const flags = os.O_CREATE | os.O_APPEND | os.O_WRONLY
	fi, err := os.OpenFile(filename, flags, 0644)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	f := &File{
		fi:      fi,
		dir:     dir,
		created: now,
		log:     log,
		mu:      &sync.Mutex{},
	}
	if err := f.rotate(now); err != nil {
		return nil, fmt.Errorf("rotate: %w", err)
	}
	if err := f.deleteOld(dur); err != nil {
		return nil, fmt.Errorf("delete old: %w", err)
	}
	go f.rotateEvery(dur)
	return f, nil
}

// Write to the Logfile. This is not threadsafe and must be called with a
// mutex lock.
func (f *File) Write(byt []byte) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	return f.fi.Write(byt)
}

// Close the file after all writes complete. Once closed the underlying os.File
// cannot be reused.
func (f *File) Close() error { return f.fi.Close() }

// Name of the current logfile.
func (f *File) Name() string { return f.Name() }

// old reports whether the logfile is older than 24 hours and needs to be
// rotated.
func (f *File) old() bool {
	return f.created.Before(time.Now().Add(-24 * time.Hour))
}

// rotateEvery rotates the logfile and deletes old entries. It's intended to be
// called in a goroutine.
func (f *File) rotateEvery(dur time.Duration) {
	for range time.Tick(24 * time.Hour) {
		now := time.Now().UTC()
		if err := f.rotate(now); err != nil {
			f.log.Printf("failed to rotate: %s\n", err)
		}
		if err := f.deleteOld(dur); err != nil {
			f.log.Printf("failed to delete old files: %s\n", err)
		}
	}
}

// rotate the log file.
func (f *File) rotate(now time.Time) error {
	f.log.Printf("rotating log file\n")
	if !f.old() {
		f.log.Printf("writing to %s\n", f.Name())
		return nil
	}
	f.log.Printf("old logfile, rotating out %s\n", f.Name())

	f.mu.Lock()
	defer f.mu.Unlock()

	err := f.Close()
	if err != nil {
		return fmt.Errorf("close: %w", err)
	}
	filename := filepath.Join(f.dir, name(now)+".log")
	f.fi, err = os.Open(filename)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	f.created = now
	f.log.Printf("writing to %s\n", f.Name())
	return nil
}

func (f *File) deleteOld(dur time.Duration) error {
	f.log.Printf("deleting old logs\n")

	// Get all files with *.log in logfile_dir
	files, err := getFilesInDir(f.dir, ".log")
	if err != nil {
		return fmt.Errorf("get files in dir: %w", err)
	}

	// Sort them ascending
	files, err = sortFilesByTimestamp(files)
	if err != nil {
		return fmt.Errorf("sort files by timestamp: %w", err)
	}

	cutoff := time.Now().Add(-1 * dur)
	for _, fi := range files {
		// parse time in filename
		name := strings.TrimSuffix(fi.Name(), filepath.Ext(fi.Name()))
		ti, err := time.Parse("20060102", name)
		if err != nil {
			return fmt.Errorf("invalid time %s: %w", name, err)
		}
		if ti.After(cutoff) {
			// We're done
			return nil
		}

		// Delete this file and continue
		f.log.Printf("deleting old logfile %s\n", fi.Name())
		if err = os.Remove(filepath.Join(f.dir, fi.Name())); err != nil {
			return err
		}
	}
	return nil
}

// name for a logfile given a time. This truncates sub-day time information to
// consistently rotate files after 24 hours.
func name(t time.Time) string {
	t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	return t.Format("20060102")
}

func getFilesInDir(dir, extension string) ([]os.FileInfo, error) {
	if !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}
	files := []os.FileInfo{}
	tmp, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}
	for _, fi := range tmp {
		// Skip directories and hidden files
		if fi.IsDir() || strings.HasPrefix(fi.Name(), ".") {
			continue
		}
		// Skip any non-relevant files
		if filepath.Ext(fi.Name()) != extension {
			continue
		}
		files = append(files, fi)
	}
	return files, nil
}

func sortFilesByTimestamp(files []os.FileInfo) ([]os.FileInfo, error) {
	var errOut error
	regexNum := regexp.MustCompile(`^\d+`)
	sort.Slice(files, func(i, j int) bool {
		if errOut != nil {
			return false
		}
		fiName1 := regexNum.FindString(files[i].Name())
		fiName2 := regexNum.FindString(files[j].Name())
		fiNum1, err := strconv.ParseUint(fiName1, 10, 64)
		if err != nil {
			errOut = fmt.Errorf("parse uint in file %s: %w",
				files[i].Name())
			return false
		}
		fiNum2, err := strconv.ParseUint(fiName2, 10, 64)
		if err != nil {
			errOut = fmt.Errorf("parse uint in file %s: %w",
				files[i].Name())
			return false
		}
		return fiNum1 < fiNum2
	})
	return files, errOut
}
