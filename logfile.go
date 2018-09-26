package sls

// Logfile is a locking representation of a file on disk. It satisfies the
// io.Writer interface. The lock is needed because Linux only guarantees that
// writes are atomic up to a certain size, but SLS does not limit the size of
// a request.
type Logfile struct {
	fi os.File
	mu sync.Mutex
}

// Write arbitrarily large data to the Logfile in a threadsafe way across
// platforms.
func (l Logfile) Write(byt []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.fi.Write(byt)
}
