# Spec Delta

## ADDED Requirements

### Requirement: Bloquear avanço após violação do escopo do agente
O sistema MUST manter a tarefa na fase atual quando uma execução do agente violar a política de escopo e MUST registrar a violação no histórico da tarefa sem conteúdo sensível.

#### Scenario: Violação detectada durante implementação
- **WHEN** a validação da execução encontra alterações não autorizadas
- **THEN** a tarefa não avança para QA e o histórico registra a fase, o tipo de violação e o resultado da execução

#### Scenario: Violação detectada durante especificação
- **WHEN** a execução de especificação altera caminhos além dos artefatos permitidos
- **THEN** o sistema não associa nem submete os artefatos para aprovação do PO

#### Scenario: Falha ao registrar a violação
- **WHEN** o sistema não consegue persistir o evento de violação
- **THEN** a tarefa permanece sem avanço e a falha é retornada ou registrada para reconciliação
