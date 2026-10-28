# Spec Delta

## Purpose

Coordena o trabalho dos agentes OpenCode em fases controladas por tarefas, preservando especificações revisáveis pelo PO, implementação condicionada à aprovação e resultados rastreáveis para recuperação de falhas.

## ADDED Requirements

### Requirement: Gerar especificação para tarefa pronta
Quando tarefa for marcada `Pronta`, o sistema MUST solicitar ao OpenCode os artefatos SDD/OpenSpec da tarefa e associar o resultado à tarefa sem aprová-lo automaticamente.

#### Scenario: Especificação gerada
- **WHEN** OpenCode conclui a geração para tarefa pronta
- **THEN** o sistema associa os artefatos à tarefa e aguarda avaliação do PO

#### Scenario: Erro ao gerar especificação
- **WHEN** a execução de OpenCode falha ou termina sem resultado utilizável
- **THEN** o sistema registra a falha, mantém a tarefa bloqueada para implementação e permite retentativa autorizada

### Requirement: Exigir aprovação de especificação antes de implementar
O sistema MUST iniciar implementação automatizada somente após o PO aprovar os artefatos de especificação da tarefa.

#### Scenario: PO aprova artefatos
- **WHEN** PO aprova a especificação associada à tarefa
- **THEN** o sistema libera a fase de implementação

#### Scenario: Tentativa de implementar antes da aprovação
- **WHEN** usuário ou automação tenta iniciar implementação sem aprovação do PO
- **THEN** o sistema bloqueia a execução

### Requirement: Implementar tarefa aprovada com OpenCode
Para tarefa aprovada, o sistema MUST executar OpenCode no workspace do projeto com o contexto da tarefa e os artefatos SDD aprovados.

#### Scenario: Implementação concluída
- **WHEN** OpenCode conclui a implementação da tarefa
- **THEN** o sistema registra o resultado e encaminha o projeto para inicialização do ambiente de validação

#### Scenario: Falha na implementação
- **WHEN** OpenCode retorna falha ou não produz uma alteração verificável
- **THEN** o sistema registra a falha e não libera o QA

### Requirement: Rastrear execuções de agente
O sistema MUST persistir para cada execução o projeto, tarefa, fase, agente, horários, estado, resumo de saída e referências a logs ou artefatos, excluindo segredos dos registros.

#### Scenario: Consultar execução
- **WHEN** usuário autorizado consulta uma execução de agente
- **THEN** o sistema apresenta seu estado e referências ao resultado e aos logs permitidos

#### Scenario: Segredo presente na saída
- **WHEN** uma saída contém credencial ou segredo conhecido pelo sistema
- **THEN** o sistema não persiste esse valor em texto claro nos logs ou no resumo
