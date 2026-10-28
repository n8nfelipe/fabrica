# Spec Delta

## MODIFIED Requirements

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
