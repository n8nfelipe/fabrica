# Spec Delta

## MODIFIED Requirements

### Requirement: Executar projeto localmente
O sistema MUST iniciar, inspecionar e encerrar os serviços de cada projeto usando Docker Compose, mantendo os workspaces em `~/workspace/<nome-do-projeto>`. Ao concluir a implementação, o sistema MUST tentar iniciar o ambiente de revisão automaticamente, registrar o resultado e tornar falhas e informações úteis para correção visíveis a QA.

#### Scenario: Subir ambiente de revisão
- **WHEN** implementação está pronta para QA
- **THEN** o sistema inicia os serviços definidos no Compose do projeto e registra o estado de cada serviço

#### Scenario: Inicialização automática ao liberar QA
- **WHEN** a implementação termina e a tarefa entra em QA
- **THEN** o sistema inicia os serviços de revisão automaticamente e informa que o ambiente está iniciando

#### Scenario: Falha ao subir serviços
- **WHEN** um serviço necessário falha ao iniciar ou fica indisponível
- **THEN** o sistema não libera o link de QA, apresenta o estado de falha, o erro e os logs úteis para correção, e permite tentar iniciar novamente

#### Scenario: Diagnosticar falha antes de validar o descritor
- **WHEN** o descritor do projeto está ausente, inválido ou resolve para fora do workspace
- **THEN** o sistema mantém visível o erro de configuração e tenta obter logs apenas dos containers rotulados para o projeto, sem seguir caminhos do descritor

### Requirement: Disponibilizar link local para QA
O sistema MUST fornecer ao QA uma URL local para a versão do projeto associada à tarefa somente depois de verificar que o serviço de aplicação está acessível e MUST impedir conflito com portas já ocupadas ou alocadas a outros projetos gerenciados. A tela da tarefa MUST atualizar o estado até a inicialização terminar ou falhar.

#### Scenario: Projeto fica disponível
- **WHEN** serviços necessários iniciam com sucesso e o serviço de aplicação responde ao health check configurado
- **THEN** o sistema fornece uma URL local válida e associada ao projeto e à tarefa

#### Scenario: Porta candidata ocupada
- **WHEN** a porta candidata está em uso por outro processo ou projeto
- **THEN** o sistema aloca uma porta alternativa sem interromper o serviço existente

#### Scenario: QA consulta enquanto o ambiente inicia
- **WHEN** QA abre a tarefa enquanto os serviços ainda estão iniciando
- **THEN** o sistema apresenta o estado de inicialização e atualiza a tarefa até exibir o link após confirmar a acessibilidade ou apresentar a falha

#### Scenario: Iniciar revisão somente após o ambiente ficar saudável
- **WHEN** a tarefa aguarda QA e o ambiente está parado, iniciando ou falhou no health check
- **THEN** o sistema mantém a revisão bloqueada, informa como iniciar ou tentar novamente o ambiente e só oferece a ação de iniciar QA depois que o health check passar

#### Scenario: Evento de falha chega após o estado de falha
- **WHEN** a tela observa `unhealthy` antes de o evento de falha ser persistido
- **THEN** a tela tenta consultar o evento novamente e apresenta o motivo registrado quando estiver disponível

## ADDED Requirements

### Requirement: Evitar publicação padrão da porta do banco no host
Projetos que executam banco de dados em Docker Compose MUST permitir que a aplicação acesse o banco pela rede interna do Compose sem publicar a porta padrão do banco no host. Publicação para acesso pelo host MUST ser opcional, configurável e vinculada ao loopback.

#### Scenario: Aplicação conecta ao banco pela rede interna
- **WHEN** a stack do projeto inclui aplicação e banco de dados em serviços Compose
- **THEN** a aplicação conecta ao nome do serviço do banco e à porta interna, sem exigir publicação da porta padrão no host

#### Scenario: Porta padrão do banco já está ocupada
- **WHEN** a porta padrão do banco já está ocupada no host e o projeto não exige acesso externo ao banco
- **THEN** o Compose do projeto sobe sem tentar vincular essa porta no host e a aplicação acessa o banco pela rede interna

#### Scenario: Projeto exige acesso local ao banco
- **WHEN** os requisitos do projeto pedem acesso ao banco a partir do host
- **THEN** a configuração permite escolher a porta publicada e a vincula somente a `127.0.0.1`
