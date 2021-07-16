package notifier

import (
	"fmt"

	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// NewLogNotifier returns a new logNotifier
func NewLogNotifier() (*logNotifier, error) {
	return &logNotifier{}, nil
}

// A notifier that just writes to log output
type logNotifier struct{}

var log = logf.Log.WithName("event-notifier")

func (s *logNotifier) NotifyState(value MuoState, description string) error {
	log.Info(fmt.Sprintf("Upgrade-State: %s Description: %s", string(value), description))
	return nil
}
