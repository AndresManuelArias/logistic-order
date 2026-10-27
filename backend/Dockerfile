# --- Etapa de build ---
FROM golang:1.22-alpine AS build

WORKDIR /src

# Descarga de dependencias en capa cacheada.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

# Binario estático, sin símbolos (imagen menor y reproducible).
RUN --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# --- Etapa de runtime ---
FROM alpine:3.20

# Usuario sin privilegios (mínimo privilegio).
RUN addgroup -S app && adduser -S -G app app \
    && apk add --no-cache ca-certificates

WORKDIR /app
COPY --from=build /out/server /usr/local/bin/server

USER app
EXPOSE 8080

# Liveness del contenedor (no depende de la BD; /readyz la comprueba).
HEALTHCHECK --interval=10s --timeout=3s --start-period=15s --retries=5 \
    CMD wget -qO- http://127.0.0.1:8080/healthz || exit 1

ENTRYPOINT ["/usr/local/bin/server"]
