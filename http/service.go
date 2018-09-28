package http

import (
	"crypto/subtle"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egtann/sls"
	"github.com/justinas/alice"
	"github.com/pkg/errors"
)

type Service struct {
	Mux *http.ServeMux

	dir       string
	apiKey    string
	log       sls.Logger
	logfile   *sls.Logfile
	listeners logChans

	// mu protects changes to the logfile when rotating or writing to it.
	mu sync.Mutex
}

// NewService prepares handlers to support health and version checks as well as
// to receive and tail out logs. The sls.Logger is for internal logging
// purposes and does not affect the logs being aggregated or tailed out.
func NewService(
	log sls.Logger,
	dir, apiKey string,
	version []byte,
) (*Service, error) {
	logfile, err := sls.NewLogfile(dir)
	if err != nil {
		return nil, errors.Wrap(err, "new logfile")
	}
	srv := &Service{
		log:       log,
		logfile:   logfile,
		dir:       dir,
		apiKey:    apiKey,
		listeners: logChans{chans: map[int]*logChan{}},
	}
	chain := alice.New()
	chain = chain.Append(removeTrailingSlash)
	chain = chain.Append(srv.isLoggedIn)
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("health checked\n")
		w.Write([]byte("OK"))
	})
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("version checked\n")
		w.Write(version)
	})
	mux.Handle("/log", chain.Then(http.HandlerFunc(srv.handleLog)))
	srv.Mux = mux
	return srv, nil
}

func (srv *Service) Shutdown() error {
	return srv.logfile.Close()
}

// TODO - check for an auth token
func (srv *Service) handleLog(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		srv.getLog(w, r)
	case "POST":
		srv.postLog(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (srv *Service) postLog(w http.ResponseWriter, r *http.Request) {
	if err := srv.execPostLog(w, r); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write([]byte("OK"))
}

func (srv *Service) execPostLog(w http.ResponseWriter, r *http.Request) error {
	srv.log.Printf("writing logs\n")
	logs := []string{}
	if err := json.NewDecoder(r.Body).Decode(&logs); err != nil {
		return errors.Wrap(err, "decode body")
	}
	data := ""
	for _, l := range logs {
		for _, lc := range srv.listeners.chans {
			srv.log.Printf("open? %t\n", lc.open)
		}
		srv.listeners.Send(l)
		data += l
	}
	srv.mu.Lock()
	defer srv.mu.Unlock()
	_, err := srv.logfile.Write([]byte(data))
	return errors.Wrap(err, "write")
}

func (srv *Service) getLog(w http.ResponseWriter, r *http.Request) {
	srv.log.Printf("tailing logs\n")
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	f, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "responsewriter not a flusher", http.StatusInternalServerError)
		return
	}
	lc := srv.listeners.NewChan()
	defer srv.listeners.Delete(lc)
	errChan := make(chan error)

	srv.log.Printf("streaming logs\n")
	go streamLogs(w, f, lc, errChan)
	lc.open = true

	// Keep alive while streaming
	srv.log.Printf("keeping alive\n")
	select {
	case err := <-errChan:
		if isClosed(err) {
			// The client is no longer listening
			srv.log.Printf("closed tail connection\n")
		} else {
			// TODO notify via ErrorReporter
			srv.log.Printf("write: %s\n", err)
		}
	}
}

func streamLogs(
	w http.ResponseWriter,
	f http.Flusher,
	lc *logChan,
	errChan chan<- error,
) {
	for l := range lc.ch {
		_, err := w.Write([]byte(l))
		if err != nil {
			errChan <- err
			return
		}
		f.Flush()
	}
}

func isClosed(err error) bool {
	return strings.HasSuffix(err.Error(), "write: broken pipe") ||
		strings.HasSuffix(err.Error(), "i/o timeout")
}

// EnforceRetentionPolicy checks on boot and every hour log files are rotated
// and that old files are deleted.
func (srv *Service) EnforceRetentionPolicy(dur time.Duration) {
	go func() {
		if err := srv.rotateLogfile(); err != nil {
			srv.log.Printf("failed to rotate: %s\n", err)
		}
		if err := srv.deleteOldFiles(dur); err != nil {
			srv.log.Printf("failed to delete old files: %s\n", err)
		}
		for range time.Tick(24 * time.Hour) {
			if err := srv.rotateLogfile(); err != nil {
				srv.log.Printf("failed to rotate: %s\n", err)
			}
			if err := srv.deleteOldFiles(dur); err != nil {
				srv.log.Printf("failed to delete old files: %s\n", err)
			}
		}
	}()
}

func (srv *Service) rotateLogfile() error {
	srv.log.Printf("rotating logfiles\n")
	if !srv.logfile.Old() {
		srv.log.Printf("writing to %s\n", srv.logfile.Name())
		return nil
	}
	srv.log.Printf("old logfile, rotating out %s\n", srv.logfile.Name())
	srv.mu.Lock()
	defer srv.mu.Unlock()
	logfile, err := sls.NewLogfile(srv.dir)
	if err != nil {
		return err
	}
	srv.logfile = logfile
	srv.log.Printf("writing to %s\n", srv.logfile.Name())
	return nil
}

func (srv *Service) deleteOldFiles(dur time.Duration) error {
	srv.log.Printf("deleting old logs\n")

	// Get all files with *.log in logfile_dir
	files, err := getFilesInDir(srv.dir, ".log")
	if err != nil {
		return errors.Wrap(err, "get files in dir")
	}

	// Sort them ascending
	files, err = sortFilesByTimestamp(files)
	if err != nil {
		return errors.Wrap(err, "sort files by timestamp")
	}

	cutoff := time.Now().Add(-1 * dur)
	for _, fi := range files {
		// parse time in filename
		name := strings.TrimSuffix(fi.Name(), filepath.Ext(fi.Name()))
		ti, err := time.Parse("20060102", name)
		if err != nil {
			return errors.Wrapf(err, "invalid time %s", name)
		}
		if ti.After(cutoff) {
			// We're done
			return nil
		}

		// Delete this file and continue
		srv.log.Printf("deleting old logfile %s\n", fi.Name())
		if err = os.Remove(filepath.Join(srv.dir, fi.Name())); err != nil {
			return err
		}
	}
	return nil
}

func removeTrailingSlash(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimSuffix(r.URL.Path, "/")
		next.ServeHTTP(w, r)
	})
}

func (srv *Service) isLoggedIn(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := []byte(r.Header.Get("X-API-Key"))
		result := subtle.ConstantTimeCompare([]byte(srv.apiKey), key)
		if result != 1 {
			http.NotFound(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func getFilesInDir(dir, extension string) ([]os.FileInfo, error) {
	if !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}
	files := []os.FileInfo{}
	tmp, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, errors.Wrapf(err, "read dir %s", dir)
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
			errOut = errors.Wrapf(err, "parse uint in file %s", files[i].Name())
			return false
		}
		fiNum2, err := strconv.ParseUint(fiName2, 10, 64)
		if err != nil {
			errOut = errors.Wrapf(err, "parse uint in file %s", files[i].Name())
			return false
		}
		return fiNum1 < fiNum2
	})
	return files, errOut
}
