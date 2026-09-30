package domain

import (
	"context"
	"errors"
)

// MultiReplySource tries each source in order and returns the first reply
// that isn't ErrNoReply. Every source is expected to reject agent kinds it
// doesn't understand with ErrNoReply, so this never needs to dispatch by
// kind itself; order among sources that could both answer does not matter
// in practice because no two sources currently claim the same kind.
type MultiReplySource []ReplySource

// LastReply implements ReplySource.
func (m MultiReplySource) LastReply(ctx context.Context, agent Agent) (Reply, error) {
	lastErr := error(ErrNoReply)
	for _, s := range m {
		if err := ctx.Err(); err != nil {
			return Reply{}, err
		}
		r, err := s.LastReply(ctx, agent)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Reply{}, ctxErr
		}
		if err == nil {
			return r, nil
		}
		if !errors.Is(err, ErrNoReply) {
			return Reply{}, err
		}
		lastErr = err
	}
	return Reply{}, lastErr
}

// Recent implements ReplySource with the same order and error rules as
// LastReply.
func (m MultiReplySource) Recent(ctx context.Context, agent Agent) (string, error) {
	lastErr := error(ErrNoReply)
	for _, s := range m {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		text, err := s.Recent(ctx, agent)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return "", ctxErr
		}
		if err == nil {
			return text, nil
		}
		if !errors.Is(err, ErrNoReply) {
			return "", err
		}
		lastErr = err
	}
	return "", lastErr
}
