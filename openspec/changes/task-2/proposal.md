# Proposta: CRUD de clientes

## Objetivo

Adicionar cadastro, consulta, edição e exclusão lógica de clientes com `id`, `nome`, `telefone` e `email`. A mesma regra de validação deve existir no servidor e no formulário, e o contrato REST deve ser publicado em OpenAPI/Swagger.

## Contexto e incertezas

- O pedido informa PHP/Laravel e React/Next.js, mas a estrutura observada do repositório é uma API Go, frontend React/Vite e PostgreSQL. Este plano segue a stack existente para não introduzir um segundo runtime; iniciar uma migração exige decisão explícita do PO e não faz parte deste task.
- O endpoint e as telas devem respeitar a autenticação e autorização já existentes.
- Será usado `id` inteiro gerado pelo banco, e o email será único sem diferenciação por maiúsculas/minúsculas.
- A exclusão será lógica para preservar rastreabilidade; clientes excluídos não aparecem nas consultas padrão.

## Escopo

- Domínio, casos de uso, repositório e migração PostgreSQL de clientes.
- Migração PostgreSQL e endpoints REST para listar, consultar, criar, atualizar e excluir clientes.
- Contrato OpenAPI/Swagger para os endpoints e erros de validação.
- Tela React para listagem, criação, edição, exclusão e mensagens de validação.
- Testes determinísticos dos fluxos novos, incluindo caminhos de erro.

## Fora do escopo

Importação/exportação, busca avançada, paginação configurável, histórico de alterações, restauração de excluídos, integração com CRM e mudança da stack atual.
