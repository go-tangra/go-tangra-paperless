package providers

import (
	"github.com/google/wire"

	"github.com/go-tangra/go-tangra-paperless/internal/event"
)

// ProviderSet is the Wire provider set for event layer
var ProviderSet = wire.NewSet(
	event.NewPublisher,
)
