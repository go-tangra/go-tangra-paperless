//go:build wireinject
// +build wireinject

//go:generate go run github.com/google/wire/cmd/wire

package providers

import (
	"github.com/google/wire"

	"github.com/go-tangra/go-tangra-paperless/internal/service"
)

// ProviderSet is the Wire provider set for service layer
var ProviderSet = wire.NewSet(
	service.NewCategoryService,
	service.NewDocumentService,
	service.NewDocumentProcessor,
	service.NewPermissionService,
	service.NewStatisticsService,
	service.NewBackupService,
	ProvideResourceLookup,
	ProvidePermissionStore,
	ProvideAuthzEngine,
	ProvideAuthzChecker,
)
