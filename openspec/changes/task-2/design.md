# Design

## Abordagem

Implementar a capacidade seguindo a estrutura atual do repositório, sem criar um segundo runtime Laravel/Next.js. O domínio de cliente ficará isolado em um pacote próprio; o handler HTTP fará apenas autenticação, decodificação, validação de transporte e mapeamento de respostas. Casos de uso coordenarão regras e repositório, e a implementação PostgreSQL ficará atrás de uma interface.

## Modelo de dados

Criar uma tabela `clients` com:

- `id BIGSERIAL PRIMARY KEY`;
- `name TEXT NOT NULL`;
- `phone TEXT NOT NULL`;
- `email TEXT NOT NULL`;
- `deleted_at TIMESTAMPTZ NULL`;
- `created_at` e `updated_at` com timestamp timezone.

Adicionar índice/constraint de unicidade para email ativo, comparando a forma normalizada em minúsculas. A API não deve expor clientes com `deleted_at` preenchido. A normalização deve ocorrer no caso de uso, não depender apenas do frontend.

## API

Usar `GET /api/clients`, `GET /api/clients/{id}`, `POST /api/clients`, `PATCH /api/clients/{id}` e `DELETE /api/clients/{id}`. Respostas de sucesso serão JSON; criação retorna `201`, exclusão retorna `204`, recurso ausente retorna `404`, conflito de email retorna `409` e entrada inválida retorna `422` com erros por campo.

O contrato OpenAPI deve descrever schemas, formatos, limites, exemplos, respostas de erro e operações de autenticação. A localização final do documento deve permanecer dentro do escopo aprovado.

## Validação

- `name`: obrigatório, texto aparado, entre 2 e 120 caracteres.
- `phone`: obrigatório, somente formato internacional E.164 (`+` seguido de 8 a 15 dígitos), armazenado normalizado.
- `email`: obrigatório, formato de email válido, aparado e armazenado em minúsculas, máximo de 254 caracteres.
- `id`: inteiro positivo no caminho.

O frontend deve apresentar os mesmos limites antes do envio e continuar tratando as respostas `422` do backend como autoridade final. Não exibir detalhes internos de banco ou stack trace.

## Frontend

Adicionar uma área de clientes integrada à navegação existente, com estados de carregamento, vazio, erro, formulário de criação/edição, confirmação de exclusão e feedback de sucesso. Manter os estilos e dependências atuais; não adicionar framework de formulários sem necessidade.

## Segurança e observabilidade

Aplicar as ações de autorização existentes para leitura e organização de dados. Usar queries parametrizadas, não aceitar campos desconhecidos silenciosamente e não registrar dados pessoais em logs de erro. Testar acesso não autorizado, IDs inválidos, conflito de email, cliente ausente e exclusão repetida.

## Testes

Cobrir regras de validação e normalização, casos de uso, queries/repositório com mocks determinísticos, handlers HTTP e os fluxos principais da interface usando as ferramentas já presentes. Se a interface não possuir runner de testes, registrar a cobertura possível por lint/build e manter testes de contrato no backend.
