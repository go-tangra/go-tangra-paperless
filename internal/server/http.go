package server

import (
	"io/fs"
	"net/http"
	"os"

	kratosHttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/tx7do/kratos-bootstrap/bootstrap"

	"github.com/go-tangra/go-tangra-paperless/cmd/server/assets"
)

// NewHTTPServer creates an HTTP server for serving frontend assets (Module Federation remote)
func NewHTTPServer(
	ctx *bootstrap.Context,
) *kratosHttp.Server {
	l := ctx.NewLoggerHelper("paperless/http")

	addr := os.Getenv("PAPERLESS_HTTP_ADDR")
	if addr == "" {
		addr = "0.0.0.0:9501"
	}

	srv := kratosHttp.NewServer(
		kratosHttp.Address(addr),
	)

	// Health check
	route := srv.Route("/")
	route.GET("/health", func(ctx kratosHttp.Context) error {
		return ctx.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	// Serve embedded frontend assets (Module Federation remote)
	fsys, err := fs.Sub(assets.FrontendDist, "frontend-dist")
	if err == nil {
		fileServer := http.FileServer(http.FS(fsys))
		srv.HandlePrefix("/", fileServer)
		l.Infof("Serving embedded frontend assets")
	} else {
		l.Warnf("Failed to load embedded frontend assets: %v", err)
	}

	l.Infof("HTTP server listening on %s", addr)
	return srv
}
