##################################
# Stage 0: Build frontend module
##################################

FROM node:20-alpine AS frontend-builder

RUN npm install -g pnpm@9

WORKDIR /frontend
COPY go-tangra-paperless/frontend/package.json go-tangra-paperless/frontend/pnpm-lock.yaml* ./
RUN pnpm install --frozen-lockfile || pnpm install
COPY go-tangra-paperless/frontend/ .
RUN pnpm build

##################################
# Stage 1: Build Go executable
##################################

FROM golang:1.23-alpine AS builder

ARG APP_VERSION=1.0.0

# Enable toolchain auto-download for newer Go versions
ENV GOTOOLCHAIN=auto

# Install build dependencies
RUN apk add --no-cache git make curl

# Install buf for proto descriptor generation
RUN curl -sSL "https://github.com/bufbuild/buf/releases/latest/download/buf-$(uname -s)-$(uname -m)" -o /usr/local/bin/buf && \
    chmod +x /usr/local/bin/buf

# Set working directory
WORKDIR /src

# Copy go mod files first for better caching
COPY go-tangra-paperless/go.mod go-tangra-paperless/go.sum ./

# Copy go-tangra-common for replace directive
COPY go-tangra-common/ /go-tangra-common/

RUN go mod download

# Copy the entire source code
COPY go-tangra-paperless/ .

# Regenerate proto descriptor (ensures embedded descriptor.bin is always up to date)
RUN buf build -o cmd/server/assets/descriptor.bin

# Build the server
RUN CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    go build -ldflags "-X main.version=${APP_VERSION} -s -w" \
    -o /src/bin/paperless-server \
    ./cmd/server

##################################
# Stage 2: Create runtime image
##################################

FROM alpine:3.20

ARG APP_VERSION=1.0.0

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Set timezone
ENV TZ=UTC

# Set working directory
WORKDIR /app

# Copy executable from builder
COPY --from=builder /src/bin/paperless-server /app/bin/paperless-server

# Copy configuration files
COPY --from=builder /src/configs/ /app/configs/

# Copy frontend assets from frontend builder
COPY --from=frontend-builder /frontend/dist /app/frontend-dist

# Create non-root user and pdfcpu font directory
RUN addgroup -g 1000 paperless && \
    adduser -D -u 1000 -G paperless paperless && \
    mkdir -p /home/paperless/.config/pdfcpu/fonts && \
    chown -R paperless:paperless /home/paperless/.config && \
    chown -R paperless:paperless /app

# Switch to non-root user
USER paperless:paperless

# Expose gRPC and HTTP ports
EXPOSE 9500 9501

# Set default command
CMD ["/app/bin/paperless-server", "-c", "/app/configs"]

# Labels
LABEL org.opencontainers.image.title="Paperless Service" \
      org.opencontainers.image.description="Document Management Service with RustFS Storage" \
      org.opencontainers.image.version="${APP_VERSION}"
