# Proposal

## Why

Criar software envolve várias passagens de responsabilidade que hoje precisam ser coordenadas manualmente. Esta mudança estabelece uma fábrica local que organiza projetos e tarefas, usa OpenCode nas etapas de especificação e implementação e mantém aprovações de produto, QA e merge sob responsabilidade dos perfis humanos.

## What Changes

- Criar gestão de usuários com CRUD administrativo e os perfis PO, QA, Admin, Scrum Master e Desenvolvedor.
- Criar projetos em `~/workspace/<nome-do-projeto>` e tarefas que percorrem abertura, especificação SDD/OpenSpec, aprovação do PO, implementação, QA e conclusão por merge.
- Integrar OpenCode para gerar especificações e implementar tarefas após as aprovações correspondentes.
- Executar a fábrica localmente com backend Go, frontend React e PostgreSQL em container Docker. Cada projeto define livremente sua stack e seus próprios serviços em Docker Compose, incluindo banco de dados em container.
- Reservar portas sem conflito e disponibilizar URLs locais para validação do QA.
- Integrar GitHub por `gh` autenticado para criação de PRs, acompanhamento dos checks e merge autorizado.
- Preservar a regra de CI: PR executa verificações; push/merge em `main` cria tag SemVer, sem misturar responsabilidades dos workflows.

## Capabilities

### New Capabilities

- `user-access`: contas, perfis, CRUD administrativo e autorização por papel.
- `project-task-workflow`: projetos, tarefas, estados, aprovações humanas e trilha de auditoria.
- `agentic-development`: execução do OpenCode para especificação SDD/OpenSpec e implementação controlada por aprovações.
- `local-project-environments`: workspaces, stacks definidas por projeto, Docker Compose e URLs locais de QA com alocação de portas.
- `github-delivery`: operações GitHub via `gh`, criação de PR, acompanhamento dos checks, permissões de merge e contrato de release.

### Modified Capabilities

Nenhuma. O projeto ainda não possui especificações OpenSpec existentes.

## Impact

- Aplicação nova: backend Go, frontend React e PostgreSQL para os dados da fábrica.
- Integrações locais necessárias: OpenCode, Git, GitHub CLI autenticado, Docker Engine e Docker Compose.
- Cada projeto é materializado em `~/workspace/<nome-do-projeto>` e pode usar uma stack própria, desde que seja executável e validável pelo fluxo Docker Compose da fábrica.
- O MVP é local: interface e links de QA são acessados na máquina executora. O design propõe contas locais, setup inicial de Admin por tela de primeiro uso, permissões limitadas para Scrum Master, execução do OpenCode no host e limpeza manual inicial dos ambientes; essas decisões ficam disponíveis para revisão antes da implementação.
