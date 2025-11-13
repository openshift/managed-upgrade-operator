package upgradesteps

import (
	"context"
	"github.com/go-logr/logr"
)

// actionFunction is a function that takes a context and returns an
// indication of success performing the action and any error.
type actionFunction func(context.Context, logr.Logger) (bool, error)

// Action returns a actionStep of name `n` which will execute the
// action function `f`. Errors from `f` are returned directly.
func Action(n string, f actionFunction) actionStep {
	return actionStep{name: n, f: f}
}

// actionStep is a struct representing an action that can be performed.
// It contains a name and a actionFunction representing the work to be
// performed.
type actionStep struct {
	name string
	f    actionFunction
}

// run executes the actionStep's actionFunction in the supplied context
func (s actionStep) run(ctx context.Context, logger logr.Logger) (bool, error) {
	return s.f(ctx, logger)
}

// String returns the actionStep's string representation.
func (s actionStep) String() string {
	return s.name
}
