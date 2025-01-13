<div style="text-align: center;">
    <img src="assets/condoguard-logo.svg" alt="CondoguardLogo" width="200" height="200">
</div>


# CondoGuard

CondoGuard é um aplicativo em desenvolvimento que visa ajudar os condôminos a administrar suas despesas condominiais de forma eficiente e preventiva. Com uma abordagem inovadora, o CondoGuard permite que os usuários gerenciem suas despesas, façam previsões financeiras e identifiquem possíveis problemas antes que eles se tornem críticos.

## Objetivo

O objetivo principal do CondoGuard é fornecer uma ferramenta robusta e amigável para a gestão financeira de condomínios, ajudando tanto os administradores quanto os moradores a terem uma visão clara de suas despesas, além de se prevenirem contra futuros problemas com base no histórico de gastos.

### Principais Funcionalidades

- 🏢 Gestão de moradores e unidades
- 💰 Controle de despesas
- 📱 Sistema de notificações
- 🔐 Autenticação e autorização
- 📊 Monitoramento e métricas
- 🔄 Versionamento de API

## 🚀 Tecnologias Utilizadas

- Go 1.21+
- MongoDB
- Redis
- Docker & Docker Compose
- Prometheus & Grafana
- Swagger/OpenAPI

## 📦 Pré-requisitos

- Go 1.21 ou superior
- Docker e Docker Compose
- Make (opcional, mas recomendado)

## 🛠️ Configuração do Ambiente de Desenvolvimento

1. Clone o repositório:
```bash
git clone https://github.com/seu-usuario/condoguard.git
cd condoguard
```

2. Copie o arquivo de ambiente:
```bash
cp .env.example .env
```

3. Configure as variáveis de ambiente no arquivo `.env`:
```env
# API
PORT=8080
ENV=development

# MongoDB
MONGODB_URI=mongodb://localhost:27017/condoguard
MONGODB_DATABASE=condoguard

# Redis
REDIS_ADDR=localhost:6379
REDIS_PASSWORD=

# JWT
JWT_SECRET=seu_secret_aqui
JWT_EXPIRATION=24h

# Logging
LOG_LEVEL=debug
```

4. Inicie os serviços com Docker Compose:
```bash
docker-compose up -d
```

## 🚀 Executando o Projeto

### Usando Make

O projeto inclui um Makefile para facilitar as operações comuns:

```bash
# Instalar dependências
make deps

# Executar testes
make test

# Executar o projeto em modo desenvolvimento
make run

# Gerar documentação Swagger
make swagger

# Executar linter
make lint

# Buildar o projeto
make build
```

### Manualmente

1. Instale as dependências:
```bash
go mod download
```

2. Execute os testes:
```bash
go test ./...
```

3. Inicie o servidor:
```bash
go run cmd/api/main.go
```

## 📚 Documentação da API

A documentação da API está disponível através do Swagger UI após iniciar o servidor:

- Local: http://localhost:8080/swagger/index.html

## 🔍 Monitoramento

O projeto inclui monitoramento completo usando Prometheus e Grafana:

1. Acesse o Prometheus:
- http://localhost:9090

2. Acesse o Grafana:
- http://localhost:3000
- Login padrão: admin/admin

### Dashboards Disponíveis

- Circuit Breaker Status
- Performance Metrics
- API Usage
- Health Checks

## 🧪 Testes

### Executando Testes

```bash
# Testes unitários
go test ./...

# Testes com cobertura
go test -cover ./...

# Testes de carga (k6)
k6 run tests/load/load_test.js
```

## 📁 Estrutura do Projeto

```
condoguard/
├── cmd/                    # Pontos de entrada da aplicação
├── internal/              # Código interno da aplicação
│   ├── auth/             # Autenticação e autorização
│   ├── handler/          # Handlers HTTP
│   ├── middleware/       # Middlewares
│   ├── model/           # Modelos de dados
│   ├── repository/      # Camada de acesso a dados
│   ├── service/         # Lógica de negócios
│   └── validator/       # Validação de dados
├── pkg/                 # Bibliotecas públicas
├── scripts/            # Scripts úteis
├── deployments/        # Configurações de deploy
└── tests/             # Testes
```

## 🤝 Contribuindo

1. Fork o projeto
2. Crie sua Feature Branch (`git checkout -b feature/AmazingFeature`)
3. Commit suas mudanças (`git commit -m 'Add some AmazingFeature'`)
4. Push para a Branch (`git push origin feature/AmazingFeature`)
5. Abra um Pull Request

## 📝 Licença

Este projeto está sob a licença MIT. Veja o arquivo [LICENSE](LICENSE) para mais detalhes.

## 📧 Contato

Seu Nome - [@seutwitter](https://twitter.com/seutwitter) - seu.email@exemplo.com

Link do Projeto: [https://github.com/seu-usuario/condoguard](https://github.com/seu-usuario/condoguard)

## 🛠️ Ferramentas de Desenvolvimento

O projeto utiliza as seguintes ferramentas de desenvolvimento:

- [swag](https://github.com/swaggo/swag) - Geração automática de documentação Swagger
- [golangci-lint](https://golangci-lint.run/) - Linter para Go
- [mockgen](https://github.com/golang/mock) - Geração de mocks para testes

Todas as ferramentas são instaladas automaticamente ao executar:
```powershell
.\scripts\dev.ps1 deps
```

### Instalação Manual das Ferramentas

Se precisar instalar as ferramentas manualmente:

```bash
# Swagger
go install github.com/swaggo/swag/cmd/swag@latest

# Linter
go install github.com/golangci/golint/cmd/golangci-lint@latest

# Mock Generator
go install github.com/golang/mock/mockgen@latest
```
