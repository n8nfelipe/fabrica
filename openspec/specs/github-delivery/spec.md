# github-delivery Specification

## Purpose

Conecta tarefas aprovadas ao fluxo de pull requests do GitHub, mantendo verificações de qualidade antes do merge e a criação de releases vinculada somente à integração em `main`.

## Requirements

### Requirement: Criar PR após aprovação do QA
O sistema MUST usar o GitHub CLI autenticado no executor para criar um pull request associado à tarefa somente depois que QA aprovar a implementação.

#### Scenario: QA aprovado
- **WHEN** QA aprova a implementação e o executor `gh` está autenticado
- **THEN** o agente cria um pull request no repositório do projeto e associa sua referência à tarefa

#### Scenario: QA ainda não aprovado
- **WHEN** automação tenta abrir PR antes da aprovação de QA
- **THEN** o sistema bloqueia a operação

#### Scenario: GitHub CLI não autenticado ou indisponível
- **WHEN** a criação do PR falha por indisponibilidade ou autenticação do `gh`
- **THEN** o sistema registra a falha sem marcar o PR como criado e permite retentativa autorizada

### Requirement: Verificar pull requests
O workflow de CI MUST ser acionado somente em eventos de pull request e executar verificações de testes, lint, typecheck e análise estática. O job MUST gerar o relatório de cobertura Go antes da análise CI-based do SonarQube Cloud e configurar o scanner para importar esse relatório.

#### Scenario: Abrir ou atualizar PR
- **WHEN** pull request é aberto ou atualizado
- **THEN** o workflow executa as verificações de qualidade exigidas

#### Scenario: Importar cobertura Go no SonarQube Cloud
- **WHEN** o workflow analisa um pull request
- **THEN** os testes Go geram `coverage.out` na raiz do checkout, os arquivos `backend/**/*_test.go` são classificados como testes e o SonarScanner importa o relatório sem incluir os testes nas métricas de código de produção

#### Scenario: Integrar na branch principal
- **WHEN** commit é enviado à branch `main` após merge
- **THEN** o workflow de release não executa verificações de PR novamente

### Requirement: Criar release após merge
O workflow de release MUST ser acionado somente por push à `main` e criar tag anotada SemVer com prefixo `v`, iniciando a sequência em `v1.0.0`, sem executar verificações de PR.

#### Scenario: Merge de mudança nova
- **WHEN** merge atualiza `main` e o HEAD ainda não tem tag
- **THEN** o workflow cria e envia a tag SemVer adequada ao tipo de mudança desde a tag anterior

#### Scenario: HEAD já marcado
- **WHEN** workflow de release é acionado para um commit que já tem tag
- **THEN** o workflow não cria uma tag duplicada

### Requirement: Restringir merge ao estado aprovado
O sistema MUST permitir merge de PR somente quando todos os checks obrigatórios forem aprovados e o ator for Admin, Desenvolvedor, QA ou PO.

#### Scenario: Merge elegível
- **WHEN** todos os checks obrigatórios passaram e um ator autorizado solicita merge
- **THEN** o sistema realiza ou solicita o merge e registra ator e resultado na tarefa

#### Scenario: Checks pendentes ou reprovados
- **WHEN** merge é solicitado com check obrigatório pendente ou reprovado
- **THEN** o sistema bloqueia o merge

#### Scenario: Perfil sem permissão de merge
- **WHEN** um ator sem permissão de merge solicita a operação
- **THEN** o sistema recusa a operação
