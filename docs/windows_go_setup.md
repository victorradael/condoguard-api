# Guia de Instalação do Go no Windows

## Passo 1: Download do Go

1. Visite a página oficial de downloads do Go:
   - https://go.dev/dl/

2. Baixe a versão mais recente para Windows:
   - Escolha o arquivo MSI para Windows (ex: `go1.21.6.windows-amd64.msi`)
   - Certifique-se de escolher a versão correta para seu sistema (64-bit ou 32-bit)

## Passo 2: Instalação

1. Execute o arquivo MSI baixado
2. Siga o assistente de instalação
   - A instalação padrão será em `C:\Go`
   - Mantenha o diretório padrão sugerido

## Passo 3: Configuração do Ambiente

1. Abra o "Painel de Controle"
2. Vá para "Sistema e Segurança" > "Sistema"
3. Clique em "Configurações avançadas do sistema"
4. Clique em "Variáveis de Ambiente"
5. Em "Variáveis do Sistema", encontre e edite "Path"
6. Adicione as seguintes entradas (se não existirem):
   ```
   C:\Go\binþ
   %USERPROFILE%\go\bin
   ```

## Passo 4: Configuração do GOPATH

1. Em "Variáveis de Usuário", clique em "Novo"
2. Adicione:
   - Nome da variável: `GOPATH`
   - Valor: `%USERPROFILE%\go`

## Passo 5: Verificação da Instalação

1. Abra um novo PowerShell ou Prompt de Comando
2. Execute os seguintes comandos:
   ```powershell
   go version
   go env
   ```

## Próximos Passos

1. Familiarize-se com a estrutura de projetos Go
2. Aprenda sobre go modules (`go mod init`, `go mod tidy`)
3. Configure seu editor de código preferido
4. Comece com projetos simples para praticar

## Links Úteis

- [Documentação Oficial do Go](https://golang.org/doc/)
- [Tour of Go](https://tour.golang.org/)
- [Effective Go](https://golang.org/doc/effective_go)
- [Go by Example](https://gobyexample.com/) 