# syntax=docker/dockerfile:1
# Paperless service image: builds the Vue remote, embeds it (-tags ui), and
# produces a slim runtime carrying paperlesssvc. Build context is the repo root
# so the module's replace directives (../.. and sibling services) resolve.

FROM node:22-alpine AS ui
WORKDIR /ui
COPY services/paperless/ui/package.json services/paperless/ui/package-lock.json* ./
RUN npm ci --no-audit --no-fund || npm install --no-audit --no-fund
COPY services/paperless/ui/ ./
RUN npm run build

FROM golang:1.26-alpine AS build
RUN apk add --no-cache git ca-certificates
WORKDIR /src
COPY . .
COPY --from=ui /ui/dist ./services/paperless/ui/dist
WORKDIR /src/services/paperless
ENV CGO_ENABLED=0 GOFLAGS=-buildvcs=false
RUN go build -tags "ui" -o /out/paperlesssvc ./cmd/paperlesssvc

FROM alpine:3.20
RUN apk add --no-cache ca-certificates postgresql-client && adduser -D -u 10001 app
COPY --from=build /out/paperlesssvc /usr/local/bin/
COPY services/paperless/deploy /app/deploy
WORKDIR /app
USER app
ENTRYPOINT ["paperlesssvc"]
CMD ["-config", "deploy/container.yaml"]
