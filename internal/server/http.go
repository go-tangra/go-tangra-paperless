package server

import (
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"strings"

	kratosHttp "github.com/go-kratos/kratos/v2/transport/http"
	"github.com/tx7do/kratos-bootstrap/bootstrap"
	grpcMD "google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/go-tangra/go-tangra-common/viewer"
	"github.com/go-tangra/go-tangra-paperless/cmd/server/assets"
	"github.com/go-tangra/go-tangra-paperless/internal/service"

	paperlessV1 "github.com/go-tangra/go-tangra-paperless/gen/go/paperless/service/v1"
)

// NewHTTPServer creates an HTTP server for the public signing endpoints
func NewHTTPServer(
	ctx *bootstrap.Context,
	signingSessionSvc *service.SigningSessionService,
	signingRequestSvc *service.SigningRequestService,
	signingTemplateSvc *service.SigningTemplateService,
) *kratosHttp.Server {
	l := ctx.NewLoggerHelper("paperless/http")

	addr := os.Getenv("PAPERLESS_HTTP_ADDR")
	if addr == "" {
		addr = "0.0.0.0:9501"
	}

	srv := kratosHttp.NewServer(
		kratosHttp.Address(addr),
	)

	// Register routes
	route := srv.Route("/")

	// CORS preflight
	route.Handle("OPTIONS", "/api/v1/signing/sessions/{token}", corsHandler())
	route.Handle("OPTIONS", "/api/v1/signing/sessions/{token}/submit", corsHandler())
	route.Handle("OPTIONS", "/api/v1/signing/sessions/{token}/document", corsHandler())
	route.Handle("OPTIONS", "/api/v1/signing/requests/{id}/download", corsHandler())
	route.Handle("OPTIONS", "/api/v1/signing/templates/{id}/pdf", corsHandler())

	// Public endpoints (no auth)
	route.GET("/api/v1/signing/sessions/{token}", handleGetSigningSession(signingSessionSvc))
	route.POST("/api/v1/signing/sessions/{token}/submit", handleSubmitSigning(signingSessionSvc))
	route.GET("/api/v1/signing/sessions/{token}/document", handleGetDocument(signingSessionSvc))
	route.GET("/api/v1/signing/requests/{id}/download", handleDownloadSignedDocument(signingRequestSvc))

	// Template PDF proxy (token-authenticated)
	route.GET("/api/v1/signing/templates/{id}/pdf", handleGetTemplatePdf(signingTemplateSvc))

	// Authenticated signing session endpoints (for internal signing requests)
	// These are accessed via the admin-service gateway which injects auth metadata
	route.Handle("OPTIONS", "/api/v1/signing/sessions/auth/{token}", corsHandler())
	route.Handle("OPTIONS", "/api/v1/signing/sessions/auth/{token}/submit", corsHandler())
	route.Handle("OPTIONS", "/api/v1/signing/sessions/auth/{token}/document", corsHandler())
	route.GET("/api/v1/signing/sessions/auth/{token}", handleGetAuthenticatedSigningSession(signingSessionSvc))
	route.POST("/api/v1/signing/sessions/auth/{token}/submit", handleSubmitAuthenticatedSigning(signingSessionSvc))
	route.GET("/api/v1/signing/sessions/auth/{token}/document", handleGetAuthenticatedDocument(signingSessionSvc))

	// Health check
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

// handleGetSigningSession returns signing session info
func handleGetSigningSession(signingSessionSvc *service.SigningSessionService) kratosHttp.HandlerFunc {
	return func(ctx kratosHttp.Context) error {
		setCORSHeaders(ctx)

		token := ctx.Vars().Get("token")
		if token == "" {
			return ctx.JSON(http.StatusBadRequest, errorResponse("token is required"))
		}

		// Extract viewer IP
		viewerIP := getViewerIP(ctx)

		// Inject system viewer context and client IP
		grpcCtx := grpcMD.NewIncomingContext(ctx, grpcMD.Pairs("x-client-ip", viewerIP))
		grpcCtx = viewer.NewSystemViewerContext(grpcCtx)

		resp, err := signingSessionSvc.GetSigningSession(grpcCtx, &paperlessV1.GetSigningSessionRequest{
			Token: token,
		})
		if err != nil {
			code, msg := mapSigningError(err)
			return ctx.JSON(code, errorResponse(msg))
		}

		return writeProtoJSON(ctx, http.StatusOK, resp)
	}
}

// handleSubmitSigning processes a signing submission
func handleSubmitSigning(signingSessionSvc *service.SigningSessionService) kratosHttp.HandlerFunc {
	return func(ctx kratosHttp.Context) error {
		setCORSHeaders(ctx)

		token := ctx.Vars().Get("token")
		if token == "" {
			return ctx.JSON(http.StatusBadRequest, errorResponse("token is required"))
		}

		// Read request body
		body, err := io.ReadAll(ctx.Request().Body)
		if err != nil {
			return ctx.JSON(http.StatusBadRequest, errorResponse("failed to read request body"))
		}

		var submitReq paperlessV1.SubmitSigningRequest
		if err := protojson.Unmarshal(body, &submitReq); err != nil {
			return ctx.JSON(http.StatusBadRequest, errorResponse("invalid request body"))
		}
		submitReq.Token = token

		// Extract viewer IP
		viewerIP := getViewerIP(ctx)

		// Inject system viewer context and client IP
		grpcCtx := grpcMD.NewIncomingContext(ctx, grpcMD.Pairs("x-client-ip", viewerIP))
		grpcCtx = viewer.NewSystemViewerContext(grpcCtx)

		resp, err := signingSessionSvc.SubmitSigning(grpcCtx, &submitReq)
		if err != nil {
			code, msg := mapSigningError(err)
			return ctx.JSON(code, errorResponse(msg))
		}

		return writeProtoJSON(ctx, http.StatusOK, resp)
	}
}

// handleDownloadSignedDocument proxies the signed document PDF from S3 to the browser (token-authenticated)
func handleDownloadSignedDocument(signingRequestSvc *service.SigningRequestService) kratosHttp.HandlerFunc {
	return func(ctx kratosHttp.Context) error {
		setCORSHeaders(ctx)

		requestID := ctx.Vars().Get("id")
		if requestID == "" {
			return ctx.JSON(http.StatusBadRequest, errorResponse("id is required"))
		}

		token := ctx.Request().URL.Query().Get("token")
		expiresStr := ctx.Request().URL.Query().Get("expires")
		if token == "" || expiresStr == "" {
			return ctx.JSON(http.StatusUnauthorized, errorResponse("missing download token"))
		}

		var expiresUnix int64
		if _, err := fmt.Sscanf(expiresStr, "%d", &expiresUnix); err != nil {
			return ctx.JSON(http.StatusBadRequest, errorResponse("invalid expires parameter"))
		}

		if !service.ValidateDownloadToken(requestID, token, expiresUnix) {
			return ctx.JSON(http.StatusUnauthorized, errorResponse("invalid or expired download token"))
		}

		grpcCtx := viewer.NewSystemViewerContext(ctx)

		pdfBytes, fileName, err := signingRequestSvc.GetDocumentBytes(grpcCtx, requestID)
		if err != nil {
			code, msg := mapSigningError(err)
			return ctx.JSON(code, errorResponse(msg))
		}

		w := ctx.Response()
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, fileName))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pdfBytes)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pdfBytes)
		return nil
	}
}

// handleGetDocument proxies the signing document PDF from S3 to the browser.
// For external requests: no auth needed. For internal requests: requires HMAC token query param.
func handleGetDocument(signingSessionSvc *service.SigningSessionService) kratosHttp.HandlerFunc {
	return func(ctx kratosHttp.Context) error {
		setCORSHeaders(ctx)

		token := ctx.Vars().Get("token")
		if token == "" {
			return ctx.JSON(http.StatusBadRequest, errorResponse("token is required"))
		}

		// Check for HMAC token (used by authenticated internal signing sessions)
		hmacToken := ctx.Request().URL.Query().Get("token")
		expiresStr := ctx.Request().URL.Query().Get("expires")
		hmacAuthenticated := false
		if hmacToken != "" && expiresStr != "" {
			var expiresUnix int64
			if _, err := fmt.Sscanf(expiresStr, "%d", &expiresUnix); err == nil {
				if service.ValidateDownloadToken(token, hmacToken, expiresUnix) {
					hmacAuthenticated = true
				}
			}
		}

		grpcCtx := viewer.NewSystemViewerContext(ctx)

		pdfBytes, fileName, err := signingSessionSvc.GetDocumentByToken(grpcCtx, token, hmacAuthenticated)
		if err != nil {
			code, msg := mapSigningError(err)
			return ctx.JSON(code, errorResponse(msg))
		}

		w := ctx.Response()
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, fileName))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pdfBytes)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pdfBytes)
		return nil
	}
}

// handleGetTemplatePdf proxies the template PDF from S3 to the browser (HMAC token auth)
func handleGetTemplatePdf(signingTemplateSvc *service.SigningTemplateService) kratosHttp.HandlerFunc {
	return func(ctx kratosHttp.Context) error {
		setCORSHeaders(ctx)

		templateID := ctx.Vars().Get("id")
		if templateID == "" {
			return ctx.JSON(http.StatusBadRequest, errorResponse("id is required"))
		}

		token := ctx.Request().URL.Query().Get("token")
		expiresStr := ctx.Request().URL.Query().Get("expires")
		if token == "" || expiresStr == "" {
			return ctx.JSON(http.StatusUnauthorized, errorResponse("missing download token"))
		}

		var expiresUnix int64
		if _, err := fmt.Sscanf(expiresStr, "%d", &expiresUnix); err != nil {
			return ctx.JSON(http.StatusBadRequest, errorResponse("invalid expires parameter"))
		}

		if !service.ValidateDownloadToken(templateID, token, expiresUnix) {
			return ctx.JSON(http.StatusUnauthorized, errorResponse("invalid or expired download token"))
		}

		grpcCtx := viewer.NewSystemViewerContext(ctx)

		pdfBytes, fileName, err := signingTemplateSvc.GetTemplatePdfBytes(grpcCtx, templateID)
		if err != nil {
			code, msg := mapSigningError(err)
			return ctx.JSON(code, errorResponse(msg))
		}

		w := ctx.Response()
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, fileName))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pdfBytes)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pdfBytes)
		return nil
	}
}

// handleGetAuthenticatedSigningSession returns signing session info for authenticated users
func handleGetAuthenticatedSigningSession(signingSessionSvc *service.SigningSessionService) kratosHttp.HandlerFunc {
	return func(ctx kratosHttp.Context) error {
		setCORSHeaders(ctx)

		token := ctx.Vars().Get("token")
		if token == "" {
			return ctx.JSON(http.StatusBadRequest, errorResponse("token is required"))
		}

		viewerIP := getViewerIP(ctx)

		// For authenticated endpoints, pass through all incoming metadata (including auth headers)
		grpcCtx := grpcMD.NewIncomingContext(ctx, grpcMD.Pairs("x-client-ip", viewerIP))
		grpcCtx = viewer.NewSystemViewerContext(grpcCtx)

		resp, err := signingSessionSvc.GetAuthenticatedSigningSession(grpcCtx, &paperlessV1.GetSigningSessionRequest{
			Token: token,
		})
		if err != nil {
			code, msg := mapSigningError(err)
			return ctx.JSON(code, errorResponse(msg))
		}

		return writeProtoJSON(ctx, http.StatusOK, resp)
	}
}

// handleSubmitAuthenticatedSigning processes an authenticated signing submission
func handleSubmitAuthenticatedSigning(signingSessionSvc *service.SigningSessionService) kratosHttp.HandlerFunc {
	return func(ctx kratosHttp.Context) error {
		setCORSHeaders(ctx)

		token := ctx.Vars().Get("token")
		if token == "" {
			return ctx.JSON(http.StatusBadRequest, errorResponse("token is required"))
		}

		body, err := io.ReadAll(ctx.Request().Body)
		if err != nil {
			return ctx.JSON(http.StatusBadRequest, errorResponse("failed to read request body"))
		}

		var submitReq paperlessV1.SubmitSigningRequest
		if err := protojson.Unmarshal(body, &submitReq); err != nil {
			return ctx.JSON(http.StatusBadRequest, errorResponse("invalid request body"))
		}
		submitReq.Token = token

		viewerIP := getViewerIP(ctx)

		grpcCtx := grpcMD.NewIncomingContext(ctx, grpcMD.Pairs("x-client-ip", viewerIP))
		grpcCtx = viewer.NewSystemViewerContext(grpcCtx)

		resp, err := signingSessionSvc.SubmitAuthenticatedSigning(grpcCtx, &submitReq)
		if err != nil {
			code, msg := mapSigningError(err)
			return ctx.JSON(code, errorResponse(msg))
		}

		return writeProtoJSON(ctx, http.StatusOK, resp)
	}
}

// handleGetAuthenticatedDocument proxies the PDF for authenticated signing sessions
func handleGetAuthenticatedDocument(signingSessionSvc *service.SigningSessionService) kratosHttp.HandlerFunc {
	return func(ctx kratosHttp.Context) error {
		setCORSHeaders(ctx)

		token := ctx.Vars().Get("token")
		if token == "" {
			return ctx.JSON(http.StatusBadRequest, errorResponse("token is required"))
		}

		grpcCtx := viewer.NewSystemViewerContext(ctx)

		pdfBytes, fileName, err := signingSessionSvc.GetAuthenticatedDocument(grpcCtx, token)
		if err != nil {
			code, msg := mapSigningError(err)
			return ctx.JSON(code, errorResponse(msg))
		}

		w := ctx.Response()
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s"`, fileName))
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(pdfBytes)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(pdfBytes)
		return nil
	}
}

func getViewerIP(ctx kratosHttp.Context) string {
	viewerIP := ctx.Header().Get("X-Real-IP")
	if viewerIP == "" {
		viewerIP = ctx.Header().Get("X-Forwarded-For")
		if viewerIP != "" {
			if idx := strings.Index(viewerIP, ","); idx > 0 {
				viewerIP = viewerIP[:idx]
			}
		}
	}
	return viewerIP
}

func setCORSHeaders(ctx kratosHttp.Context) {
	w := ctx.Response()
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
}

func corsHandler() kratosHttp.HandlerFunc {
	return func(ctx kratosHttp.Context) error {
		setCORSHeaders(ctx)
		ctx.Response().WriteHeader(http.StatusNoContent)
		return nil
	}
}

func mapSigningError(err error) (int, string) {
	errMsg := err.Error()
	switch {
	case strings.Contains(errMsg, "authentication required") || strings.Contains(errMsg, "please log in"):
		return http.StatusUnauthorized, errMsg
	case strings.Contains(errMsg, "not authorized") || strings.Contains(errMsg, "forbidden"):
		return http.StatusForbidden, errMsg
	case strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "not active"):
		return http.StatusNotFound, "signing session not found or invalid token"
	case strings.Contains(errMsg, "already") || strings.Contains(errMsg, "completed"):
		return http.StatusConflict, "signing has already been completed"
	case strings.Contains(errMsg, "expired"):
		return http.StatusGone, "this signing request has expired"
	case strings.Contains(errMsg, "cancelled"):
		return http.StatusGone, "this signing request has been cancelled"
	default:
		return http.StatusInternalServerError, "internal error"
	}
}

func errorResponse(msg string) map[string]string {
	return map[string]string{"error": msg}
}

// writeProtoJSON serializes a protobuf message using protojson (camelCase field names)
// instead of encoding/json (which uses snake_case from Go struct tags)
func writeProtoJSON(ctx kratosHttp.Context, statusCode int, msg proto.Message) error {
	opts := protojson.MarshalOptions{EmitUnpopulated: true}
	data, err := opts.Marshal(msg)
	if err != nil {
		return ctx.JSON(http.StatusInternalServerError, errorResponse("failed to serialize response"))
	}
	w := ctx.Response()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(data)
	return nil
}
