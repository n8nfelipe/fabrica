# PRD — Fábrica de Software Agêntica

- Estado: rascunho para revisão de produto
- Versão: 0.1
- Data: 2026-10-27
- Fontes: pedido do usuário nesta conversa; convenção de CI em `.agents/skills/regra-ci/SKILL.md`; configuração OpenSpec em `openspec/config.yaml`
- Aceite de negócio: pendente; nenhum PO aprovou este documento ainda

## Problema e resultado esperado

Times que constroem software precisam coordenar descoberta, especificação, implementação, validação e entrega. A fábrica deve manter esse fluxo visível por projeto e tarefa, delegando fases bem definidas a agentes e deixando decisões de produto, QA e merge sob controle dos perfis autorizados.

O resultado esperado é uma aplicação web em que usuários criam projetos e tarefas, acompanham cada etapa, revisam especificações geradas por agente, testam a implementação em ambiente local e encaminham mudanças por pull request.

## Escopo

- Incluído: aplicação web para gestão de projetos e tarefas; perfis PO, QA, Admin, Scrum Master e Desenvolvedor; fluxo SDD com OpenSpec; integração de agente OpenCode; ambiente de projeto em `~/workspace/<nome-do-projeto>`; execução por Docker Compose; link local para QA; criação e acompanhamento de PR; permissões de merge.
- Fora do escopo inicial: hospedagem de projetos em produção, cobrança, gestão de portfólio corporativo, suporte a outros provedores Git além do definido na implantação, e execução de agentes sem revisão humana nos pontos de aprovação.
- Restrições confirmadas: backend Go; frontend React; PostgreSQL; projetos executados por Docker Compose; usar OpenSpec/SDD para especificações; agente OpenCode; verificações de PR separadas da criação de release no merge, conforme `regra-ci`.
- Dependências: GitHub CLI (`gh`) instalado e autenticado na máquina que executa a fábrica; OpenCode disponível ao executor; Docker Engine e Docker Compose; acesso aos diretórios de workspace. A integração de repositórios e PRs usa GitHub via `gh`; o agente de especificação e desenvolvimento é OpenCode.

## Atores, permissões e fluxo

### Perfis iniciais

| Perfil | Responsabilidades e permissões iniciais |
| --- | --- |
| PO | Criar/acompanhar projetos e tarefas; avaliar especificações; aprovar ou devolver especificações; pode fazer merge. |
| QA | Validar a implementação pelo link local; aprovar ou reprovar QA; pode fazer merge. |
| Admin | Administrar usuários, projetos, tarefas e configurações; pode executar qualquer ação do sistema, inclusive aprovações e merge. |
| Scrum Master | Organizar projetos e tarefas, acompanhar impedimentos e fluxo. O limite de edição/aprovação ainda requer decisão de produto. |
| Desenvolvedor | Acompanhar e implementar tarefas, revisar resultado e executar merge após validações aprovadas. |

As permissões acima transcrevem as ações explicitamente descritas e propõem responsabilidades mínimas para os demais perfis. A matriz completa de permissões deve ser confirmada antes de implementar autorização fina.

### Jornada de tarefa

1. Um usuário autorizado cria um projeto. O sistema cria ou associa o diretório `~/workspace/<nome-do-projeto>` e registra a configuração do projeto.
2. Uma tarefa começa em `Aberta`. Ao ser marcada `Pronta`, fica elegível para geração de especificação pelo OpenCode usando a estrutura OpenSpec/SDD.
3. A tarefa passa para `Aguardando aprovação das especificações`. O PO aprova ou devolve com comentários. Aprovação libera desenvolvimento; devolução retorna para ajuste e nova avaliação.
4. O agente de desenvolvimento implementa a tarefa. Para projeto novo, cria a estrutura de frontend, backend, banco de dados e demais componentes descritos no projeto. O projeto deve ter Docker Compose para subir os serviços.
5. Ao terminar, o ambiente é iniciado localmente. O sistema verifica portas ocupadas e fornece um link acessível ao QA sem conflitar com outros projetos em execução.
6. A tarefa passa para `Aguardando QA`. QA valida a implementação pelo link e aprova ou reprova com evidências/comentários. Reprovação retorna a tarefa para correção e nova validação.
7. Após aprovação do QA, o agente cria um pull request. O PR recebe somente verificações (testes, lint, typecheck e análise estática) no evento de abertura/atualização. A criação de tag de release ocorre somente no push/merge para `main`; os dois objetivos não devem ser misturados em workflows, conforme a regra `regra-ci`.
8. Após os checks obrigatórios do PR passarem, a tarefa fica `Pronta para merge`. Desenvolvedor, QA ou PO pode fazer merge. Admin pode fazer tudo.
9. Após merge, a tarefa fica `Concluída` e o ambiente de revisão pode ser encerrado conforme política de retenção ainda a definir.

### Estados propostos

`Aberta` → `Pronta` → `Especificando` → `Aguardando aprovação das especificações` → `Em desenvolvimento` → `Aguardando QA` → `Em QA` → `Aguardando PR` → `PR aberto` → `Pronta para merge` → `Concluída`.

Estados de retorno: `Ajustes nas especificações`, `Correção necessária` e `Bloqueada`. Toda transição deve registrar ator, data/hora, estado anterior e novo, além de comentário/motivo quando houver reprovação ou bloqueio. Uma tarefa só pode avançar quando os critérios da fase anterior forem atendidos. O estado `Pronta` significa que a tarefa tem informação suficiente para iniciar a especificação; não significa concluída.

## Requisitos funcionais

| ID | Comportamento esperado | Prioridade | Origem |
| --- | --- | --- | --- |
| RF-001 | Admin gerencia usuários por CRUD: cria, lista/consulta, edita e desativa contas, atribuindo um dos perfis PO, QA, Admin, Scrum Master ou Desenvolvedor. Usuários desativados não podem autenticar. | Must | Pedido do usuário |
| RF-002 | Usuários autorizados criam projetos com nome, descrição e requisitos, e o sistema associa cada projeto a `~/workspace/<nome-do-projeto>`. | Must | Pedido do usuário |
| RF-003 | Projeto organiza tarefas e cada tarefa começa em `Aberta`; usuários autorizados podem marcá-la `Pronta` quando estiver preparada para especificação. | Must | Pedido do usuário |
| RF-004 | Ao entrar em `Pronta`, o sistema aciona OpenCode para criar/atualizar artefatos OpenSpec/SDD ligados à tarefa, sem substituir aprovação humana. | Must | Pedido do usuário |
| RF-005 | PO pode aprovar ou devolver especificações com comentário; somente aprovação libera desenvolvimento. | Must | Pedido do usuário |
| RF-006 | Agente implementa tarefa aprovada; para projeto novo gera a estrutura descrita no projeto, incluindo frontend, backend, banco e Docker Compose. | Must | Pedido do usuário |
| RF-007 | A stack-base da fábrica é Go no backend, React no frontend e PostgreSQL como banco de dados. | Must | Pedido do usuário |
| RF-008 | Sistema sobe o projeto por Docker Compose, detecta conflitos de portas e disponibiliza um link local ao QA. | Must | Pedido do usuário |
| RF-009 | QA valida pelo link, registra aprovação ou reprovação e comentário; somente QA aprovado libera criação do PR. | Must | Pedido do usuário |
| RF-010 | Após QA aprovado, agente cria PR e sistema acompanha status e checks do provedor Git configurado. | Must | Pedido do usuário e regra `regra-ci` |
| RF-011 | PR executa somente testes, lint, typecheck e análise estática; merge para `main` executa somente criação de tag SemVer `v*`, iniciando em `v1.0.0`. | Must | Regra `regra-ci` |
| RF-012 | Após todos os checks obrigatórios passarem, Desenvolvedor, QA ou PO pode fazer merge; Admin pode executar qualquer ação. | Must | Pedido do usuário |
| RF-013 | Sistema mantém histórico auditável de transições, aprovações, comentários, execuções de agentes, links e PRs. | Should | Necessário para rastrear o fluxo descrito |
| RF-014 | Usuário pode ver estado atual, responsável/ator, impedimentos e próxima ação de cada tarefa. | Should | Necessário para operar o fluxo |

## Dados e interfaces

- **Usuário:** identificador, nome, e-mail/login, perfil, estado ativo e credencial/autenticação gerenciada com segurança.
- **Projeto:** identificador, nome único no escopo do workspace, descrição/requisitos, caminho de workspace, configuração de execução e timestamps.
- **Tarefa:** identificador, projeto, título, descrição/critério de aceite, estado, ator responsável, artefatos de especificação, evidências de QA, referência de branch/PR e histórico.
- **Execução de agente:** tarefa/fase, agente e versão/configuração, início/fim, status, saída resumida e referências a logs/artefatos. Segredos não devem ser gravados em logs.
- **Ambiente de QA:** projeto, serviços, portas alocadas, URL local, estado e timestamps de início/fim.
- Links locais devem ser acessíveis pelo navegador da equipe na máquina/host onde a fábrica executa. Acesso remoto, proxy e TLS não estão definidos nesta versão.
- Falhas em agente, Docker, alocação de porta ou provedor Git devem interromper o avanço automático, preservar logs úteis e permitir retentativa autorizada sem apagar o histórico.

## Requisitos não funcionais

| ID | Métrica e alvo | Condições de medição | Método de verificação | Origem |
| --- | --- | --- | --- | --- |
| RNF-001 | Toda transição de fase e decisão humana deve ser persistida com ator e timestamp. | Fluxo normal e falhas recuperáveis. | Testes de integração e inspeção do histórico. | Rastreabilidade do fluxo solicitado |
| RNF-002 | Alocação de porta não pode devolver uma porta já ocupada por outro projeto gerenciado. | Dois projetos iniciados concorrentemente e processos externos ocupando portas candidatas. | Teste de integração/concorrência e verificação do socket antes de iniciar serviços. | Pedido do usuário |
| RNF-003 | Segredos de acesso ao Git e agentes não devem aparecer em texto claro em logs ou respostas da API. | Execuções bem-sucedidas e com erro. | Inspeção de logs e testes com valores sentinela. | Prática de segurança necessária à integração |
| RNF-004 | Metas de disponibilidade, latência, capacidade de usuários e retenção de logs permanecem por definir. | A definir pelo responsável de produto/infra. | Decisão registrada antes do lançamento. | Não especificado no pedido |

## Critérios de aceite

### CA-001 — Projeto e diretório — RF-002

- Dado um usuário autorizado e um nome válido de projeto, quando criar o projeto, então a aplicação registra o projeto e cria/associa `~/workspace/<nome-do-projeto>`.
- Dado um nome que conflita com projeto existente, quando tentar criar outro projeto com o mesmo nome, então a operação informa o conflito sem sobrescrever arquivos.

### CA-002 — Início de tarefa — RF-003

- Dado um projeto existente, quando criar tarefa, então ela é registrada em `Aberta`.
- Dada uma tarefa `Aberta`, quando usuário autorizado a marcar `Pronta`, então a geração de especificação é enfileirada e o histórico registra a transição.

### CA-003 — Especificação e decisão do PO — RF-004, RF-005

- Dada uma tarefa `Pronta`, quando OpenCode concluir, então os artefatos OpenSpec/SDD são associados à tarefa e o estado aguarda avaliação do PO.
- Dado que o PO aprove a especificação, quando confirmar a decisão, então a tarefa fica elegível para desenvolvimento.
- Dada uma especificação devolvida, quando o PO informar motivo, então o motivo fica registrado e a tarefa retorna para ajuste; o agente não inicia implementação antes de nova aprovação.

### CA-004 — Implementação e ambiente — RF-006, RF-007, RF-008

- Dada uma tarefa aprovada num projeto novo, quando o agente concluir implementação, então o repositório contém os componentes descritos e uma configuração Docker Compose utilizável.
- Dado que uma porta candidata está ocupada, quando iniciar o ambiente, então o sistema escolhe outra porta disponível e fornece URL válida ao QA.
- Dado que não seja possível iniciar algum serviço, quando a inicialização falhar, então o sistema registra a falha e não libera a etapa de QA.

### CA-005 — QA e PR — RF-009, RF-010, RF-011

- Dado um ambiente ativo, quando QA abrir o link, então consegue validar a versão associada à tarefa.
- Dado QA reprovado, quando registrar reprovação e comentário, então a tarefa retorna para correção e nenhum PR é criado.
- Dado QA aprovado, quando a automação abrir PR, então os checks configurados cobrem testes, lint, typecheck e análise estática e o PR fica ligado à tarefa.
- Dado merge em `main`, quando disparar o workflow de release, então ele cria a tag SemVer apropriada sem repetir checks de PR.

### CA-006 — Merge e auditoria — RF-012, RF-013, RF-014

- Dado PR com checks obrigatórios aprovados, quando Desenvolvedor, QA ou PO autorizado fizer merge, então a tarefa é marcada `Concluída` e a ação é auditada.
- Dado usuário sem permissão, quando tentar aprovar especificação, aprovar QA ou fazer merge, então a ação é recusada e não altera o estado.
- Dada qualquer transição registrada, quando consultar o histórico, então aparecem estado anterior/novo, ator, timestamp e comentário aplicável.

### CA-007 — CRUD de usuários e perfis — RF-001

- Dado um Admin, quando cadastrar usuário com dados válidos e um perfil inicial, então a conta é criada com o perfil atribuído.
- Dado um Admin, quando listar ou consultar usuários, então pode localizar contas e visualizar seus perfis e estado (ativo/desativado).
- Dado um Admin, quando editar os dados ou perfil de um usuário, então as alterações persistem e passam a valer nas próximas verificações de autorização.
- Dado um Admin, quando desativar uma conta, então o usuário deixa de poder autenticar; o registro permanece disponível para auditoria e referência histórica.
- Dado um usuário sem permissão administrativa, quando tentar criar, editar ou desativar contas, então a operação é recusada.

## Rastreabilidade e verificação

| Requisito | Critérios | Verificação prevista | Evidência observada |
| --- | --- | --- | --- |
| RF-001 | CA-007 | Testes de CRUD, desativação e autorização administrativa | Não executada; especificação somente |
| RF-012 | CA-006 | Testes de autorização de merge | Não executada; especificação somente |
| RF-002, RF-003 | CA-001, CA-002 | Testes de API e persistência | Não executada; especificação somente |
| RF-004, RF-005 | CA-003 | Teste do fluxo do agente e decisão do PO | Não executada; especificação somente |
| RF-006, RF-007, RF-008 | CA-004 | Teste de geração de projeto e inicialização Compose/portas | Não executada; especificação somente |
| RF-009, RF-010, RF-011 | CA-005 | Integração QA/provedor Git e inspeção dos workflows | Não executada; especificação somente |
| RF-013, RF-014 | CA-006 | Testes de histórico e consulta do quadro | Não executada; especificação somente |

## Hipóteses, dúvidas e riscos

| ID | Questão | Impacto / dependência | Resolução proposta | Estado |
| --- | --- | --- | --- | --- |
| Q-001 | Qual provedor Git hospedará os repositórios e PRs (GitHub, GitLab ou outro)? | Define API, autenticação, webhooks e integração de checks. | Decidido: GitHub e GitHub CLI (`gh`) já instalado e autenticado no executor; não requerer novo login Git na fábrica para o MVP. | Resolvida pelo usuário em 2026-10-27 |
| Q-002 | Como autenticar usuários da própria fábrica? | Define identidade, sessões e autorização dos perfis. A credencial Git do executor não autentica os usuários do sistema. | Definir provedor de identidade antes da implementação de autenticação. | Pendente |
| Q-003 | O diretório `~/workspace` é relativo ao usuário do serviço, a um usuário do host ou a um volume configurável? | Impacta permissões, containers e instalação. | Definir ambiente alvo e estratégia de montagem de volume. | Pendente |
| Q-004 | Como OpenCode será executado: processo local ou container? | Impacta isolamento, permissões, logs e disponibilidade. | Decidido: OpenCode é o agente utilizado; definir apenas o modo de execução durante o desenho operacional. | Parcialmente resolvida pelo usuário em 2026-10-27 |
| Q-005 | Como o sistema deve expor o link de QA se a fábrica estiver em container ou em outra máquina? | Determina rede, hostname, proxy e acesso do navegador. | Definir topologia de execução e público do link antes da implementação de ambientes. | Pendente |
| Q-006 | Qual é a matriz completa de permissões do Scrum Master e dos demais perfis para criar/editar projetos e tarefas? | Bloqueia autorização final e interface de ações. | PO/Admin aprovar matriz de permissões. | Pendente |
| Q-007 | Quais limites de carga, retenção de logs e disponibilidade são esperados? | Impede dimensionamento e critérios operacionais quantitativos. | Definir antes do lançamento. | Pendente |
| R-001 | Código gerado por agente pode ser destrutivo ou conter falhas. | Pode danificar workspace ou expor vulnerabilidades. | Isolar workspace, versionar mudanças e exigir aprovações humanas previstas no fluxo. | Aberto |

## Histórico de mudanças

| Data | IDs afetados | Mudança e motivo | Fonte |
| --- | --- | --- | --- |
| 2026-10-27 | Todos | Criação inicial do PRD a partir do pedido do usuário e das regras locais consultadas. | Conversa e skill `regra-ci` |
