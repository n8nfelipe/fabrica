# Instruções do repositório

## Gerenciador de pacotes JavaScript

- O repositório MUST usar pnpm para instalar, adicionar, remover e executar dependências JavaScript. Não use npm ou Yarn para essas tarefas.
- Mantenha `pnpm-lock.yaml` atualizado e use `pnpm install --frozen-lockfile` na CI.

## OpenSpec

- Antes de iniciar qualquer tarefa, leia os arquivos de `openspec/` pertinentes ao escopo. Consulte sempre as especificações em `openspec/specs/` e os artefatos de mudanças ativas em `openspec/changes/`; leia o histórico em `openspec/changes/archive/` quando ele for relevante.
- Se a tarefa implementar ou alterar um requisito documentado no OpenSpec, siga esses artefatos e mantenha-os alinhados com a implementação.

## Testes e cobertura

- Toda funcionalidade nova e toda correção de funcionalidade existente MUST incluir ou atualizar testes que cubram os objetivos e critérios de aceitação da mudança, incluindo os caminhos de erro e os limites relevantes.
- A meta do projeto é alcançar e manter 100% de cobertura de testes. Ao alterar código, execute a suíte pertinente e confira a cobertura; não reduza a cobertura existente.
- Prefira testes unitários determinísticos com mocks para dependências externas, como banco de dados, serviços de rede, executáveis e APIs. Use testes de integração quando o comportamento depender da integração real entre componentes.
