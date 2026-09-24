// Package deployermanifest declares what the paperless module registers with the
// application gateway: routes derived from the embedded OpenAPI document, the
// API permissions, the CASL abilities and the navigation entries. The
// paperless gRPC surface is service-to-service and not proxied.
package paperlessmanifest

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/getkin/kin-openapi/openapi3"

	"github.com/go-tangra/go-tangra-paperless/v4/api/openapi"
	"github.com/go-tangra/go-tangra-portal/sdk/v4/pkg/gatewayclient"
)

// Module identity.
const (
	Module       = "paperless"
	DisplayName  = "Paperless"
	Version      = "1.0.0"
	RemotePrefix = "/ui"
)

// OpenAPI operation extensions.
const (
	PermissionExtension = "x-freya-permission"
	PublicExtension     = "x-freya-public"
	BodyLimitExtension  = "x-freya-max-body-bytes"
	TimeoutExtension    = "x-freya-timeout-seconds"
)

// Permissions the module registers.
var Permissions = []gatewayclient.Permission{
	{Resource: "documents", Action: "read", Description: "List, read and download documents"},
	{Resource: "documents", Action: "write", Description: "Upload, update and move documents"},
	{Resource: "documents", Action: "delete", Description: "Delete documents"},
	{Resource: "categories", Action: "read", Description: "Browse the category tree"},
	{Resource: "categories", Action: "manage", Description: "Create, change, move and delete categories"},
	{Resource: "permissions", Action: "manage", Description: "Grant and revoke access to documents and categories"},
	{Resource: "search", Action: "read", Description: "Full-text search documents"},
	{Resource: "stats", Action: "read", Description: "Read document statistics"},
	{Resource: "backup", Action: "manage", Description: "Export and import tenant documents and permissions"},
}

// Grants maps built-in role slugs to the permissions they hold.
var Grants = map[string][]string{
	"owner":    PermissionRefs(),
	"admin":    PermissionRefs(),
	"member":   {"documents:read", "documents:write", "categories:read", "search:read", "stats:read"},
	"auditor":  {"documents:read", "categories:read", "search:read", "stats:read"},
	"operator": {"documents:read", "documents:write", "documents:delete", "categories:read", "categories:manage", "permissions:manage", "search:read", "stats:read"},
}

// Methods proxied by the gateway: none (paperless gRPC is service to service).
var Methods []gatewayclient.Method

// Abilities are the CASL rules bound to the permissions.
var Abilities = []gatewayclient.Ability{
	{Action: []string{"read", "create", "update", "delete"}, Subject: []string{"PaperlessDocument"}, Requires: "documents:read"},
	{Action: []string{"read", "create", "update", "delete"}, Subject: []string{"PaperlessCategory"}, Requires: "categories:read"},
	{Action: []string{"manage"}, Subject: []string{"PaperlessPermission"}, Requires: "permissions:manage"},
	{Action: []string{"read"}, Subject: []string{"PaperlessSearch"}, Requires: "search:read"},
	{Action: []string{"read"}, Subject: []string{"PaperlessStats"}, Requires: "stats:read"},
	{Action: []string{"manage"}, Subject: []string{"PaperlessBackup"}, Requires: "backup:manage"},
}

// Nav lists the navigation contributions.
var Nav = []gatewayclient.NavEntry{
	{Title: "Documents", Path: "/paperless", Icon: "mdi-file-document-multiple-outline", Order: 500, Requires: "documents:read"},
	{Title: "Categories", Path: "/paperless/categories", Icon: "mdi-folder-outline", Order: 510, Requires: "categories:read"},
	{Title: "Search", Path: "/paperless/search", Icon: "mdi-file-search-outline", Order: 520, Requires: "search:read"},
	{Title: "Dashboard", Path: "/paperless/dashboard", Icon: "mdi-view-dashboard-outline", Order: 530, Requires: "stats:read"},
}

// PermissionRefs lists "resource:action" for every declared permission.
func PermissionRefs() []string {
	out := make([]string, 0, len(Permissions))
	for _, p := range Permissions {
		out = append(out, p.Resource+":"+p.Action)
	}
	return out
}

// Routes derives the gateway routes from the OpenAPI document.
func Routes(doc *openapi3.T) ([]gatewayclient.Route, error) {
	known := map[string]bool{}
	for _, p := range PermissionRefs() {
		known[p] = true
	}
	var routes []gatewayclient.Route
	for p, item := range doc.Paths.Map() {
		for m, op := range item.Operations() {
			r := gatewayclient.Route{Method: strings.ToUpper(m), Path: p}
			perm, _ := op.Extensions[PermissionExtension].(string)
			public, _ := op.Extensions[PublicExtension].(bool)
			switch {
			case public:
				r.Public = true
			case perm == "":
				return nil, fmt.Errorf("deployermanifest: %s %s declares no permission", r.Method, p)
			case !known[perm]:
				return nil, fmt.Errorf("deployermanifest: %s %s uses undeclared permission %q", r.Method, p, perm)
			default:
				r.Permission = perm
			}
			if v, ok := op.Extensions[BodyLimitExtension]; ok {
				n, ok := v.(float64)
				if !ok || n <= 0 {
					return nil, fmt.Errorf("deployermanifest: %s %s has a bad body limit", r.Method, p)
				}
				r.MaxBodyBytes = uint64(n)
			}
			if v, ok := op.Extensions[TimeoutExtension]; ok {
				n, ok := v.(float64)
				if !ok || n <= 0 || n > 600 {
					return nil, fmt.Errorf("deployermanifest: %s %s has a bad timeout", r.Method, p)
				}
				r.Timeout = time.Duration(n) * time.Second
			}
			routes = append(routes, r)
		}
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i].Method+" "+routes[i].Path < routes[j].Method+" "+routes[j].Path })
	return routes, nil
}

// Load parses the embedded document.
func Load() (*openapi3.T, error) {
	return openapi3.NewLoader().LoadFromData(openapi.Paperless)
}

// Manifest builds the gateway manifest from the embedded OpenAPI document.
func Manifest() (gatewayclient.Manifest, error) {
	doc, err := Load()
	if err != nil {
		return gatewayclient.Manifest{}, err
	}
	routes, err := Routes(doc)
	if err != nil {
		return gatewayclient.Manifest{}, err
	}
	return gatewayclient.Manifest{
		Module: Module, DisplayName: DisplayName, Version: Version,
		Prefixes:    []string{"/api/paperless"},
		Routes:      routes,
		Methods:     Methods,
		Permissions: Permissions,
		Abilities:   Abilities,
		Exposes:     []string{"./routes", "./nav"},
		Nav:         Nav,
	}, nil
}
