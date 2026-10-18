# QA-REPORT-FULL — Rodada de correção e QA completa
Data: 2026-09-27
Responsável: tech-lead (orquestração de dev-db, dev-backend, dev-frontend) + QA manual em navegador real (Claude in Chrome)

## Escopo
Fazer todas as telas do GymScore funcionarem corretamente (especialmente perfil:
ver dados do usuário e logout), garantir a IA de recomendação funcionando (modo
heurístico, sem ANTHROPIC_API_KEY por decisão do usuário) e validar tudo com QA
real em navegador antes da entrega.

## Build / testes automatizados
- `go build ./...` — OK
- `go vet ./...` — OK
- `go test ./...` — OK, 100% passando (internal/client, internal/services, pkg/utils)

## Bugs encontrados e corrigidos nesta rodada

### 1. [CRÍTICO] Tela principal (menu) travava ao carregar
`menu-principal.html` chamava `renderHeader()` antes de `_ELO_MAP` ser
inicializado (ordem de execução do script). Isso lançava uma exceção síncrona
que interrompia todo o resto do script — desafios, recomendações IA e saldo
nunca carregavam, a tela ficava presa em skeleton. Corrigido reordenando as
declarações antes da primeira chamada.

### 2. [CRÍTICO] Mensagens de erro/status invisíveis em ~12 telas, incluindo o login
Um elemento `<p class="gs-msg" style="display:none">` tem prioridade de CSS
inline sobre a classe `.show` que deveria exibi-lo — então toda vez que o
JavaScript tentava mostrar uma mensagem de erro trocando só a `className`, a
mensagem ficava no DOM mas nunca visível. Reproduzido ao vivo: erro de PIX
(chave Asaas inválida) e erro de login com perfil errado ficavam
completamente silenciosos para o usuário — o app "não fazia nada" visualmente.
Corrigido em duas frentes:
- Removido o `display:none` inline redundante do HTML em ~10 elementos
  (`academia-dashboard.html`, `academia-instrutores.html`, `academia-podio.html`,
  `instrutor-agenda.html`, `instrutor-checkin.html`, `instrutor-podio.html`,
  `depositar.html`, `ingressos.html`).
- Corrigida uma variante do mesmo bug em `login.html` e outros arquivos, onde o
  próprio JS reintroduzia o bloqueio via `elemento.style.display = 'none'` no
  caminho de sucesso — trocado por `classList.remove('show')` para nunca deixar
  inline style residual.

### 3. Escala de Elo divergente entre backend e frontend
O backend calculava Elo com uma escala antiga de 5 níveis (Bronze→Elite,
default "Bronze") enquanto o frontend (perfil.html) já usava uma escala nova
de 6 níveis (Iniciante→Elite). Resultado: o mesmo usuário com 0 XP aparecia
como "BRONZE" no menu e "INICIANTE" no perfil. Unificado: backend agora usa a
escala de 6 níveis e todas as respostas da API (`UsuarioResponse`,
`AmigoResponse`, perfil da recomendação IA) derivam o Elo a partir dos pontos
atuais em vez de um valor potencialmente desatualizado gravado no banco.

### 4. Cadastro de academia 100% quebrado
`cadastro-academia.html` enviava CNPJ no campo de CPF (formato incompatível),
não enviava data de nascimento/gênero (obrigatórios no DTO de atleta) e não
existia contrato de backend para perfil "academia" — era impossível criar uma
conta de academia. Implementado de ponta a ponta: model `Academia`,
`POST /api/academias` (endpoint público dedicado), `Usuario.CPF`/`DataNascimento`/
`Genero` tornados opcionais no schema (nullable) para contas de pessoa
jurídica, e a tela religada no novo contrato. Testado no navegador: cadastro,
login como academia, dashboard e gestão de instrutores funcionando.

### 5. Botão "GARANTIR VAGA" inacessível em desafios-detalhado
A tela chamava `renderBottomNav()` além de já ter seu próprio rodapé fixo com
o botão de inscrição — a bottom nav padrão (`z-index:100`) cobria o rodapé de
ação (`z-index:50`), tornando o botão de inscrição clicável só via DOM, nunca
visualmente. Corrigido removendo a bottom nav dessa tela (ela já tem seu
próprio rodapé de navegação/ação). Testado: inscrição em desafio funcionando
de ponta a ponta ("Vaga garantida! Boa sorte!").

### 6. Sessão expirada não desloga o usuário
Nenhuma chamada de API tratava `401` de forma especial — um token expirado
deixava a tela presa em estado quebrado (múltiplos toasts de erro, dados
desatualizados) sem nunca mandar o usuário para o login. Adicionado
tratamento global em `api.js`: qualquer `401` numa rota autenticada agora
limpa a sessão e redireciona para `/login` automaticamente.

### 7. `AmigoResponse` sem elo/pontos
A lista de busca de amigos nunca mostrava o Elo real de ninguém (sempre
"Iniciante" por fallback do frontend). Adicionados os campos `elo` e `pontos`
ao `AmigoResponse` no backend.

### 8. [CRÍTICO] Contas de academia e instrutor não tinham como ver perfil nem fazer logout
`academia-dashboard.html`, `academia-instrutores.html`, `instrutor-agenda.html`
e as demais telas desses dois perfis não tinham nenhum botão de logout nem
link para uma tela de perfil — o item "Perfil" simplesmente não existia na
navegação inferior desses dois perfis (só existia para atleta). Corrigido:
- Adicionado item "Perfil" na navegação de academia e instrutor (`api.js`).
- `perfil.html` adaptada para funcionar com os três perfis: esconde as
  seções que só fazem sentido para atleta (Elo/XP, pódios/desafios/ganhos,
  histórico de participações) quando o usuário é academia ou instrutor,
  mantendo hero, Editar Perfil, Alterar Senha e Sair da Conta para todos.
- `alterar-perfil.html`: o campo "Gênero" (obrigatório no formulário) não se
  aplica a academia/instrutor — agora fica oculto e não é mais enviado pra
  esses perfis (antes seria gravado silenciosamente como "Masculino" por
  padrão do `<select>`).
Testado ao vivo: perfil, edição e logout funcionando para os três tipos de
conta (atleta, academia, instrutor).

### 9. Mensagens de erro genéricas mascarando o erro real
`usuario_controller.go` (Ingressos) e `pix_controller.go` (ConsultarPagamento)
retornavam mensagens fixas em vez do erro real da camada de serviço.
Corrigido para propagar `err.Error()`.

## Limitações conhecidas — não corrigidas nesta rodada (fora do escopo de "bug", são gaps de produto/config)

- **IA de recomendação em modo heurístico**: por decisão do usuário, sem
  `ANTHROPIC_API_KEY` configurada. O fallback é parte do design original e
  funciona 100% (razões heurísticas tipo "desafio recente"). Basta configurar
  a chave no `.env` quando disponível.
- **PIX real bloqueado**: `ASAAS_API_KEY_SANDBOX` no `.env` está inválida/
  expirada ("A chave de API fornecida é inválida" — erro real do Asaas, agora
  visível corretamente na tela). Não é um bug de código; precisa de uma chave
  sandbox válida do painel Asaas para testar geração de PIX de ponta a ponta.
- **Dashboard de academia**: "Faturamento" e "Inscritos no mês" sempre mostram
  zero — o backend não calcula esses dois campos (`GET /api/academia/dashboard`
  só retorna `avaliacao_media`, `desafios_encerrados`, `total_desafios`). Não
  quebra a tela (degrada graciosamente para zero), mas é uma feature
  incompleta. "Eventos ativos" funciona (calculado no frontend).
  **Recomendação de próximos passos** — se estender: `total_inscritos_mes`
  (contar participantes de desafios da academia com inscrição no mês corrente)
  e `faturamento` (somar taxas dos desafios da academia no período).
- **Agenda do instrutor**: mostra todos os desafios abertos do sistema, não
  filtrados por academia/instrutor — limitação já documentada em sprint
  anterior (schema não tem vínculo direto desafio↔instrutor).
- **Filtro de categoria no feed**: `Desafio` não tem campo `categoria` nem
  `elo_minimo` no backend — os chips de categoria na tela principal nunca
  filtram de fato (sempre mostram "Todos"). Gap de produto pré-existente, não
  investigado a fundo nesta rodada por exigir mudança de schema.
- **`gymScore-frontend/public/html/desafios.html`**: arquivo órfão do
  pré-redesign, não roteado por nenhuma URL (a rota `/desafios` serve
  `desafios-detalhado.html`). Inofensivo, mas pode ser removido em uma limpeza
  futura.
- Data zero-value (`31 de dezembro de 1`) aparece em `ingressos.html` quando o
  desafio associado não tem data de evento definida — cosmético, não quebra o
  fluxo.
- Cache de navegador: como as páginas HTML são servidas como estático puro,
  correções neste tipo de arquivo podem exigir hard-refresh do usuário em
  produção (sem cache-busting de asset configurado).

## Checklist manual — telas testadas ao vivo no navegador

| Tela | Perfil | Status |
|---|---|---|
| Cadastro de atleta | público | OK |
| Login (3 abas, erro de perfil errado) | público | OK (corrigido) |
| Menu principal / feed | atleta | OK (corrigido) |
| Perfil (dados, elo, histórico, logout) | atleta | OK |
| Editar perfil | atleta | OK |
| Amigos (busca, adicionar) | atleta | OK |
| Detalhe de desafio + inscrição | atleta | OK (corrigido) |
| Carteira / depositar (PIX) | atleta | OK — erro real visível (bloqueado por chave Asaas inválida, não é bug) |
| Extrato | atleta | OK |
| Treinos | atleta | OK |
| Ingressos | atleta | OK |
| Cadastro de instrutor | atleta→instrutor | OK |
| Agenda do instrutor | instrutor | OK |
| Check-in do instrutor | instrutor | OK |
| Cadastro de academia | público | OK (corrigido, antes 100% quebrado) |
| Dashboard de academia | academia | OK (2 KPIs sempre zero, ver limitações) |
| Gestão de instrutores (academia) | academia | OK |
| Perfil + editar perfil + logout (academia) | academia | OK (corrigido, antes inexistente) |
| Perfil + editar perfil + logout (instrutor) | instrutor | OK (corrigido, antes inexistente) |

## Conclusão
Todas as telas testadas funcionam de ponta a ponta. Os bugs críticos que
impediam uso real (menu travado, mensagens de erro invisíveis, cadastro de
academia quebrado, botão de inscrição inacessível, sessão expirada sem
logout) foram corrigidos e verificados ao vivo no navegador. As limitações
remanescentes são gaps de produto/configuração, não bugs, e estão listadas
acima para priorização futura.
