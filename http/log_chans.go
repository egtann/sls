package http

import (
	"strings"
	"sync"
)

// logChans tracks and atomically increments the ID of the current channel and
// sends logs to any listening channels.
type logChans struct {
	id    int
	mu    sync.RWMutex
	chans map[int]*logChan
}

type logChan struct {
	id   int
	ch   chan string
	open bool
}

func (l logChans) Send(s string) {
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	for _, lc := range l.chans {
		if lc.open {
			lc.ch <- s
		}
	}
}

// NewChan also reports the ID of the channel, so the caller can later delete
// the channel and free up resources.
func (l logChans) NewChan() *logChan {
	l.mu.Lock()
	defer l.mu.Unlock()
	ch := make(chan string)
	l.id++
	lc := &logChan{ch: ch, id: l.id}
	l.chans[l.id] = lc
	return lc
}

func (l logChans) Delete(c *logChan) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.chans, c.id)
}
