# Spec Delta

## Purpose

Permite que a equipe administre contas e aplique permissões por perfil em todas as etapas da fábrica, mantendo as ações administrativas e de aprovação restritas aos atores autorizados.

## ADDED Requirements

### Requirement: Administrador gerencia usuários
O sistema MUST permitir que Admin crie, liste, consulte, edite e desative usuários. A desativação deve preservar o registro para auditoria e referências históricas.

#### Scenario: Criar usuário com perfil
- **WHEN** Admin cadastra um usuário com dados válidos e um perfil inicial
- **THEN** o sistema cria uma conta ativa associada ao perfil escolhido

#### Scenario: Desativar usuário
- **WHEN** Admin desativa uma conta
- **THEN** o sistema bloqueia novas autenticações para essa conta e mantém seu registro histórico

#### Scenario: Ação administrativa sem permissão
- **WHEN** um usuário que não é Admin tenta criar, editar ou desativar uma conta
- **THEN** o sistema recusa a operação e não altera os dados do usuário

### Requirement: Atribuir perfis iniciais
O sistema MUST oferecer os perfis PO, QA, Admin, Scrum Master e Desenvolvedor para atribuição às contas.

#### Scenario: Atribuir perfil disponível
- **WHEN** Admin cria ou edita uma conta e escolhe um dos cinco perfis iniciais
- **THEN** o perfil é persistido e usado nas próximas decisões de autorização

### Requirement: Restringir ações por perfil
O sistema MUST autorizar ações conforme o perfil: PO avalia especificações e pode fazer merge; QA registra resultados de QA e pode fazer merge; Desenvolvedor implementa e pode fazer merge; Scrum Master organiza projetos e tarefas e acompanha o fluxo, mas não administra usuários, aprova especificações/QA ou faz merge; Admin pode executar todas as ações. Ações não concedidas explicitamente a cada perfil devem ser negadas.

#### Scenario: Ação permitida ao perfil
- **WHEN** usuário autenticado executa uma ação concedida ao seu perfil
- **THEN** o sistema permite a ação e registra o ator

#### Scenario: Ação não permitida ao perfil
- **WHEN** usuário tenta executar uma ação não concedida ao seu perfil
- **THEN** o sistema recusa a ação sem avançar o fluxo

### Requirement: Exigir conta ativa para autenticação
O sistema MUST autenticar usuários por conta individual e impedir autenticação de contas desativadas.

#### Scenario: Autenticação de conta ativa
- **WHEN** usuário ativo apresenta credenciais válidas
- **THEN** o sistema inicia uma sessão associada à identidade e ao perfil do usuário

#### Scenario: Autenticação de conta desativada
- **WHEN** usuário desativado tenta autenticar
- **THEN** o sistema recusa a autenticação

### Requirement: Inicializar o primeiro administrador
Antes de existir qualquer usuário, o sistema MUST oferecer uma forma controlada de criar a primeira conta Admin sem credenciais padrão compartilhadas.

#### Scenario: Fábrica sem usuários
- **WHEN** a fábrica inicia sem contas cadastradas
- **THEN** somente o fluxo de inicialização de Admin fica disponível para configurar a primeira conta

#### Scenario: Primeiro administrador criado
- **WHEN** a primeira conta Admin é criada
- **THEN** o fluxo de inicialização é encerrado e novas contas só podem ser geridas por Admin
