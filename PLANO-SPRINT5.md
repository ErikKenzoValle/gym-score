# PLANO — GymScore Sprint 5 (Figma pixel-perfect)
Data: 2026-09-19
Diretório: /cod/gym-score

## Objetivo
Redesenhar o frontend completo com fidelidade ao Figma v7 e criar as telas e endpoints
que ainda não existem. Backend serve HTML estático via Fiber; zero build step.

## Gap vs. estado atual

### Telas existentes que precisam de redesign
- login.html — funcional mas sem visual Figma (fontes condensadas, cor por perfil, footer Asaas)
- criar-conta.html — HTML antigo sem design system
- menu-principal.html — falta elo badge, chips de categoria, cards de desafio com imagem/barra
- desafios.html / desafios-detalhado.html — falta imagem de capa, distribuição de prêmios, botão GARANTIR VAGA
- depositar.html — falta toggle Depositar/Sacar no mesmo form
- amigos.html — falta seção "X solicitações pendentes", badges remodelados
- perfil.html — falta card Elo/XP, Escala de Elos, share no histórico de participações
- academia-dashboard.html — falta grid KPI 2x2, lista Próximos Eventos com barra de progresso
- academia-podio.html — falta fluxo de seleção de presentes, slots numerados

### Telas novas (não existem)
- cadastro-academia.html — cadastro de instituição (CNPJ, endereço, cor laranja)
- ingressos.html — Meus Ingressos com QR code client-side
- academia-criar-desafio.html — form completo com instrutor checklist, categorias, elo mínimo
- academia-instrutores.html — código da instituição + lista de instrutores vinculados
- instrutor-agenda.html — agenda de eventos do instrutor
- instrutor-checkin.html — check-in com lista de atletas e status presente/registrado
- instrutor-podio.html — lançamento de pódio pelo instrutor (só presentes)

### CSS novas
- cadastro-academia.css
- ingressos.css
- academia-criar-desafio.css (ou ampliar academia.css)
- instrutor.css — design system azul para as 3 telas do instrutor

### Novos endpoints de backend necessários
- GET /api/usuarios/ingressos — participações confirmadas do atleta (para ingressos + QR)
- GET /api/instrutor/agenda — desafios onde o instrutor está vinculado
- HTML routes: /signup-academia, /ingressos, /academia/criar-desafio, /academia/instrutores,
  /instrutor, /instrutor/checkin, /instrutor/podio

## Arquitetura de cores por perfil
Usar atributo `data-perfil` no `<body>` + CSS variables por perfil:
```css
[data-perfil="atleta"]   { --accent: #22c55e; --accent-dark: #16a34a; --accent-glow: rgba(34,197,94,0.18); }
[data-perfil="academia"] { --accent: #f97316; --accent-dark: #ea580c; --accent-glow: rgba(249,115,22,0.18); }
[data-perfil="instrutor"]{ --accent: #3b9ce0; --accent-dark: #2563eb; --accent-glow: rgba(59,156,224,0.18); }
```
Renomear `--primary` → `--accent` nas telas autenticadas. Login usa verde fixo.

## Tipografia
Google Fonts: Bebas Neue (títulos condensados) + sistema (corpo).
`<link rel="preconnect">` + `<link href="https://fonts.googleapis.com/css2?family=Bebas+Neue&display=swap">`

## DEV_BACKEND (executar primeiro)

### Novos endpoints
- [x] GET /api/usuarios/ingressos — listar participações do usuário logado (status qualquer),
      join com desafios, retornar id_desafio, titulo, data, local, status, codigo_ingresso
      codigo_ingresso = "AC-{IDDesafio:04X}-{IDUsuario:03d}"
- [x] GET /api/instrutor/agenda — buscar instrutores.id_usuario = user_id, retornar desafios
      associados via id_academia (MVP: retornar desafios abertos criados pela mesma academia)

### Novos modelos de resposta
- [x] IngressoResponse struct em models/ingresso.go: IDDesafio, Titulo, Data, Local, Status, CodigoIngresso

### Novas rotas HTML
- [x] /signup-academia → cadastro-academia.html
- [x] /ingressos → ingressos.html
- [x] /academia/criar-desafio → academia-criar-desafio.html
- [x] /academia/instrutores → academia-instrutores.html
- [x] /instrutor → instrutor-agenda.html
- [x] /instrutor/checkin → instrutor-checkin.html
- [x] /instrutor/podio → instrutor-podio.html

### api.js — novos métodos
- [x] API.ingressos() → GET /api/usuarios/ingressos
- [x] API.agendaInstrutor() → GET /api/instrutor/agenda
- [x] API.listarInstrutoresAcademia() → GET /api/instrutores (já existe)
- [x] API.criarDesafio(payload) → POST /api/desafios (já existe — confirmado)

## DEV_FRONTEND (executar após backend)

### 1. base.css — design system
- [x] Adicionar variáveis `--accent` / `--accent-dark` / `--accent-glow` por data-perfil
- [x] Manter `--primary` como alias para compatibilidade
- [x] Adicionar import Bebas Neue
- [x] Adicionar classe `.titulo-bebas` (font-family: 'Bebas Neue'; letter-spacing: 0.05em)
- [x] Bottom nav: usar `--accent` na cor ativa (não `--primary`)
- [x] Adicionar helper `setPerfilAtual(perfil)` em api.js que seta `document.body.dataset.perfil`

### 2. login.html — redesign pixel-perfect
- [x] Fundo #0a0a0f com gradiente esverdeado no topo (radial-gradient verde 20% opacidade)
- [x] Logo "GYM SCORE" em Bebas Neue: "GYM" verde, "SCORE" branco
- [x] Subtítulo "Plataforma de desafios esportivos"
- [x] Abas Atleta/Academia/Instrutor — mudam cor de destaque da tela inteira via data-perfil
- [x] Campo "E-mail ou CPF" (academia: E-mail ou CNPJ) com ícone colar
- [x] Campo Senha com ícone olho
- [x] Instrutor: campo extra "Código da Instituição" + sem botão CADASTRAR
- [x] Botão "ENTRAR" preenchido na cor do perfil
- [x] Dois botões lado a lado: "CADASTRAR" (→ /signup ou /signup-academia) e "RECUPERAR SENHA"
- [x] Footer fixo "Powered by Asaas Gateway • v2.0.1"

### 3. criar-conta.html — Atleta (redesign)
- [x] Header "← VOLTAR" (verde) + título "CADASTRO / DE ATLETA" Bebas Neue
- [x] Subtítulo "Crie sua conta e comece a competir"
- [x] Card "DADOS PESSOAIS": Nome, E-mail, CPF, Data nasc, Gênero (pills M/F/O), Senha
- [x] Card destaque verde "ELO INICIAL: INICIANTE — Sua carteira digital será criada automaticamente..."
- [x] Botão "CRIAR CONTA E COMPETIR"

### 4. cadastro-academia.html — NOVA
- [x] Mesma estrutura que atleta, cor laranja, título "CADASTRO / INSTITUIÇÃO"
- [x] Seção "DADOS DA EMPRESA": Razão Social, Nome Fantasia, CNPJ
- [x] Seção "CONTATO E LOCALIZAÇÃO": E-mail corp, Telefone, Endereço
- [x] Seção "ACESSO": Senha
- [x] Botão "CADASTRAR INSTITUIÇÃO"
- [x] Chama API.criarUsuario com perfil='academia'

### 5. menu-principal.html — Feed redesign
- [x] Header "OLÁ, MARIA" + saldo em destaque + avatar com inicial
- [x] Badge elo "PRATA 210/1000 XP" (busca de perfil)
- [x] Barra de busca "Buscar desafios..."
- [x] Chips de categoria horizontais (Todos / Musculação / Corrida / CrossFit / Natação)
- [x] Seção IA Recomendados (carrossel, já existe — manter)
- [x] Lista desafios abertos: cards com imagem fundo, badges, prêmio, vagas+barra progresso

### 6. desafios-detalhado.html — redesign
- [x] Imagem de capa com gradiente overlay + botão voltar + badge categoria
- [x] Título grande (Bebas Neue)
- [x] Cards: Data/Vagas restantes side-by-side
- [x] Card "LOCAL DO EVENTO"
- [x] Card "REGRAS TÉCNICAS" com bullets
- [x] Card "DISTRIBUIÇÃO DE PRÊMIOS" — 1º/2º/3º/4º com valores
- [x] Rodapé fixo: taxa, elo mínimo, botão "GARANTIR VAGA →"

### 7. ingressos.html — NOVA
- [x] Título "MEUS INGRESSOS" + subtítulo "Apresente o QR Code no check-in"
- [x] Cards por evento: badge CONFIRMADO, data, local
- [x] Area colapsável "Toque para exibir o QR Code" → gera QR via qrcode.js (CDN)
- [x] Código alfanumérico ex: AC-8X7K2-001
- [x] Usar API.ingressos()

### 8. depositar.html — Carteira redesign
- [x] Card grande verde "SALDO DISPONÍVEL" com valor
- [x] Toggle pills "Depositar" / "Sacar" (já tem algo parecido — expandir)
- [x] Form Depositar: chips R$25/50/100/200, campo manual, "GERAR QR CODE PIX"
- [x] Form Sacar: chave Pix, valor, "SOLICITAR SAQUE PIX"
- [x] Link "SOLICITAR EXTRATO" no rodapé

### 9. amigos.html — redesign
- [x] Título "AMIGOS" + "5 conexões ativas"
- [x] Busca por nome/usuário
- [x] Card destaque "2 solicitações pendentes" + botão "VER" (se houver pendentes)
- [x] Lista: avatar colorido, nome, @usuário, elo colorido, contagem troféus, botão ADICIONAR

### 10. perfil.html — redesign
- [x] Avatar grande, nome, "Atleta desde {mês ano}"
- [x] Card "ELO/PATENTE": medalha, nome elo, XP/pontos, barra de progresso
- [x] 3 mini-cards: Pódios, Desafios, Ganhos
- [x] Card "ESCALA DE ELOS" (Iniciante→Prata→Ouro→Diamante→Elite, com "VOCÊ ESTÁ AQUI")
- [x] Lista "HISTÓRICO DE PARTICIPAÇÕES" com ícone share por item
- [x] Modal share (já existe — verificar se compatível com novo design)
- [x] Botão outline vermelho "SAIR DA CONTA"

### 11. academia-dashboard.html — redesign
- [x] "BEM-VINDA / Iron House" + ícone academia
- [x] Grid 2x2 KPIs: Eventos Ativos, Inscritos no Mês, Faturamento, Avaliação Média
- [x] Botões "+ CRIAR DESAFIO" → /academia/criar-desafio e "INSTRUTORES" → /academia/instrutores
- [x] Lista "PRÓXIMOS EVENTOS": badge ABERTO, inscritos x/y, barra progresso, botão "GERENCIAR"

### 12. academia-criar-desafio.html — NOVA
- [x] Título "CRIAR DESAFIO" Bebas Neue, cor laranja
- [x] Seção "INFORMAÇÕES DO EVENTO": Título, Descrição/Regras, Categoria chips
- [x] Seção "DATA E LOCAL": Data, Hora, Endereço
- [x] Seção "VAGAS E RESTRIÇÕES": Limite vagas, Restrição Elo chips
- [x] Seção "INSTRUTOR(ES) RESPONSÁVEL(IS)": checklist com nome+CPF, validação ≥1
- [x] Seção "FINANCEIRO": Taxa inscrição, Prêmio total
- [x] Botão "PUBLICAR DESAFIO"
- [x] Chama API existente POST /api/desafios

### 13. academia-instrutores.html — NOVA
- [x] Título "GESTÃO DE INSTRUTORES"
- [x] Card laranja "CÓDIGO DA INSTITUIÇÃO: GS-IH-XXXX" + botão "COPIAR"
  (código gerado no frontend: "GS-" + base36(userID) em maiúsculas, 6 chars)
- [x] Botão "+ CADASTRAR NOVO INSTRUTOR" → /cadastro-instrutor
- [x] Lista "INSTRUTORES VINCULADOS": avatar, nome, CPF, badge ATIVO
- [x] Chama API.listarInstrutoresAcademia()

### 14. academia-podio.html — redesign (Auditoria)
- [x] Título "PÓDIO / TOP 5" + nome e data do evento (via ?id= param)
- [x] Aviso informativo laranja
- [x] Lista dos inscritos presentes — cada um com botão "SELECIONAR" para slot
- [x] 5 slots de posição (1º a 5º) com nome selecionado
- [x] Botão "CONFIRMAR PÓDIO (X/5 SELECIONADOS)" — habilita quando todos 5 preenchidos

### 15. instrutor-agenda.html — NOVA (perfil azul)
- [x] `document.body.dataset.perfil = 'instrutor'`
- [x] "BEM-VINDO / Carlos Souza" + badge "INSTRUTOR • Iron House"
- [x] "MINHA AGENDA — N EVENTOS"
- [x] Cards: badge EM ANDAMENTO/AGUARDANDO, data, local, link "GERENCIAR →"
- [x] Bottom nav: Agenda / Check-in / Pódio
- [x] Chama API.agendaInstrutor()

### 16. instrutor-checkin.html — NOVA
- [x] Título do evento (via ?id= param)
- [x] 3 KPIs: Inscritos, Presentes, Aguardando
- [x] Botão "ESCANEAR QR CODE" (abre câmera via jsQR/BarcodeDetector API)
- [x] Lista de atletas: bolinha verde/vermelha, número, nome, elo, badge PRESENTE/REGISTRADO
- [x] Botão manual de marcar presente por linha
- [x] Botão "IR PARA LANÇAMENTO DO PÓDIO →" → /instrutor/podio?id=X
- [x] Usa API.listarPresentes() e API.marcarPresenca()

### 17. instrutor-podio.html — NOVA
- [x] Título "LANÇAMENTO DO PÓDIO"
- [x] Aviso azul "Apenas atletas com status Presente podem ser selecionados..."
- [x] 5 blocos (1º a 5º): ícone medalha + select "— Selecionar atleta presente —"
- [x] Botão desabilitado "PREENCHA TODOS OS 5 LUGARES" → habilita quando completo
- [x] Chama API.listarPresentes() e API.lancarPodio()
- [x] Bottom nav azul: Agenda / Check-in / Pódio

## QA (após frontend)
- [x] Verificar rotas novas acessíveis (status 200) — rotas HTML registradas e arquivos existem
- [x] Testar login com cada perfil e validar roteamento — código correto (atleta→/menu, academia→/academia, instrutor→/instrutor)
- [⚠] Testar criação de desafio pela academia — BUG CRÍTICO: GET /api/instrutores não registrado (lista de instrutores não carrega)
- [x] Testar fluxo ingressos (lista + QR exibido) — endpoint protegido, modelo correto, QR via qrcode.js
- [x] Testar check-in do instrutor + pódio — telas completas, API calls corretos, 5 selects, botão desabilitado
- [ ] Validar responsividade mobile (390px viewport) — não testado em ambiente de execução
- [x] go test ./... ainda passa (sem regressão backend) — 8 testes passando, 2 pacotes
