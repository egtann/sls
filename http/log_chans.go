package http

import (
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

func (l *logChans) Send(s string) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	for _, lc := range l.chans {
		if lc.open {
			lc.ch <- s
		}
	}
}

func (l *logChans) NewChan() *logChan {
	l.mu.Lock()
	defer l.mu.Unlock()
	ch := make(chan string)
	l.id++
	lc := &logChan{ch: ch, id: l.id}
	l.chans[l.id] = lc
	return lc
}

func (l *logChans) Delete(c *logChan) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.chans, c.id)
}
