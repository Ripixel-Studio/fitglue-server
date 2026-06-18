package pubsub

import (
	"testing"

	"github.com/fitglue/server/src/go/internal/infra"
)

func TestPubSubAdapter_Logger(t *testing.T) {
	a := &PubSubAdapter{Logger: infra.NewLogger()}
	if a.logger() == nil {
		t.Error("expected a component logger")
	}
}
