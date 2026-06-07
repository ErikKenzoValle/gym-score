# 🏋️ GymScore

**GymScore** é uma plataforma que incentiva a prática de exercícios físicos e musculação por meio de **desafios com prêmios**, **sistema de amigos**, **treinos compartilháveis** e **depósitos via PIX**.

A ideia é transformar a rotina de treino em algo social e motivador: você cria ou entra em desafios com prêmios em dinheiro, monta e compartilha treinos, acompanha seu progresso em um radar de grupos musculares e gerencia seu saldo direto pelo app.

---

## 🧱 Arquitetura

O projeto é dividido em duas partes:

| Pasta | O que é | Stack |
|-------|---------|-------|
| [`gymScore-backend/`](./gymScore-backend) | API REST e regras de negócio | Go, Fiber, GORM, MySQL |
| [`gymScore-frontend/`](./gymScore-frontend) | Interface web | HTML, CSS e JavaScript puro |

O **backend serve o frontend** como arquivos estáticos — ao subir o backend, a aplicação completa fica disponível em `http://localhost:3000`.

---

## ✨ Principais Funcionalidades

- **Autenticação** com JWT (cadastro, login, recuperação de senha)
- **Desafios** multi-participante com vagas limitadas, prêmio em dinheiro e indicação de vencedor
- **Filtros de desafios** por status, faixa de valor e proximidade geográfica
- **Amigos** com solicitações (enviar, aceitar, recusar, remover)
- **Treinos** próprios e múltiplos, com compartilhamento por código e modo guiado de execução
- **Radar muscular** que evolui conforme você completa exercícios
- **Depósitos via PIX** (gateway Asaas) com confirmação automática

---

## 🚀 Começando

### Pré-requisitos
- [Go](https://go.dev/) 1.21+
- MySQL 8+
- Conta no [Asaas](https://www.asaas.com/) (sandbox para testes) — opcional, só para o PIX
- (Opcional) [air](https://github.com/air-verse/air) para hot-reload

### Passos

```bash
# 1. Configure o backend
cd gymScore-backend
cp .env.example .env
# edite o .env com suas credenciais (banco e Asaas)

# 2. Suba a aplicação (sobe API + serve o frontend)
go mod tidy
air            # ou: go run main.go
```

Acesse **http://localhost:3000**.

> Na primeira execução, o backend cria/ajusta as tabelas automaticamente (auto-migração). Em bancos remotos isso pode levar alguns segundos.

---

## 📚 Documentação

- **Backend** (arquitetura, endpoints, configuração): [`gymScore-backend/README.md`](./gymScore-backend/README.md)
- **Frontend** (telas e funcionalidades): [`gymScore-frontend/README.md`](./gymScore-frontend/README.md)
- **API (Swagger)**: `http://localhost:3000/swagger/` com o backend rodando

---

## 💸 Sobre o PIX

O pagamento usa o gateway **Asaas** e alterna entre testes e produção com uma única variável (`ASAAS_ENV`):

- `sandbox` — ambiente de testes; o QR Code é fictício, mas dá para **simular** o pagamento.
- `prod` — produção; QR Code real, pagável em qualquer banco.

A confirmação do pagamento é feita por **polling ativo** na API do Asaas, sem necessidade de webhook/URL pública. Detalhes no README do backend.
