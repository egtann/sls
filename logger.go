package sls

// Logger is used to log internal sls events and has no bearing on the logs
// being aggregated or tailed out.
type Logger interface {
	Printf(string, ...interface{})
}
