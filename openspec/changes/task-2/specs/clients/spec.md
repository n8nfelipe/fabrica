# clients Specification

## Purpose

Disponibilizar o cadastro de clientes com dados consistentes, validação no servidor e no formulário, e operações REST documentadas para usuários autorizados.

## Requirements

### Requirement: Criar cliente validado

O sistema MUST permitir que um usuário autorizado crie um cliente com nome, telefone e email válidos, gerando o `id` no servidor.

#### Scenario: Criação válida

- **WHEN** o usuário envia nome entre 2 e 120 caracteres, telefone E.164 e email válido de até 254 caracteres
- **THEN** o sistema normaliza os campos definidos, persiste o cliente ativo e retorna `201` com `id`, nome, telefone, email e timestamps

#### Scenario: Campo inválido

- **WHEN** o usuário envia campo ausente, nome fora do limite, telefone fora de E.164 ou email inválido
- **THEN** o sistema não persiste o cliente e retorna `422` com erro identificável por campo

#### Scenario: Email já utilizado

- **WHEN** o usuário envia email equivalente, ignorando maiúsculas/minúsculas, ao de outro cliente ativo
- **THEN** o sistema não cria duplicata e retorna `409`

### Requirement: Consultar clientes

O sistema MUST permitir listar e consultar clientes ativos para usuários autorizados.

#### Scenario: Listar clientes

- **WHEN** usuário autorizado solicita `GET /api/clients`
- **THEN** o sistema retorna somente clientes não excluídos em JSON com schema documentado

#### Scenario: Cliente inexistente ou excluído

- **WHEN** usuário solicita um `id` inexistente ou logicamente excluído
- **THEN** o sistema retorna `404` sem revelar se um registro excluído existiu

### Requirement: Atualizar cliente

O sistema MUST permitir atualizar nome, telefone e email com as mesmas regras de validação da criação.

#### Scenario: Atualização válida

- **WHEN** usuário autorizado envia um PATCH válido para cliente ativo
- **THEN** o sistema persiste os novos valores normalizados e retorna o recurso atualizado

#### Scenario: Atualização inválida ou conflitante

- **WHEN** o PATCH contém valor inválido ou email usado por outro cliente ativo
- **THEN** o sistema retorna `422` ou `409`, respectivamente, sem alteração parcial

### Requirement: Excluir cliente

O sistema MUST permitir exclusão lógica de cliente ativo para usuário autorizado.

#### Scenario: Exclusão válida

- **WHEN** usuário autorizado envia `DELETE /api/clients/{id}`
- **THEN** o sistema registra `deleted_at`, deixa de retornar o cliente nas consultas padrão e responde `204`

#### Scenario: Exclusão repetida

- **WHEN** usuário tenta excluir cliente inexistente ou já excluído
- **THEN** o sistema responde `404` e não altera outros registros

### Requirement: Validar também no frontend

O formulário de cliente MUST aplicar os limites e formatos antes do envio e MUST apresentar erros retornados pelo backend junto aos campos correspondentes.

#### Scenario: Erro de formulário

- **WHEN** usuário deixa campo obrigatório vazio ou informa formato inválido
- **THEN** o frontend impede o envio e exibe mensagem acessível no campo sem chamar a API

#### Scenario: Erro de API

- **WHEN** a API retorna `409`, `422` ou falha transitória
- **THEN** o frontend mantém os valores digitados, apresenta feedback compreensível e permite correção ou nova tentativa

### Requirement: Documentar contrato

O sistema MUST manter documentação OpenAPI/Swagger para as cinco operações, schemas de cliente, regras de validação, autenticação e respostas de erro.

#### Scenario: Contrato verificável

- **WHEN** a documentação é consultada
- **THEN** cada rota implementada possui método, parâmetros, request body, response body, códigos de erro e exemplos coerentes com a API
