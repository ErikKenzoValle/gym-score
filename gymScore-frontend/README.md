# GymScore — Frontend

Interface web do **GymScore**, uma plataforma que incentiva a prática de exercícios físicos por meio de desafios com prêmios, sistema de amigos, treinos compartilháveis e depósitos via PIX.

Construído em **HTML, CSS e JavaScript puro** (sem framework/build), servido como estático pelo backend Go.

---

## 🧩 Stack

- HTML5 + CSS3 (tema escuro, mobile-first)
- JavaScript puro (sem dependências externas)
- Comunicação com a API via `fetch` (JWT no `localStorage` + cookie HttpOnly)
- Radar de treinos desenhado em **SVG puro**, sem bibliotecas

> Não há etapa de build. Os arquivos em `public/` são servidos diretamente.

---

## 📁 Estrutura

```
gymScore-frontend/
└── public/
    ├── index.html          # Redireciona para /login ou /menu conforme o token
    ├── css/                # Estilos por página + base.css (tema, sino, abas, utilitários)
    ├── img/                # Imagens
    ├── js/
    │   └── api.js          # Auth, escHtml/h() (anti-XSS), dinheiro (centavos), sino, API
    └── html/
        ├── login.html / criar-conta.html
        ├── esqueci-senha.html / redefinir-senha.html   # link por e-mail (token, 30 min)
        ├── menu-principal.html
        ├── desafios-detalhado.html   # Lista, criação e todo o ciclo do desafio (rota /desafios)
        ├── amigos.html               # Amigos, solicitações e ranking entre amigos
        ├── treinos.html              # Treinos, modo guiado e radar muscular
        ├── carteira.html             # Saldo, extrato paginado e saque PIX (rota /carteira)
        ├── depositar.html            # Depósito via PIX
        ├── perfil.html / alterar-perfil.html / alterar-senha.html
        └── privacidade.html          # Política de privacidade (LGPD)
```

---

## 🛡️ Convenções do código

- **Dinheiro**: o backend fala em centavos (`*_centavos`). O frontend converte só na borda:
  `fmtBRL(centavos)` para exibir e `reaisParaCentavos("12,50")` para ler campos (sem aritmética de float).
- **Identidade**: nenhuma requisição envia `id_usuario`/`id_criador`; quem age é sempre o JWT.
  Ids de terceiros só como alvo (`id_amigo`, `id_votado`).
- **XSS**: todo conteúdo vindo da API é inserido com `textContent`/`h()` (criação de elementos) e
  `addEventListener`. `escHtml()` (escapa `& < > " '`) existe em `api.js` para o caso excepcional de HTML montado em string.
  Os únicos `innerHTML` restantes são SVG estáticos (ícones do nav e radar de treinos, sem dados do usuário).
  Os `onclick` que sobraram são estáticos, com argumentos literais.
- **Sessão**: `Authorization: Bearer` (token no `localStorage`) + cookie HttpOnly do backend.
  O logout chama `POST /api/logout` e limpa o storage; um 401 em rota protegida volta para o login.
- **Perfil**: `/api/usuarios/me` é a fonte do saldo e do CPF (`Auth.refresh()`).

---

## ✨ Funcionalidades da Interface

### 🏆 Desafios
- Abas **Disponíveis** / **Meus desafios**, filtro por status (aberto, em andamento, em votação, encerrado, cancelado),
  busca, faixa de entrada, ordenação (inclui proximidade) e paginação ("Carregar mais").
- **Criar** com valor de entrada: o criador paga a entrada na hora (a tela mostra o débito e o saldo antes de confirmar).
- **Entrar** (confirmação com débito), **sair** e **cancelar** (reembolso), **iniciar** (mín. 2 participantes),
  **convidar amigos**.
- **Check-in** (1/dia, só participantes, só em andamento): texto, foto (JPG/PNG/WEBP até 3 MB, via `FormData`) e localização opcional.
- **Abrir votação**, **votar** (não em si mesmo, voto único), **apurar** (criador) e **resultado** com prêmio e empate.
- Links diretos: `/desafios?id=N` (usado pelas notificações) e `/desafios?criar=1`.

### 🤝 Amigos
- Busca por nome (mín. 2 letras; sem e-mail/CPF de terceiros), aceitar/recusar/cancelar/remover.
- **Ranking entre amigos** (aba): pontos = exercícios + 3 × check-ins no período (7/30/90/365 dias).

### 💰 Carteira e depósito
- Saldo (com ocultação LGPD), **extrato paginado** e **saque PIX** com acompanhamento por polling (5 s) e reembolso automático em falha.
- Depósito via PIX com QR Code, Copia-e-Cola, polling e, em sandbox, botão de simulação.

### 🔔 Notificações
- Sino ("Avisos") no menu inferior de todas as páginas logadas, contador por polling a cada 30 s, lista, marcar uma/todas como lida.

### 💪 Treinos
- Criar treinos de um catálogo por grupo muscular, **compartilhar/importar** por código, **modo guiado** e **radar** SVG.

### 🔒 Privacidade (LGPD)
- Banner de consentimento e mascaramento de dados sensíveis por padrão.

---

## ▶️ Como Rodar

O frontend é servido pelo backend Go. Basta subir o backend (veja `../gymScore-backend/README.md`) e acessar:

```
http://localhost:3000
```

O caminho da pasta `public/` é configurável no backend via `FRONTEND_PATH` (`.env`).

> Após alterar HTML/CSS/JS, faça um **hard refresh** no navegador (Ctrl+Shift+R) para evitar cache.
