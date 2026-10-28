# Design

## Context

Ver `proposal.md` para a motivação. `persistImplementationResult` já registra `starting`, tenta subir o Compose e registra um evento de falha se não conseguir. `startQAEnvironmentForTask` grava `healthy` e URL após health check, ou `unhealthy` sem URL. A tela em `App.tsx` consulta o ambiente enquanto o status é `starting`, mas encerra a atualização ao receber qualquer outro status; o painel carrega logs apenas após uma ação manual bem-sucedida. A decisão de QA já exige ambiente saudável.

## Goals / Non-Goals

**Goals:**
- Manter a tarefa informada sobre a mesma tentativa automática até sucesso ou falha terminal.
- Apresentar diagnóstico e permitir retry a partir do estado de falha.
- Exibir o link somente quando a verificação de acessibilidade tiver sucesso.
- Evitar que o Compose gerado publique a porta padrão do banco no host quando os serviços podem se comunicar pela rede interna.

**Non-Goals:**
- Mudar o formato do descritor Compose ou adicionar nova infraestrutura de execução.
- Alterar os papéis autorizados a iniciar, revisar ou aprovar QA.
- Reiniciar automaticamente em loop ou ocultar falhas transitórias do histórico.
- Publicar portas de bancos no host por padrão.

## Decisions

- Tratar `project_qa_environments` como fonte do status e URL. Preservar `starting`, `healthy`, `unhealthy` e `stopped`; não criar uma segunda máquina de estados no frontend.
- Ao observar `unhealthy`, a tela deve buscar o evento de falha mais recente e os logs do serviço declarado, apresentar ambos no painel e manter a ação de tentar iniciar novamente. Em erro ao buscar diagnóstico, manter visível o estado de falha e permitir nova consulta.
- Continuar a consulta periódica enquanto estiver `starting`; parar em estado terminal. A nova tentativa muda o estado novamente para `starting`, retomando a atualização e só revela a URL depois do health check.
- Manter a checagem de saúde no backend como condição para persistir a URL e para iniciar a revisão QA. Isso impede que uma URL de container iniciado, porém inacessível, pareça pronta.
- Incluir nas instruções comuns de implementação de projetos a regra de conexão ao banco por nome do serviço Compose e porta interna, sem `ports` de host por padrão. Quando requisitos pedirem acesso pelo host, exigir porta configurável vinculada a `127.0.0.1`. Isso evita colisão também quando o desenvolvedor sobe a stack manualmente, pois a configuração base já não tenta vincular `5432` (ou a porta padrão de outro banco).
- Não depender apenas do override de QA para resolver portas do banco: o override atual mapeia o serviço web, enquanto `docker compose up` executado diretamente usa o arquivo base do projeto.
- Reutilizar eventos existentes para registrar falha e logs Compose já disponíveis pela API; não adicionar tabela ou dependência externa. Alternativa considerada: armazenar mensagens de erro duplicadas no registro do ambiente, o que pode deixar status e histórico divergentes.
- Se a validação do descritor ou da configuração Compose impedir `compose logs`, consultar os IDs de containers pela label `com.docker.compose.project=fabrica-<id>` e buscar logs por ID validado. Isso mantém o fallback limitado ao projeto sem usar caminhos de configuração inválidos. Ao observar `unhealthy`, repetir brevemente a consulta de eventos para cobrir a janela entre a persistência do status e o evento de falha.

## Risks / Trade-offs

- [Logs Compose podem estar indisponíveis quando o Compose falha antes de criar o serviço] → Exibir o erro do evento mesmo sem logs e oferecer atualização/retry.
- [Eventos antigos podem ser confundidos com a tentativa atual] → Selecionar e apresentar o evento de falha mais recente junto ao horário, sem substituir o status canônico do ambiente.
- [Polling continua ativo enquanto a inicialização demora] → Encerrar no primeiro estado terminal e limpar o timer ao sair da tarefa, conforme o ciclo de vida atual do componente.
- [Alguns projetos precisam conectar ferramentas locais ao banco] → Permitir que os requisitos solicitem a publicação explicitamente e manter essa porta configurável, local ao host e documentada.

## Migration Plan

Sem migração de banco. Publicar backend e frontend juntos; se necessário reverter, a inicialização automática e o endpoint atual continuam operando, embora o diagnóstico da falha volte ao comportamento anterior.
