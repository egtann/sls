package main

import "log"

// logger satisfies the sls.Logger interface.
type logger struct{}

func (l *logger) Printf(s string, vs ...interface{}) {
	log.Printf(s, vs...)
}

func (l *logger) Fatal(err error) {
	log.Fatal(err)
}
