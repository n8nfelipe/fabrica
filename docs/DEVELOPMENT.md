# Desenvolvimento local

## Pré-requisitos

- Go 1.22 ou mais recente.
- Node.js 22 ou mais recente e pnpm 12.15.0.
- Docker Engine com o plugin Docker Compose v2.
- Portas locais `5433` (PostgreSQL), `8080` (API) e `5173` (Vite) disponíveis.

OpenCode, Git e GitHub CLI (`gh`) também serão necessários para executar os fluxos de agente e entrega descritos no produto. Para autenticar a integração GitHub, execute `gh auth login` como o mesmo usuário que inicia a fábrica.

## Preparar a configuração

Na raiz do repositório, crie o arquivo local de ambiente e defina uma senha forte para o PostgreSQL:

```sh
cp .env.example .env
```

Edite `.env` e substitua `POSTGRES_PASSWORD`. Use a mesma senha em `DATABASE_URL`, preservando o restante da URL. O arquivo `.env` é local e não deve ser versionado. Ajuste `FABRICA_DB_PORT` se a porta padrão `5433` já estiver ocupada. `FABRICA_WORKSPACE` define a raiz dos projetos e, por padrão, aponta para `~/workspace`. `OPENCODE_MODEL` seleciona o modelo dos agentes e, por padrão, usa `opencode/deepseek-v4.1-flash-free` pelo OpenCode Zen.

## Usar o Codex pelo plano ChatGPT

Também é possível usar o login OAuth do plano ChatGPT pelo provedor OpenAI do OpenCode. No terminal do mesmo usuário que inicia a API, abra o OpenCode, execute `/connect`, escolha `OpenAI` e depois `ChatGPT Plus/Pro`. Em seguida, altere `.env` para selecionar o atalho Codex:

```dotenv
OPENCODE_MODEL=codex
```

O atalho seleciona `openai/gpt-5.6-luna`. Também é possível definir esse ID diretamente em `OPENCODE_MODEL`. Reinicie a API para aplicar o modelo. A autenticação e a seleção do modelo são separadas: o login ChatGPT Plus/Pro habilita a rota Codex. O padrão continua sendo o modelo gratuito do OpenCode Zen.

## Iniciar a fábrica

Em um terminal, na raiz do repositório, inicie o PostgreSQL:

```sh
docker compose up -d --wait
```

Em outro terminal, inicie a API. O script carrega `.env`, aplica as migrações pendentes e escuta somente em loopback:

```sh
./scripts/run-api.sh
```

Em um terceiro terminal, instale as dependências do frontend e inicie o servidor de desenvolvimento:

```sh
cd frontend
pnpm install --frozen-lockfile
pnpm run dev
```

Abra <http://127.0.0.1:5173>. O Vite encaminha chamadas `/api` para a API em `127.0.0.1:8080`. Para verificar a saúde da API e do banco:

```sh
curl --fail http://127.0.0.1:8080/api/healthz
```

Uma resposta saudável informa `"status":"ok"` e `"database":"connected"`.

## Descritor de ambiente de projeto

Cada projeto gerado deve incluir `.fabrica/project.json` na raiz do workspace. O descritor aponta para o arquivo Compose da stack e identifica o serviço e a porta interna que o QA deve acessar. O `healthCheck` é opcional.

```json
{
  "version": 1,
  "composeFile": "compose.yaml",
  "qaService": "web",
  "containerPort": 4173,
  "healthCheck": {
    "path": "/healthz"
  }
}
```

`composeFile` deve ser um caminho relativo a partir da raiz do projeto e resolver para um arquivo regular dentro do mesmo workspace. Caminhos absolutos, traversal (`..`) e links simbólicos que saiam do workspace são rejeitados. `qaService` precisa corresponder a um serviço do Compose, `containerPort` deve estar entre 1 e 65535, e o health check deve ser um caminho local como `/healthz`.

Em stacks com banco de dados, a aplicação deve conectar ao nome do serviço Compose e à porta interna do banco. Não publique a porta padrão do banco no host por padrão; assim, subir o projeto manualmente com `docker compose up` não colide com outro PostgreSQL local. Se os requisitos exigirem acesso ao banco pelo host, use uma porta configurável e vincule-a somente a `127.0.0.1`.

O executor valida o arquivo com `docker compose config --quiet` e usa o ID do projeto como nome Compose (`fabrica-<id>`). Assim, `up`, inspeção, `stop` e `restart` atuam somente sobre os containers daquele projeto; volumes nomeados descritos pela stack permanecem associados ao mesmo projeto.

A fábrica aloca portas do pool local `41000–41999`, registra a lease no PostgreSQL e publica a porta do serviço QA somente em `127.0.0.1`. A publicação vem de um override gerenciado em `.fabrica/runtime-compose.yaml`; o Compose do projeto deve declarar a porta interna do serviço e não fixar uma porta de host.

Quando a implementação chega a `Aguardando QA`, uma pessoa com perfil QA pode iniciar o ambiente pela tela da tarefa. A fábrica aguarda o caminho de `healthCheck` (ou `/` quando não configurado) responder com HTTP 2xx/3xx antes de gravar e exibir a URL local. Se o serviço não ficar saudável, o link não é liberado, a stack é parada e a porta é liberada. O início da revisão (`Em QA`) só é aceito quando o ambiente está saudável. A tela também oferece reinício, parada, estado e logs recentes do serviço QA; logs passam pela mesma redaction de credenciais usada nas execuções OpenCode.

## Entrega pelo GitHub

O executor precisa ter `gh` instalado, estar em um workspace que seja a raiz de um repositório Git e ter um remoto `origin` configurado. Faça login uma vez com `gh auth login` e configure o Git para usar as credenciais do CLI com `gh auth setup-git`; a Fábrica não lê nem armazena tokens.

Depois da aprovação de QA, uma pessoa de PO, QA, Desenvolvimento ou Admin pode criar o PR na tarefa. A Fábrica cria uma branch `fabrica/task-<id>-<titulo>`, inclui somente os arquivos registrados na implementação aprovada, envia ao remoto e pede ao `gh` que crie o PR. Checks são consultados como JSON e o merge é executado manualmente pela interface somente depois que todos os checks obrigatórios passarem. O `gh` usa o repositório associado ao `origin`.

Se o status de autenticação falhar, rode `gh auth status` e repita o login. Erros de remoto ou envio pedem que se confira `git remote -v` e `gh auth setup-git`; a tarefa permanece aguardando PR e pode ser tentada novamente. Não coloque tokens em requisitos, prompts, arquivos de configuração da Fábrica ou variáveis persistidas.

## Parar e reiniciar

- Pare a API e o Vite com `Ctrl+C` nos respectivos terminais.
- Pare o PostgreSQL, mantendo o volume persistente, com `docker compose stop`.
- Para iniciar novamente, use `docker compose up -d --wait` e repita os comandos da API e do frontend.
- `docker compose down` remove o container e a rede, mas preserva o volume de dados nomeado.

Não use `docker compose down --volumes` para uma parada normal: essa opção apaga o banco local da fábrica.

## Backup e recuperação

Faça um dump antes de atualizar ou mover a instalação. Mantenha o arquivo em local protegido: ele contém contas, sessões, requisitos, histórico e referências de tarefas.

```sh
set -a
. ./.env
set +a
docker compose exec -T postgres pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" -Fc > fabrica-backup.dump
```

Para restaurar, pare a API primeiro, confirme que o PostgreSQL está ativo e carregue a mesma configuração `.env`. O comando substitui os objetos daquele banco; guarde o dump atual antes de executá-lo.

```sh
set -a
. ./.env
set +a
docker compose exec -T postgres pg_restore -U "$POSTGRES_USER" -d "$POSTGRES_DB" --clean --if-exists --no-owner < fabrica-backup.dump
```

## Limpeza de ambientes e limites locais

Use **Parar ambiente** na tarefa para encerrar a stack e liberar a lease da porta. Para remover containers e redes parados, execute no workspace do projeto `docker compose --project-directory "$PWD" -f <compose-file-do-descritor> -f .fabrica/runtime-compose.yaml -p fabrica-<id> down`; mantenha volumes nomeados se eles guardarem dados que serão usados numa próxima revisão. `docker compose down --volumes` também apaga esses dados e deve ser uma decisão intencional.

O executor é local e usa o usuário do processo da Fábrica para acessar arquivos, Docker, OpenCode, Git e `gh`. Docker Compose mantém serviços e volumes do projeto no mesmo host. A aplicação não provisiona runners remotos, não cria repositórios GitHub e não guarda credenciais do GitHub; o operador prepara o workspace, o remoto e a autenticação do `gh`.

## Verificações de desenvolvimento

```sh
cd backend
go test ./...
cd ../frontend
pnpm run lint
pnpm run build
```

Esses comandos executam os testes Go, o lint e a compilação de produção do frontend.
