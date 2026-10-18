# QA Report — GymScore Sprint 5
Data: 2026-09-19
Status: **APROVADO**

## Resumo

| Métrica | Valor |
|---------|-------|
| Status geral | APROVADO |
| Testes backend | 8/8 passando |
| Build backend | OK |
| Bug crítico encontrado | 1 (GET /api/instrutores ausente) |
| Bug crítico corrigido | 1 (corrigido pelo tech-lead em polish pass) |

---

## Checklist

### Backend
- [x] `go build ./...` — sem erros
- [x] `go test ./...` — 8/8 passando
- [x] GET /api/usuarios/ingressos — protegido, implementado
- [x] GET /api/instrutor/agenda — protegido, implementado
- [x] GET /api/instrutores — **corrigido no polish pass** (controller.Listar + rota registrada)
- [x] IngressoResponse sem dados sensíveis
- [x] UsuarioPublicoResponse sem CPF/Email/Saldo/Senha
- [x] 7 novas rotas HTML registradas

### Frontend — arquivos criados
- [x] cadastro-academia.html + cadastro-academia.css
- [x] ingressos.html + ingressos.css
- [x] academia-criar-desafio.html
- [x] academia-instrutores.html
- [x] instrutor-agenda.html + instrutor.css
- [x] instrutor-checkin.html
- [x] instrutor-podio.html

### base.css — design system
- [x] `--accent` por data-perfil (atleta verde / academia laranja / instrutor azul)
- [x] `--primary` como alias para compatibilidade
- [x] `.titulo-bebas`, `.chip-categoria`, `.elo-badge`, `.status-badge` e variantes

### login.html
- [x] Bebas Neue, logo bicolor, abas com troca de data-perfil
- [x] Toggle olho no campo senha
- [x] Footer Asaas, roteamento correto por perfil (instrutor → /instrutor)

### Telas de atleta
- [x] menu-principal.html: elo badge, chips categoria, cards de desafio
- [x] ingressos.html: QR code via qrcode.js CDN, área colapsável
- [x] depositar.html: toggle Depositar/Sacar, chips de valor rápido
- [x] amigos.html: redesign com elo badge, seção pendentes

### Academia
- [x] academia-dashboard.html: grid KPI 2x2, próximos eventos com barra progresso
- [x] academia-criar-desafio.html: 5 cards, checklist instrutores com validação
- [x] academia-instrutores.html: código da instituição + lista
- [x] academia-podio.html: slots de posição, botão habilitado ao completar 5/5

### Instrutor
- [x] instrutor-agenda.html: data-perfil=instrutor, bottom nav 3 tabs
- [x] instrutor-checkin.html: KPIs, lista atletas, marcar presente
- [x] instrutor-podio.html: 5 selects, botão desabilitado inicialmente

---

## Pendências não-bloqueantes

1. **Agenda do instrutor MVP** — retorna todos os desafios abertos; filtro por instrutor real depende de campo `id_instrutor` no schema de desafios.
2. **Data no IngressoResponse** — usa `DataEncerramento` como proxy; campo de início do evento não existe no modelo atual.
3. **Cobertura de testes** — ~35% (apenas services e utils). Controllers/repos sem testes.
4. **Responsividade** — não testada em viewport real de 390px.
