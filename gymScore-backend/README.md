# GymScore — Backend (Go)

API do **GymScore**, uma plataforma que incentiva a prática de exercícios físicos por meio de desafios com prêmios, sistema de amigos, treinos compartilháveis e depósitos via PIX.

Construído em **Go** com Clean Architecture, framework **Fiber**, ORM **GORM** e **MySQL**.

---

## 🚀 Tecnologias

- **Go (Golang)** + **Fiber v2**
- **MySQL 8+** com **GORM** (auto-migração de schema)
- Autenticação **JWT** + senhas com **Bcrypt**
- Gateway de pagamento **Asaas** (PIX)
- Swagger para documentação da API

---

## 📦 Domínios da Aplicação

### 👤 Usuários
Cadastro, login (JWT), atualização de perfil, troca e recuperação de senha, saldo.

### 🤝 Amigos
Solicitações de amizade com fluxo completo: enviar, **aceitar/recusar** solicitações recebidas, cancelar enviadas e remover. A listagem distingue amizades aceitas e pendentes (campo `recebida`).

### 🏆 Desafios (multi-participante)
- Desafios com **vagas limitadas** e **prêmio fixo** bancado pelo criador.
- O criador ocupa a 1ª vaga automaticamente; outros usuários **entram** enquanto houver vaga.
- Ao **encerrar**, o criador indica o **vencedor** (que deve ser um participante); o vencedor recebe o prêmio e o criador paga (neutro se o próprio criador vence).
- **Visibilidade**: desafio lotado some da listagem para quem não participa — só o criador e inscritos continuam vendo.
- Suporta **coordenadas** (latitude/longitude) para busca por proximidade no frontend.
- Estados: `aberto`, `pendente`, `em_andamento`, `encerrado` + coluna `estado` (1=ativo, 0=fechado, 2=cancelado).

### 💪 Treinos
- Cada usuário cria **múltiplos treinos**, cada um com uma sequência de exercícios.
- Cada treino tem um **código de compartilhamento**; outro usuário **importa** pelo código (vira uma cópia própria).
- Registro de **conclusão de exercícios** alimenta um **radar** de grupos musculares (quanto cada parte do corpo foi treinada).

### 💸 Pagamento PIX (Asaas)
- Gera cobrança PIX (QR Code Base64 + Copia-e-Cola) via Asaas.
- Transação nasce como `pending`.
- **Confirmação por polling ativo**: o backend consulta o status direto na API do Asaas (não depende de webhook/URL pública). Ao confirmar, credita o saldo de forma **idempotente** (sem crédito duplo).
- **Toggle de ambiente** por uma única variável (`ASAAS_ENV=sandbox|prod`) — URL e chave são escolhidas automaticamente.
- Em **sandbox** há um endpoint de **simulação** de pagamento (bloqueado em produção).

---

## ⚙️ Configuração

1. Copie o exemplo de variáveis de ambiente:
   ```bash
   cp .env.example .env
   ```
2. Edite o `.env` com suas credenciais de banco e do Asaas. Veja `.env.example` para a descrição de cada variável.

### Alternar Sandbox ↔ Produção do PIX
Mude **apenas** a linha:
```env
ASAAS_ENV=sandbox   # ou: prod
```
A URL base (`api-sandbox.asaas.com` / `api.asaas.com`) e a chave (`ASAAS_API_KEY_SANDBOX` / `ASAAS_API_KEY_PROD`) são selecionadas automaticamente.

> No sandbox o QR Code é **fictício** (não paga em banco real) — use a simulação de pagamento para testar. QR pagável de verdade só em produção.

---

## 🛠️ Como Rodar

```bash
go mod tidy

# Desenvolvimento com hot-reload (recomendado)
air

# Ou diretamente
go run main.go
```

A API sobe em `http://localhost:3000`. O frontend (estático) é servido pelo mesmo servidor.
Swagger: `http://localhost:3000/swagger/`

> A primeira subida roda a auto-migração no MySQL — pode levar alguns segundos em bancos remotos.

---

## 🔌 Principais Endpoints

| Método | Rota | Descrição |
|--------|------|-----------|
| POST | `/api/login` | Autenticação (retorna JWT) |
| POST | `/api/usuarios` | Cadastro de usuário |
| GET  | `/api/desafios/view` | Lista desafios visíveis |
| POST | `/api/desafios` | Cria desafio |
| POST | `/api/desafios/aceitar_desafio` | Entra em um desafio (ocupa vaga) |
| POST | `/api/desafios/encerrar` | Encerra e indica o vencedor |
| GET  | `/api/amigos/:id` | Lista amigos e solicitações |
| POST | `/api/amigos/aceitar` | Aceita solicitação de amizade |
| GET  | `/api/treinos` | Lista treinos do usuário |
| POST | `/api/treinos` | Cria treino |
| POST | `/api/treinos/importar` | Importa treino por código |
| POST | `/api/treinos/concluir` | Registra exercício concluído |
| GET  | `/api/treinos/radar` | Radar de grupos musculares |
| POST | `/api/pagamento/pix` | Gera cobrança PIX |
| GET  | `/api/pagamento/pix/:id` | Consulta/confirma pagamento (polling) |
| POST | `/api/pagamento/pix/:id/simular` | Simula pagamento (somente sandbox) |

Rotas protegidas exigem o header:
```
Authorization: Bearer SEU_TOKEN
```

---

## 📁 Estrutura do Projeto

```
gymScore-backend/
├── database/
│   └── schema.sql               # DDL de referência do MySQL
├── internal/
│   ├── client/                  # Cliente HTTP do Asaas
│   ├── config/                  # Configurações e conexão com o DB (.env)
│   ├── controllers/             # Handlers HTTP
│   ├── middlewares/             # Auth (JWT), CORS, Logger, Recover
│   ├── models/                  # Entidades de domínio e DTOs
│   ├── repositories/            # Acesso a dados (GORM)
│   ├── routes/                  # Definição das rotas
│   └── services/                # Regras de negócio
├── pkg/
│   └── utils/                   # JWT, respostas, validações
├── .env.example                 # Exemplo de variáveis de ambiente
├── go.mod / go.sum
├── main.go                      # Ponto de entrada
└── README.md
```

---

## 🧪 Testes

```bash
go test ./... -v
```

---

## 🚀 Próximos Passos

- Webhook real do Asaas quando houver URL pública (hoje a confirmação é por polling).
- Geocodificação do campo "Local" do desafio para preencher coordenadas automaticamente.
- Expiração/cancelamento automático de cobranças PIX pendentes.
