# Spec Delta

## Purpose

Cria e executa projetos em workspaces locais isolados, permitindo que cada projeto declare sua própria stack e oferecendo ao QA um ambiente acessível na mesma máquina sem conflito de portas.

## ADDED Requirements

### Requirement: Definir stack livre por projeto
O sistema MUST aceitar requisitos de stack específicos por projeto, sem catálogo tecnológico obrigatório, e MUST exigir que o agente produza uma configuração Docker Compose compatível para os serviços do projeto.

#### Scenario: Criar projeto com stack própria
- **WHEN** usuário descreve a stack e os requisitos de um novo projeto
- **THEN** o agente usa essa definição para gerar os componentes e a configuração Docker Compose do projeto

#### Scenario: Banco do projeto em container
- **WHEN** a stack do projeto inclui um banco de dados
- **THEN** o ambiente Compose do projeto sobe o banco como container e configura os serviços da aplicação para acessá-lo

### Requirement: Executar projeto localmente
O sistema MUST iniciar, inspecionar e encerrar os serviços de cada projeto usando Docker Compose, mantendo os workspaces em `~/workspace/<nome-do-projeto>`.

#### Scenario: Subir ambiente de revisão
- **WHEN** implementação está pronta para QA
- **THEN** o sistema inicia os serviços definidos no Compose do projeto e registra o estado de cada serviço

#### Scenario: Falha ao subir serviços
- **WHEN** um serviço necessário falha ao iniciar ou fica indisponível
- **THEN** o sistema não libera o link de QA e apresenta estado e logs úteis para correção

### Requirement: Disponibilizar link local para QA
O sistema MUST fornecer ao QA uma URL local para a versão do projeto associada à tarefa e MUST impedir conflito com portas já ocupadas ou alocadas a outros projetos gerenciados.

#### Scenario: Projeto fica disponível
- **WHEN** serviços necessários iniciam com sucesso
- **THEN** o sistema fornece uma URL local válida e associada ao projeto e à tarefa

#### Scenario: Porta candidata ocupada
- **WHEN** a porta candidata está em uso por outro processo ou projeto
- **THEN** o sistema aloca uma porta alternativa sem interromper o serviço existente

### Requirement: Isolar ciclos de vida dos ambientes
O sistema MUST permitir identificar, parar e reiniciar o ambiente de um projeto sem afetar os demais projetos gerenciados.

#### Scenario: Parar ambiente de um projeto
- **WHEN** usuário autorizado encerra o ambiente de revisão de um projeto
- **THEN** somente os serviços e recursos associados a esse projeto são parados

#### Scenario: Reiniciar ambiente
- **WHEN** usuário autorizado reinicia ambiente parado
- **THEN** o sistema inicia os serviços do projeto e atualiza a URL se a porta atribuída mudar
