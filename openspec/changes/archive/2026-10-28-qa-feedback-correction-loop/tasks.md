# Tasks

## 1. Iniciar uma execução de correção

- [x] 1.1 Atualizar a transição `start_correction` para marcar implementação como `running` e fazer o handler criar o registro de execução e iniciar o agente após commit; verificar transições válidas/inválidas e que o estado não muda se o registro não puder ser criado.
- [x] 1.2 Incluir no prompt da correção o comentário e a evidência do evento `reject_qa` mais recente, mantendo a especificação aprovada como escopo; verificar que o agente recebe o feedback atual sem misturar recomendações antigas.
- [x] 1.3 Cobrir a correção no histórico de implementação e na interface de tarefa, mostrando a nova tentativa e seu estado; verificar a progressão de `Correção necessária` para `Em desenvolvimento` e os estados em execução, falha e conclusão.

## 2. Preservar os gates de QA

- [x] 2.1 Verificar que uma correção bem-sucedida retorna a tarefa para `Aguardando QA`, exige nova aprovação antes da criação de PR e mantém comentário/evidência da reprovação anterior no histórico.
- [x] 2.2 Verificar que falha na correção deixa a tarefa em desenvolvimento, registra o erro e permite nova tentativa autorizada sem perder o feedback de QA.
