package stats

import "context"

type ctxKey struct{}

// WithRecorder returns a copy of ctx carrying rec, to be read at the upstream
// capture point via FromContext.
func WithRecorder(ctx context.Context, rec *Recorder) context.Context {
	return context.WithValue(ctx, ctxKey{}, rec)
}

// FromContext returns the Recorder stored in ctx, or nil if none is set.
func FromContext(ctx context.Context) *Recorder {
	rec, _ := ctx.Value(ctxKey{}).(*Recorder)
	return rec
}
