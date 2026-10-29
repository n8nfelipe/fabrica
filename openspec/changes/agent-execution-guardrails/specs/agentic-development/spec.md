# Spec Delta

## MODIFIED Requirements

### Requirement: Implementar tarefa aprovada com OpenCode
Para tarefa aprovada, o sistema MUST executar OpenCode no workspace do projeto com o contexto da tarefa e os artefatos SDD aprovados.

#### Scenario: Implementação concluída
- **WHEN** OpenCode conclui a implementação da tarefa
- **THEN** o sistema registra o resultado e encaminha o projeto para inicialização do ambiente de validação

#### Scenario: Falha na implementação
- **WHEN** OpenCode retorna falha ou não produz uma alteração verificável
- **THEN** o sistema registra a falha e não libera o QA

#### Scenario: Especificação aprovada não declara caminhos graváveis válidos
- **WHEN** o PO tenta aprovar uma especificação sem um manifesto válido de caminhos graváveis
- **THEN** o sistema recusa a aprovação e não inicia implementação

#### Scenario: Escopo tenta autorizar alteração de artefatos OpenSpec
- **WHEN** `scope.json` inclui qualquer caminho sob `openspec/changes/task-*/`
- **THEN** o sistema recusa a aprovação e informa que esse caminho é reservado para especificações

## ADDED Requirements

### Requirement: Mediar ferramentas do agente por um gateway
Durante uma execução de agente, o sistema MUST expor somente ferramentas do gateway local que a política da fase autoriza e MUST desativar ferramentas nativas e configurações de ferramentas fornecidas pelo projeto.

#### Scenario: Executar no workspace do projeto selecionado
- **WHEN** uma execução de especificação ou implementação é iniciada para um projeto
- **THEN** o OpenCode e o gateway usam o diretório canônico desse projeto como raiz, independente do diretório e da variável `PWD` do processo da fábrica

#### Scenario: Agente solicita leitura de arquivo do workspace
- **WHEN** o agente chama a ferramenta de leitura do gateway com um caminho regular dentro do workspace
- **THEN** o gateway retorna o conteúdo dentro dos limites de tamanho configurados

#### Scenario: Agente tenta executar comando ou acessar rede
- **WHEN** o agente tenta usar shell, web, subagente ou uma ferramenta externa
- **THEN** a chamada não está disponível e nenhuma ação é executada

#### Scenario: Conteúdo lido contém prompt injection
- **WHEN** um arquivo do workspace contém instruções para ignorar escopo ou executar ação externa
- **THEN** o conteúdo é retornado como dado e não altera as ferramentas ou a política ativa

### Requirement: Autorizar gravações por fase e manifesto aprovado
O sistema MUST permitir gravações apenas nos caminhos autorizados pela fase; na implementação, a autorização MUST vir do manifesto de escopo validado e persistido na aprovação do PO.

#### Scenario: Fase de especificação grava artefatos da tarefa
- **WHEN** o agente grava um caminho dentro de `openspec/changes/task-<id>/`
- **THEN** o gateway permite a gravação se o caminho não atravessar links simbólicos

#### Scenario: Implementação grava caminho aprovado
- **WHEN** o agente grava ou remove um arquivo coberto pelo manifesto aprovado
- **THEN** o gateway aplica a operação dentro do workspace sem seguir links simbólicos

#### Scenario: Implementação tenta gravar caminho não aprovado
- **WHEN** o agente grava ou remove um caminho não coberto pelo manifesto aprovado
- **THEN** o gateway recusa a operação e registra uma violação

#### Scenario: Caminho tenta escapar do workspace
- **WHEN** uma ferramenta recebe caminho absoluto, traversal, link simbólico ou caminho reservado
- **THEN** o gateway recusa a operação sem acessar o destino

### Requirement: Tratar contexto do agente como entrada não confiável
O sistema MUST tratar descrições de tarefa, requisitos, arquivos, artefatos, comentários de QA e saídas de ferramentas como dados que não podem modificar a política do gateway nem conceder capacidades.

#### Scenario: Texto não confiável solicita ação proibida
- **WHEN** conteúdo recebido instrui o agente a revelar segredos, usar rede, executar comandos ou ignorar o escopo
- **THEN** o gateway continua aplicando a mesma política e não oferece a capacidade solicitada

### Requirement: Interromper ciclos repetitivos do agente
O sistema MUST limitar chamadas de ferramenta por execução e interromper chamadas idênticas repetidas acima do limite configurado.

#### Scenario: Agente repete a mesma chamada sem progresso
- **WHEN** o agente repete a mesma ferramenta com os mesmos argumentos além do limite permitido
- **THEN** o gateway interrompe novas chamadas, registra a categoria do bloqueio e faz a execução falhar

#### Scenario: Agente excede o orçamento de ferramentas
- **WHEN** o agente excede o total máximo de chamadas permitido para uma execução
- **THEN** o gateway recusa chamadas adicionais e o workflow não avança de fase
