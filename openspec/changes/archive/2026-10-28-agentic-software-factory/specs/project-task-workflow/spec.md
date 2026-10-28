# Spec Delta

## Purpose

Organiza o trabalho da fábrica em projetos e tarefas rastreáveis, aplicando uma sequência explícita de estados e exigindo decisões humanas nos pontos de aprovação de produto e qualidade.

## ADDED Requirements

### Requirement: Criar projeto e workspace
O sistema MUST permitir criar um projeto com nome, descrição e requisitos e associá-lo a um diretório dedicado em `~/workspace/<nome-do-projeto>`.

#### Scenario: Criar projeto
- **WHEN** usuário autorizado cria projeto com nome válido
- **THEN** o sistema registra o projeto e cria ou associa seu diretório de workspace sem sobrescrever outro projeto

#### Scenario: Nome de projeto já utilizado
- **WHEN** usuário tenta criar projeto com nome já associado a outro projeto
- **THEN** o sistema informa o conflito e preserva os arquivos existentes

### Requirement: Criar tarefas no estado inicial
O sistema MUST permitir criar tarefas vinculadas a um projeto, e cada tarefa nova MUST iniciar no estado `Aberta`.

#### Scenario: Nova tarefa
- **WHEN** usuário autorizado cria uma tarefa em um projeto existente
- **THEN** a tarefa fica vinculada ao projeto e seu estado inicial é `Aberta`

### Requirement: Controlar fluxo de tarefa
O sistema MUST controlar as transições da tarefa entre preparação, especificação, aprovação do PO, implementação, QA, PR, merge e conclusão, sem permitir pular gates obrigatórios.

#### Scenario: Marcar tarefa pronta
- **WHEN** usuário autorizado muda tarefa `Aberta` para `Pronta`
- **THEN** a tarefa fica elegível para geração de especificação

#### Scenario: Aprovar especificação
- **WHEN** PO aprova os artefatos de especificação associados à tarefa
- **THEN** a tarefa fica elegível para implementação

#### Scenario: Devolver especificação
- **WHEN** PO devolve especificação com comentário
- **THEN** a tarefa retorna para ajuste e implementação permanece bloqueada

#### Scenario: QA reprovado
- **WHEN** QA reprova implementação com comentário
- **THEN** a tarefa retorna para correção e criação do PR permanece bloqueada

#### Scenario: Bloquear avanço sem gate
- **WHEN** um ator tenta avançar tarefa sem satisfazer a aprovação exigida para a fase
- **THEN** o sistema recusa a transição e informa o gate pendente

### Requirement: Auditar alterações de estado e decisões
O sistema MUST registrar cada transição e decisão de fluxo com ator, horário, estado anterior, estado novo e justificativa quando aplicável.

#### Scenario: Registrar transição
- **WHEN** uma tarefa muda de estado
- **THEN** o histórico persistido contém ator, horário e estados anterior e novo

#### Scenario: Registrar devolução ou bloqueio
- **WHEN** usuário devolve uma aprovação ou bloqueia uma tarefa
- **THEN** o histórico contém o comentário ou motivo informado

### Requirement: Consultar estado e próxima ação
O sistema MUST permitir consultar o estado atual, histórico, responsável ou ator mais recente, impedimentos e próxima ação esperada para cada tarefa.

#### Scenario: Consultar tarefa em andamento
- **WHEN** usuário autorizado abre uma tarefa
- **THEN** o sistema apresenta seu estado atual, histórico e ação necessária para continuar
