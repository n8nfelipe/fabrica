# Tarefas

## 1. Domínio e persistência

- [ ] 1.1 Registrar a decisão do PO sobre a divergência entre a stack solicitada (Laravel/Next.js) e a stack existente (Go/React/Vite); sem decisão de migração, manter a implementação na stack existente.
- [ ] 1.2 Criar entidade/DTOs, regras de normalização e validação de cliente, incluindo limites e erros por campo.
- [ ] 1.3 Criar interfaces de caso de uso e repositório, com implementação PostgreSQL parametrizada e exclusão lógica.
- [ ] 1.4 Adicionar migração, índice/constraint de email ativo e testes de persistência, exclusão lógica e concorrência do email.

## 2. API e documentação

- [ ] 2.1 Implementar rotas REST de listagem, consulta, criação, atualização e exclusão, integradas à autorização existente.
- [ ] 2.2 Mapear respostas `201`, `204`, `404`, `409` e `422` sem expor detalhes internos.
- [ ] 2.3 Publicar o contrato OpenAPI/Swagger com schemas, limites, exemplos e erros.
- [ ] 2.4 Adicionar testes de handler para sucesso, payload inválido, campo desconhecido, ID inválido, conflito, ausência, acesso não autorizado e exclusão repetida.

## 3. Frontend

- [ ] 3.1 Adicionar tipos, cliente de API e tela de listagem com loading, vazio e erro.
- [ ] 3.2 Adicionar formulário reutilizável de criação/edição com validação de nome, telefone e email.
- [ ] 3.3 Adicionar confirmação e feedback para exclusão, incluindo atualização sem recarregamento completo.
- [ ] 3.4 Validar acessibilidade básica, responsividade, lint e build.

## 4. Verificação

- [ ] 4.1 Executar testes backend pertinentes e verificar cobertura sem redução.
- [ ] 4.2 Executar lint/build frontend e revisar o contrato OpenAPI contra os handlers.
- [ ] 4.3 Conferir que todas as alterações estão no `scope.json` e que não há segredos ou dados pessoais em logs/documentação.
