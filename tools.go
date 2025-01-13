//go:build tools
// +build tools

package tools

import (
	_ "github.com/swaggo/swag/cmd/swag"
	// Outras ferramentas de desenvolvimento podem ser adicionadas aqui
	_ "github.com/golangci/golangci-lint/cmd/golangci-lint"
	_ "github.com/golang/mock/mockgen"
) 