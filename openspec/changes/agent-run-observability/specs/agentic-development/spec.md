## ADDED Requirements

### Requirement: Consultar logs redigidos das execuções do agente
O sistema MUST permitir que usuários autorizados consultem os logs redigidos e as referências de cada execução do agente vinculada à tarefa.

#### Scenario: Consultar execução concluída
- **WHEN** usuário autorizado abre a área de observabilidade de uma tarefa
- **THEN** o sistema apresenta fase, tentativa, estado, resumo, stdout e stderr redigidos, horários e referências aos artefatos

#### Scenario: Consultar execução em andamento
- **WHEN** usuário autorizado consulta uma execução ainda ativa
- **THEN** o sistema apresenta o estado em andamento e informa que os logs completos estarão disponíveis após o término

### Requirement: Consultar visão global de observabilidade dos agentes
O sistema MUST oferecer um menu global para usuários autorizados acompanharem execuções recentes de todos os projetos.

#### Scenario: Ver agentes e atividade
- **WHEN** usuário autorizado abre Observabilidade
- **THEN** o sistema mostra agentes em execução, execuções recentes e gráficos de atividade dos últimos sete dias e distribuição por estado

#### Scenario: Consultar logs de uma execução
- **WHEN** usuário seleciona uma execução na lista global
- **THEN** o sistema apresenta resumo, estado e stdout/stderr redigidos; se ela ainda estiver em execução, informa que a saída completa estará disponível após o término
