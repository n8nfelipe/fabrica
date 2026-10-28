# Design

## Context

Ver `proposal.md` para a motivação. `ActionRejectQA` grava o comentário e a evidência em `task_events` e move a tarefa para `Correção necessária`. A interface oferece “Iniciar correção” a Desenvolvedor/Admin. `ActionStartCorrection` hoje apenas move a tarefa para `Em desenvolvimento` e deixa `implementation_status` como `not_started`; o handler não cria uma execução de agente para essa ação. O gerador de implementação recebe descrição e especificação aprovada, mas não lê a última reprovação de QA.

## Goals / Non-Goals

**Goals:**
- Fazer “Iniciar correção” registrar a execução e iniciar o agente dentro da mesma transição persistida.
- Incluir no prompt o comentário e a evidência da reprovação mais recente, mantendo a especificação aprovada como limite de escopo.
- Preservar a auditoria, os papéis atuais e o gate que exige nova aprovação de QA antes do PR.

**Non-Goals:**
- Permitir que QA execute diretamente alterações de código ou aprove sua própria correção.
- Alterar o conteúdo dos eventos de QA já armazenados ou introduzir uma nova tabela de feedback.
- Iniciar a correção automaticamente no momento da reprovação; o Desenvolvedor continua decidindo quando começar.

## Decisions

- Tratar `start_correction` como o início de uma nova execução de implementação: ao aceitar a ação, atualizar status para `running`, criar `agent_runs` e `task_implementations` no ciclo existente e disparar o gerador após commit. Isso evita que o estado visual de desenvolvimento fique sem trabalho ativo.
- Ler o evento `reject_qa` mais recente da tarefa (`comment` como recomendações e `reason` como evidência) ao preparar a execução e incluí-los como seção própria no prompt. Não concatenar feedback antigo de tentativas anteriores; os eventos continuam disponíveis no histórico.
- Manter a ação de rejeição e o estado intermediário `Correção necessária`. A separação dá a QA controle da decisão e deixa o início da alteração sob responsabilidade de Desenvolvedor/Admin.
- Registrar falha da nova execução usando o fluxo existente de implementação. Em caso de sucesso, `ActionImplementationDone` retorna a tarefa para QA e redefine a aprovação como pendente; assim a regra existente continua impedindo PR sem nova aprovação.
- Não criar migração: eventos e registros de execução existentes já acomodam comentários, evidências e tentativas. Alternativa considerada: adicionar campos dedicados de recomendação; isso duplicaria dados presentes no histórico append-only.

## Risks / Trade-offs

- [Feedback de QA pode ser muito amplo] → Delimitar no prompt como recomendações para a correção e manter a especificação aprovada como fonte de escopo.
- [Evento de reprovação pode estar ausente em dados legados] → Não iniciar a execução sem uma reprovação correspondente; retornar erro claro e manter a tarefa em `Correção necessária`.
- [Falha ao criar a execução deve reverter a mudança de estado] → Persistir transição, registro de execução e status na mesma transação antes de iniciar o agente.

## Migration Plan

Sem migração de banco. Publicar backend e frontend juntos. Reversão do backend restaura a ação antiga, que move para desenvolvimento sem iniciar execução; tarefas já reprovadas mantêm seus eventos e podem ser iniciadas após a correção ser reaplicada.
