# Proposal

## Why

O backend já tenta iniciar o ambiente automaticamente ao concluir a implementação, mas, quando a inicialização falha, a tela deixa de acompanhar o estado sem mostrar o erro ou os logs. Além disso, um banco definido no Compose pode publicar sua porta padrão no host e impedir a subida quando essa porta já estiver ocupada. QA fica sem link e sem orientação para destravar a revisão.

## What Changes

- Manter a tentativa automática de iniciar o ambiente Compose quando a implementação for concluída e a tarefa entrar em QA.
- Continuar atualizando a tela até o ambiente ficar saudável ou falhar, apresentando o erro e os logs da tentativa malsucedida.
- Preservar o diagnóstico da falha mesmo quando o descritor impede a execução do Compose ou a consulta do evento de falha chega com atraso.
- Exibir o link de revisão assim que o ambiente estiver acessível e oferecer uma ação de nova tentativa após falha.
- Manter bancos de dados acessíveis aos serviços da aplicação pela rede interna do Compose, sem publicar portas padrão do banco no host automaticamente.
- Permitir publicação do banco no host somente quando exigida, com porta configurável e vinculada ao loopback.
- Preservar a proteção que impede iniciar a revisão ou aprovar QA enquanto o ambiente não estiver saudável.

## Capabilities

### New Capabilities

### Modified Capabilities

- `local-project-environments`: garantir inicialização automática na entrada em QA, disponibilização verificável do link e retorno visível em caso de falha.

## Impact

Afeta as instruções de geração e validação dos arquivos Compose dos projetos, o registro e a consulta do resultado de inicialização do ambiente no backend e a apresentação/atualização do estado, erro, logs e link na tela de tarefa. Reutiliza a capability existente de ambientes locais; não requer nova stack nem serviço externo.
