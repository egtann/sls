package http

import (
	"net/http"
)

type handler struct {
	Logfile Logfile
}

func NewMux(logfile sls.Logfile) *http.Mux {
	h := &handler{Logfile: logfile}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		hlog.FromRequest(r).Debug().Msg("health")
		w.Write([]byte("OK"))
	})
	mux.HandleFunc("/version", func(w http.ResponseWriter, r *http.Request) {
		hlog.FromRequest(r).Debug().Msg("version")
		w.Write(version)
	})
	mux.Handle("/log", h.handleLog)
}

func (h *handler) Log(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		h.GetLog(w, r)
	case "POST":
		h.PostLog(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *handler) PostLog(w http.ResponseWriter, r *http.Request) {
	// read the json body

	// construct the log format

	// append to the file on disk - h.Logfile
}

func (h *handler) GetLog(w http.ResponseWriter, r *http.Request) {
}
