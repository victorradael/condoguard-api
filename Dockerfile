# -- Build Stage --
FROM golang:1.27.3-alpine3.24@sha256:fa3d596898d9f126af7c01d14ee7597bab3f39cccf392ef36b69fe77cd681f18 AS builder

WORKDIR /src

# Copiar arquivos de dependência
COPY go.mod go.sum ./
RUN go mod download

# Copiar o restante do código
COPY . .

# Compilar os binários: servidor, migrações e criação do primeiro admin
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/condoguard-api ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/condoguard-migrate ./cmd/migrate && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/condoguard-createadmin ./cmd/createadmin

# -- Final Stage --
FROM alpine:3.24@sha256:f59abe656b692d901e184be3445e084786602108599d2a51d229c0f5a48c71bf

# Certificados CA (TLS para PostgreSQL/Redis fora do host) e usuário sem privilégios
RUN apk --no-cache add ca-certificates && \
    addgroup -S -g 10001 condoguard && \
    adduser -S -D -H -u 10001 -G condoguard condoguard

COPY --from=builder /out/ /usr/local/bin/

USER 10001:10001

# Expor a porta configurada (default 8080)
EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=3s --start-period=15s --retries=3 \
    CMD wget -q -O /dev/null "http://127.0.0.1:${PORT:-8080}/health" || exit 1

# O primeiro admin: docker compose exec api condoguard-createadmin
ENTRYPOINT ["condoguard-api"]
