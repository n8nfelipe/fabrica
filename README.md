# Fábrica

Aplicação local para coordenar o ciclo de desenvolvimento de software com apoio de agentes. Organiza projetos e tarefas, gera e revisa especificações, acompanha implementações, disponibiliza ambientes para QA e integra a entrega a pull requests do GitHub. As decisões de aprovação e merge permanecem com as pessoas autorizadas.

## O que oferece

- Projetos com workspace próprio e quadro de tarefas.
- Fluxo de especificação usando OpenSpec/SDD, com revisão e aprovação.
- Execução de agentes OpenCode para especificação e implementação.
- Ambientes locais de QA executados com Docker Compose e verificados por health check.
- Integração com GitHub CLI (`gh`) para criar pull requests, consultar checks e fazer merge.
- Contas com perfis PO, QA, Admin, Scrum Master e Desenvolvedor, além de histórico das ações.

## Tecnologias

- **Frontend:** React, TypeScript e Vite.
- **Backend:** Go 1.22+.
- **Banco de dados:** PostgreSQL 17, com migrações aplicadas pela API.
- **Execução local:** Docker Compose.
- **Agentes e entrega:** OpenCode, Git e GitHub CLI.

## Início rápido

### Pré-requisitos

- Go 1.22 ou mais recente.
- Node.js 22 ou mais recente e npm.
- Docker Engine com Docker Compose v2.
- Git, OpenCode e GitHub CLI (`gh`) para usar os fluxos de agente e entrega.
- Portas `5433` (PostgreSQL), `8080` (API) e `5173` (Vite) disponíveis.

### Configurar

Na raiz do repositório, crie a configuração local:

```sh
cp .env.example .env
```

Edite `.env` e defina uma senha forte em `POSTGRES_PASSWORD`. Use a mesma senha em `DATABASE_URL`. O arquivo `.env` é local e não deve ser versionado. `FABRICA_WORKSPACE` define onde os workspaces dos projetos serão criados (por padrão, `~/workspace`); `FABRICA_DB_PORT` altera a porta local do PostgreSQL; `OPENCODE_MODEL` seleciona o modelo do agente.

### Iniciar

Suba o PostgreSQL:

```sh
docker compose up -d --wait
```

Em um terminal, inicie a API. O script carrega `.env`, aplica as migrações pendentes e escuta em `127.0.0.1:8080`:

```sh
./scripts/run-api.sh
```

Em outro terminal, inicie o frontend:

```sh
cd frontend
npm ci
npm run dev
```

Acesse <http://127.0.0.1:5173>. O Vite encaminha as chamadas `/api` para a API. A saúde da API e a conexão com o banco podem ser conferidas em:

```sh
curl --fail http://127.0.0.1:8080/api/healthz
```

Para usar o modelo Codex com autenticação do plano ChatGPT pelo OpenCode, configure o provedor OpenAI no OpenCode e defina `OPENCODE_MODEL=codex` em `.env`; reinicie a API para aplicar a mudança.

## Desenvolvimento

Verificações disponíveis:

```sh
cd backend && go test ./...
cd ../frontend && npm run lint && npm run build
```

Mais detalhes sobre configuração, descritor de ambiente de QA, GitHub, backup e recuperação estão em [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md). A visão de produto está em [docs/PRD.md](docs/PRD.md).

## Estrutura do repositório

```text
backend/     API Go, lógica de domínio, persistência e migrações
frontend/    Interface React e TypeScript
docs/        Documentação de desenvolvimento e produto
openspec/    Especificações e mudanças do produto
scripts/     Scripts operacionais locais
compose.yaml PostgreSQL para desenvolvimento local
```

## Operação local

Para parar o banco preservando os dados, use `docker compose stop`. Para iniciar novamente, use `docker compose up -d --wait`. `docker compose down` remove container e rede, mas preserva o volume nomeado do PostgreSQL. Evite `docker compose down --volumes` em uma parada normal, pois isso apaga o banco local.

Os agentes e os ambientes de QA operam no host da Fábrica e acessam o workspace, Docker, OpenCode, Git e `gh` com o usuário que iniciou a API. Para configurar a autenticação e o fluxo de pull requests, consulte [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md#entrega-pelo-github).

## Licença

Consulte [LICENSE](LICENSE).
