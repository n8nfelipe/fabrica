# Proposal

## Why

Falhas e execuções do OpenCode ficam difíceis de investigar quando o operador só consegue consultar status e resumo. A saída redigida do agente já é persistida, mas não está acessível na tela da tarefa.

## What Changes

- Expor stdout e stderr redigidos na consulta de execuções de agente.
- Adicionar uma área de observabilidade no detalhe da tarefa para selecionar tentativas e consultar estado, resumo, logs e referências.
- Informar que os logs completos aparecem após o término da execução, pois o backend ainda os persiste ao finalizar o processo.
- Adicionar um menu global de observabilidade com agentes ativos, histórico recente de logs e gráficos de atividade e estado.

## Capabilities

### Modified Capabilities
- `agentic-development`: apresentar logs redigidos e referências de execução para usuários autorizados.

## Impact

API global de execuções de agente, navegação e painel de observabilidade no frontend e testes associados. Nenhuma migração é necessária porque stdout e stderr já são armazenados em colunas redigidas.
