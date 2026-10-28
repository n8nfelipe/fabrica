# Tasks

## 1. Diagnóstico de falhas de inicialização

- [x] 1.1 Ajustar o painel de ambiente para carregar e exibir o evento de falha e os logs quando o status for `unhealthy`, mantendo o erro visível se a consulta aos logs falhar; verificar o estado de falha sem link e o diagnóstico apresentado.
- [x] 1.2 Manter os logs atualizados para Compose que falha após criar serviços e garantir que a API retorne uma resposta útil quando não houver logs; verificar tanto falha antes da criação do serviço quanto falha durante o health check.
- [x] 1.3 Atualizar as instruções de implementação de projetos e a documentação para que serviços de banco usem rede interna Compose sem publicar a porta padrão no host; quando acesso pelo host for requisito, documentar porta configurável restrita a `127.0.0.1` e verificar uma stack com PostgreSQL sobe via `docker compose up` mesmo com a porta padrão ocupada.

## 2. Acompanhamento e nova tentativa

- [x] 2.1 Atualizar a consulta do ambiente para continuar em `starting`, encerrar em estado terminal e retomá-la após uma nova tentativa; verificar os estados `starting`, `unhealthy` e `healthy` na tela.
- [x] 2.2 Verificar o fluxo completo de implementação concluída até link de QA: URL só aparece após health check, retry bem-sucedido libera o link e a aprovação de QA permanece bloqueada sem ambiente saudável.
- [x] 2.3 Preservar o diagnóstico quando a validação do descritor falha, coletando logs por labels do projeto sem seguir caminhos externos e repetindo a consulta do evento de falha quando ele ainda não estiver disponível.
