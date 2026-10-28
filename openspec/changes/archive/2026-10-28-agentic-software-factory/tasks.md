# Tasks

## 1. Base local da fábrica

- [x] 1.1 Criar estrutura do backend Go e frontend React com inicialização local e verificar que a UI acessa um endpoint de saúde da API.
- [x] 1.2 Criar o Compose dedicado do PostgreSQL da fábrica, volume persistente, configuração local e migrações iniciais; verificar inicialização, conexão da API e persistência após reinício do container.
- [x] 1.3 Implementar configuração do workspace em `~/workspace`, validação de nomes e resolução segura de caminhos; verificar rejeição de traversal e de colisão sem sobrescrever arquivos.
- [x] 1.4 Documentar pré-requisitos locais, configuração e comandos de inicialização/parada da fábrica; verificar os comandos em uma instalação limpa de desenvolvimento.

## 2. Usuários e autorização

- [x] 2.1 Implementar autenticação local, sessão, armazenamento de hash de senha e setup de primeiro Admin restrito a loopback; verificar login, logout e encerramento do bootstrap após criar a primeira conta.
- [x] 2.2 Implementar CRUD de usuários, desativação e atribuição dos cinco perfis; verificar operações permitidas ao Admin e recusadas a outros perfis.
- [x] 2.3 Implementar autorização por perfil para aprovações, QA, implementação, merge e administração, com permissões mínimas do Scrum Master; verificar transições proibidas e permitidas por perfil.
- [x] 2.4 Criar telas React de login, setup inicial e gestão de usuários; verificar fluxos de criação, edição, desativação e mensagens de autorização.

## 3. Projetos, tarefas e auditoria

- [x] 3.1 Implementar criação, consulta e edição de projetos e tarefas com persistência PostgreSQL; verificar projeto associado ao workspace e tarefa iniciada em `Aberta`.
- [x] 3.2 Implementar máquina de estados e gates de especificação, PO, implementação, QA, PR, checks, merge e conclusão; verificar que nenhuma transição obrigatória pode ser pulada.
- [x] 3.3 Implementar eventos auditáveis para transições, aprovações, comentários, falhas e tentativas; verificar ator, horário, estados e motivo no histórico.
- [x] 3.4 Criar telas React para projetos, quadro de tarefas, detalhe/histórico e próxima ação; verificar atualização visual depois de cada transição principal.

## 4. Execução OpenCode

- [x] 4.1 Implementar executor local que invoque OpenCode em diretório de projeto, envie prompt por stdin e processe eventos `--format json`; verificar captura de stdout/stderr, exit code, cancelamento e limite de saída.
- [x] 4.2 Implementar geração de artefatos SDD/OpenSpec ao marcar tarefa `Pronta`, com prompt focado em especificação e sem iniciar implementação; verificar vínculo dos artefatos e gate de aprovação do PO.
- [x] 4.3 Implementar revisão, aprovação e devolução de especificações pelo PO; verificar que aprovação libera implementação e devolução exige nova rodada.
- [x] 4.4 Implementar execução de implementação com OpenCode somente após aprovação, registrando resultados, arquivos alterados e falhas; verificar bloqueio do QA em caso de falha.
- [x] 4.5 Implementar persistência e retentativa idempotente de execuções, redaction de segredos e reconciliação após reinício; verificar que a retomada não duplica runs nem expõe valores sentinela.

## 5. Ambientes Compose e QA local

- [x] 5.1 Definir e documentar o descritor de projeto que informa arquivo Compose, serviço QA, porta interna e health check; verificar validação de campos obrigatórios e rejeição de caminhos fora do workspace.
- [x] 5.2 Implementar inicialização, inspeção, parada e reinício isolados de cada projeto via Docker Compose, incluindo serviços de banco descritos pela stack livre; verificar que iniciar/parar um projeto não afeta outro.
- [x] 5.3 Implementar alocação de porta local com registro de leases, verificação de portas e retry para colisões de bind; verificar concorrência entre dois projetos e porta externa já ocupada.
- [x] 5.4 Iniciar automaticamente o Compose quando OpenCode concluir a implementação; só disponibilizar a URL local de QA após health check saudável e registrar falhas de subida com estado e diagnóstico visíveis.
- [x] 5.5 Criar telas React de estado do ambiente, URL, logs permitidos e ações iniciar/parar/reiniciar; verificar ciclo completo de revisão local.

## 6. GitHub, PR e releases

- [x] 6.1 Implementar adaptador local do `gh` que verifica autenticação e executa comandos sem interpolação de shell ou armazenamento de token; verificar erros de CLI e sessão expirada com retentativa segura.
- [x] 6.2 Implementar aprovação/reprovação do QA com comentários e evidências; verificar que somente QA aprovado libera criação do PR.
- [x] 6.3 Implementar criação de branch/PR vinculado à tarefa, consulta JSON dos checks e merge manual por Admin, Desenvolvedor, QA ou PO; verificar bloqueio com checks pendentes/reprovados e registro do resultado.
- [x] 6.4 Criar `ci.yml` acionado somente por `pull_request`, cobrindo testes, lint, typecheck e análise estática; verificar triggers, permissões `contents: read` e presença dos quatro grupos de checks.
- [x] 6.5 Criar `release.yml` acionado somente por push em `main`, gerando tag SemVer anotada desde `v1.0.0` sem rodar checks; verificar permissões `contents: write`, salto de tag existente e ausência de checks no workflow.
- [x] 6.6 Documentar integração GitHub e dependência de `gh auth login` prévio; verificar instruções de autenticação, criação do PR e resolução de falhas.

## 7. Integração e validação do fluxo

- [x] 7.1 Integrar os fluxos ponta a ponta de projeto novo, tarefa `Aberta` → `Pronta`, geração/aprovação SDD, implementação, ambiente QA, PR, checks, merge e conclusão; verificar cada gate e ator permitido.
- [x] 7.2 Adicionar verificação de recuperação após reinício do backend com tarefa em execução, ambiente ativo ou PR aberto; verificar estado reconciliado com PostgreSQL, Docker e GitHub.
- [x] 7.3 Completar documentação operacional de backup do volume PostgreSQL, encerramento/limpeza de ambientes e limitações do executor local; verificar passos a partir de uma instalação limpa.
