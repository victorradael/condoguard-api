# AGENTS — Padrões para Specs

## Propósito

Este documento define os padrões obrigatórios para a criação de novas specs neste projeto. Toda spec deve ser autossuficiente, orientada a testes e focada em regras de negócio.

---

## Estrutura Obrigatória de uma Spec

```
# <Nome do Domínio> — Spec

## Contexto
Uma ou duas frases descrevendo o que esse domínio representa no negócio.

## Regras de Negócio
Lista numerada das invariantes que o código deve garantir.
Cada regra deve ser testável de forma independente.

## Contrato HTTP
Tabela com: método, rota, autenticação requerida, descrição resumida.

## Casos de Teste
Lista de cenários, agrupados por endpoint ou comportamento.
Cada cenário deve especificar: entrada, estado inicial esperado, resultado esperado.

## Ordem de Implementação
Sequência numerada: testes unitários → testes de integração → implementação.
Nunca inverter essa ordem.

## Critérios de Aceite
Condições objetivas e verificáveis que indicam que a spec está completa.
```

---

## Regras de Escrita

**Test-First é inegociável.** Toda seção de implementação começa pelos testes, nunca pelo código de produção.

**Regras de negócio são o centro.** A spec existe para documentar regras, não endpoints. Endpoints são consequência.

**Casos de teste cobrem falhas.** Para cada caminho feliz, deve existir ao menos um caso de erro documentado.

**Sem ambiguidade em valores.** Especifique tipos (string, inteiro, booleano), formatos (ISO 8601, centavos, UUID) e restrições (único, não nulo, positivo).

**Critérios de aceite são binários.** Cada critério deve ser verificável com `go test`. Nada de "deve funcionar bem" — especifique o comportamento exato.

**Uma spec por domínio.** Não misture responsabilidades de domínios diferentes em uma única spec.

---

## Nomenclatura

- Nome do arquivo: `<dominio>.md` em minúsculas (ex: `expense.md`, `auth.md`).
- Nomes de casos de teste: `<entidade>_<acao>_<condicao>` (ex: `user_create_duplicate_email`).

---

## O que uma Spec não é

- Não é documentação de API (não substitui OpenAPI/Swagger).
- Não é guia de implementação técnica detalhada.
- Não é registro de decisões de arquitetura (para isso, use ADRs separados).

---

## Checklist antes de abrir uma spec para revisão

- [ ] Todas as regras de negócio estão listadas e numeradas.
- [ ] Cada regra tem ao menos um caso de teste correspondente.
- [ ] Casos de erro estão documentados.
- [ ] Critérios de aceite são verificáveis por testes automatizados.
- [ ] A ordem test-first → implementação está explícita.
- [ ] Nenhum detalhe de implementação técnica vaza para as regras de negócio.
