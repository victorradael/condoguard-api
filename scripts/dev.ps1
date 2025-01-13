param(
    [Parameter(Position=0)]
    [string]$Command
)

function Show-Help {
    Write-Host "Comandos disponíveis:"
    Write-Host "  deps      - Instala as dependências"
    Write-Host "  test      - Executa os testes"
    Write-Host "  run       - Executa o projeto"
    Write-Host "  swagger   - Gera a documentação Swagger"
    Write-Host "  build     - Compila o projeto"
    Write-Host "  clean     - Limpa os arquivos gerados"
    Write-Host "  dev       - Inicia o ambiente de desenvolvimento"
    Write-Host "  monitor   - Inicia os serviços de monitoramento"
}

function Install-Dependencies {
    Write-Host "Instalando dependências..."
    go mod download
    go mod tidy
}

function Run-Tests {
    Write-Host "Executando testes..."
    go test -v -race ./...
}

function Start-App {
    Write-Host "Iniciando aplicação..."
    docker-compose up -d mongodb redis
    go run cmd/api/main.go
}

function Generate-Swagger {
    Write-Host "Gerando documentação Swagger..."
    swag init -g cmd/api/main.go -o docs
}

function Build-Project {
    Write-Host "Compilando projeto..."
    go build -o bin/condoguard.exe cmd/api/main.go
}

function Clean-Project {
    Write-Host "Limpando projeto..."
    go clean
    Remove-Item -Path bin/condoguard.exe -ErrorAction SilentlyContinue
    docker-compose down
}

function Start-Dev {
    Write-Host "Iniciando ambiente de desenvolvimento..."
    Install-Dependencies
    docker-compose up -d
}

function Start-Monitoring {
    Write-Host "Iniciando serviços de monitoramento..."
    docker-compose -f docker-compose.monitoring.yml up -d
}

switch ($Command) {
    "deps" { Install-Dependencies }
    "test" { Run-Tests }
    "run" { Start-App }
    "swagger" { Generate-Swagger }
    "build" { Build-Project }
    "clean" { Clean-Project }
    "dev" { Start-Dev }
    "monitor" { Start-Monitoring }
    default { Show-Help }
} 