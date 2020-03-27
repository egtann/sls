package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"git.sr.ht/~egtann/sls"
)

type Service struct {
	Mux *http.ServeMux
	log sls.Logger

	// targets to Write logs to. These may be rotating files on disk,
	// remote, or even just in-memory.
	targets []sls.LogTarget
}

// NewService prepares handlers to support health checks as well as to receive
// and tail out logs. The sls.Logger is for internal logging purposes and does
// not affect the logs being aggregated or tailed out.
func NewService(
	log sls.Logger,
	targets []sls.LogTarget,
) (*Service, error) {
	srv := &Service{
		log:     log,
		targets: targets,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("health checked\n")
		w.Write([]byte("OK"))
	})
	mux.Handle("/log", http.HandlerFunc(srv.handleLog))
	srv.Mux = mux
	return srv, nil
}

// Shutdown closes all log targets. If any fail, we return the an error. If
// multiple fail, only one of the errors is returned.
func (srv *Service) Shutdown() error {
	var anyErr error
	for _, t := range srv.targets {
		if err := t.Close(); err != nil {
			anyErr = err
		}
	}
	return anyErr
}

func (srv *Service) handleLog(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		srv.postLog(w, r)
		return
	}
	http.NotFound(w, r)
}

func (srv *Service) postLog(w http.ResponseWriter, r *http.Request) {
	if err := srv.execPostLog(r); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Write([]byte("OK"))
}

func (srv *Service) execPostLog(r *http.Request) error {
	srv.log.Printf("writing logs\n")
	logs := []string{}
	if err := json.NewDecoder(r.Body).Decode(&logs); err != nil {
		return fmt.Errorf("decode body: %w", err)
	}
	data := ""
	for _, l := range logs {
		if !strings.HasSuffix(l, "\n") {
			l += "\n"
		}
		data += l
	}

	// Concurrently write the log to all targets
	errs := make(chan error)
	for _, t := range srv.targets {
		go logData(t, []byte(data), errs)
	}

	ctx := r.Context()
	for {
		select {
		case err := <-errs:
			if err != nil {
				return err
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// logData is intended to be used from within a goroutine.
func logData(t sls.LogTarget, data []byte, errs chan<- error) {
	_, err := t.Write(data)
	if err != nil {
		fmt.Printf("failed to log to target %s: %v\n",
			t.Name(), err)
	}

	// Send if there's any listener. Err might be nil, but we want to send
	// it regardless. Don't hang if there's no listener, just discard the
	// message.
	select {
	case errs <- err:
	default:
	}
}

func isClosed(err error) bool {
	return strings.HasSuffix(err.Error(), "write: broken pipe") ||
		strings.HasSuffix(err.Error(), "i/o timeout")
}

func removeTrailingSlash(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimSuffix(r.URL.Path, "/")
		next.ServeHTTP(w, r)
	})
}
