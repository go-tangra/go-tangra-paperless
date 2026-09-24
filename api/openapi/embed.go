// Package openapi embeds the paperless browser API contract.
package openapi

import _ "embed"

// Paperless is the OpenAPI 3.1 document served and validated by the service.
//
//go:embed paperless.yaml
var Paperless []byte
