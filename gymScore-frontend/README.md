# GymScore — Frontend

Interface web do **GymScore**, uma plataforma que incentiva a prática de exercícios físicos por meio de desafios com prêmios, sistema de amigos, treinos compartilháveis e depósitos via PIX.

Construído em **HTML, CSS e JavaScript puro** (sem framework/build), servido como estático pelo backend Go.

---

## 🧩 Stack

- HTML5 + CSS3 (tema escuro, mobile-first)
- JavaScript puro (sem dependências externas)
- Comunicação com a API via `fetch` (JWT no `localStorage`)
- Radar de treinos desenhado em **SVG puro**, sem bibliotecas

> Não há etapa de build. Os arquivos em `public/` são servidos diretamente.

---

## 📁 Estrutura

```
gymScore-frontend/
└── public/
    ├── index.html          # Redireciona para /login ou /menu conforme o token
    ├── css/                # Estilos por página + base.css (variáveis de tema)
    ├── img/                # Imagens
    ├── js/
    │   └── api.js          # Auth, helpers e todas as chamadas à API
    └── html/
        ├── login.html / criar-conta.html / esqueci-senha.html
        ├── menu-principal.html
        ├── desafios-detalhado.html   # Lista, filtros, criação e execução de desafios
        ├── amigos.html               # Amigos + solicitações pendentes
        ├── treinos.html              # Treinos, modo guiado e radar muscular
        ├── depositar.html            # Depósito via PIX
        ├── perfil.html / alterar-perfil.html / alterar-senha.html
        └── privacidade.html          # Política de privacidade (LGPD)
```

---

## ✨ Funcionalidades da Interface

### 🏆 Desafios
- Criação com prêmio, vagas e (opcional) localização para busca por proximidade.
- Filtros: por **status** (Ativos / Abertos / Pendentes / Em andamento / 📜 Histórico), por **faixa de valor** (mín–máx) e **ordenação** (recentes, menor/maior prêmio, mais próximos).
- Entrar em desafios com vagas; o criador encerra escolhendo o vencedor.
- Desafios lotados somem da lista para quem não participa.

### 🤝 Amigos
- Buscar e adicionar usuários.
- Seções de **solicitações recebidas** (aceitar/recusar), **enviadas** (aguardando) e **amigos**.

### 💪 Treinos
- Criar múltiplos treinos escolhendo exercícios de um catálogo por grupo muscular.
- **Compartilhar** por código e **importar** treinos de outras pessoas.
- **Modo guiado**: um exercício por vez, com barra de progresso.
- **Radar** (gráfico de teia) mostrando o equilíbrio do treino entre os grupos musculares.

### 💸 Depósito (PIX)
- Geração de QR Code + Copia-e-Cola.
- Confirmação automática por polling.
- Em ambiente de testes, botão para **simular o pagamento**.

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
