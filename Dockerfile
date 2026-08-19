# --- frontend: React + shadcn/ui SPA ---
FROM node:22-alpine AS frontend
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ .
RUN npm run build

# --- backend: single Go binary with the SPA embedded ---
# --platform=$BUILDPLATFORM plus GOARCH cross-compilation: the Go toolchain
# runs natively on the runner and emits the target architecture, instead of
# emulating the whole compile under QEMU.
FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /gitgit .

FROM alpine:3.20
RUN apk add --no-cache git bash ca-certificates
COPY --from=build /gitgit /usr/local/bin/gitgit
VOLUME /data
EXPOSE 3000
ENV GITGIT_ADDR=:3000 GITGIT_DATA=/data
ENTRYPOINT ["gitgit"]
