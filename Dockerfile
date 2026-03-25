# -- Build Stage --
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copiar arquivos de dependência
COPY go.mod go.sum ./
RUN go mod download

# Copiar o restante do código
COPY . .

# Compilar o binário
RUN CGO_ENABLED=0 GOOS=linux go build -o bin/condoguard-api ./cmd/server

# -- Final Stage --
FROM alpine:latest

# Instalar certificados CA (necessário para conexões TLS externas, se houver)
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copiar o binário do estágio de build
COPY --from=builder /app/bin/condoguard-api .

# Expor a porta configurada no app.go (default 8080)
EXPOSE 8080

# Comando para rodar a aplicação
CMD ["./condoguard-api"]
