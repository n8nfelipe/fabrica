# Design

## Context

`agent_runs` já armazena `stdout_redacted` e `stderr_redacted` ao concluir a execução. A API lista apenas metadados e referências, e a tela de tarefa não tem uma área para inspecionar os logs.

## Decisions

- Estender a resposta autorizada de `GET /api/tasks/{id}/agent-runs` com stdout e stderr já redigidos, sem ler os campos originais do processo.
- Expor `GET /api/agent-runs` para retornar até 500 execuções recentes com projeto e tarefa, protegido pela permissão de consulta de progresso.
- Mostrar as execuções em um painel do detalhe da tarefa, com tentativa, fase, estado, resumo, horários, referências e painéis separados para saída e erros.
- Adicionar o menu global Observabilidade, que atualiza a cada dois segundos, resume agentes ativos/falhos/concluídos, desenha atividade dos últimos sete dias e distribuição por estado, e permite selecionar uma execução para ver os logs.
- Para execuções ativas, indicar que a saída completa será armazenada após o término; não apresentar um painel vazio como se fosse uma falha de logging.
- Manter compatibilidade sem migração e reutilizar o mecanismo de autorização `ViewProgress` já aplicado ao endpoint.

## Risks

- A resposta pode ficar grande se a saída persistida for extensa; o executor já limita a saída capturada e os logs continuam redigidos.
- A observabilidade não é streaming. Logs em tempo real exigiriam persistência incremental e um contrato separado.
