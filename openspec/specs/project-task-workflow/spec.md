# project-task-workflow Specification

## Purpose

Organiza o trabalho da fábrica em projetos e tarefas rastreáveis, aplicando uma sequência explícita de estados e exigindo decisões humanas nos pontos de aprovação de produto e qualidade.

## Requirements

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
O sistema MUST controlar as transições da tarefa entre preparação, especificação, aprovação do PO, implementação, QA, PR, merge e conclusão, sem permitir pular gates obrigatórios. A reprovação de QA MUST preservar comentário e evidência como recomendações; ao iniciar a correção, o sistema MUST iniciar uma nova implementação informada por essas recomendações.

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
- **WHEN** QA reprova a implementação com comentário e evidência
- **THEN** a tarefa retorna para correção, mantém as recomendações no histórico e criação do PR permanece bloqueada

#### Scenario: Desenvolvedor inicia correção
- **WHEN** Desenvolvedor autorizado inicia correção de uma tarefa reprovada por QA
- **THEN** a tarefa entra em desenvolvimento, uma nova execução de implementação é iniciada com o comentário e a evidência da reprovação, e o sistema apresenta o estado da execução

#### Scenario: Correção concluída volta para QA
- **WHEN** a execução da correção termina com sucesso e alterações verificáveis
- **THEN** a tarefa volta para QA e continua sem elegibilidade para PR até nova aprovação de QA

#### Scenario: Correção falha
- **WHEN** a execução da correção falha
- **THEN** a tarefa permanece em desenvolvimento com falha registrada e permite nova tentativa sem perder as recomendações de QA

#### Scenario: Bloquear avanço sem gate
- **WHEN** um ator tenta criar PR ou avançar tarefa sem satisfazer a aprovação exigida para a fase
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
