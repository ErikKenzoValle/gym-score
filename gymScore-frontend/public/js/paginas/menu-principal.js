// Script da página menu-principal.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
if (!Auth.require()) throw new Error('unauthenticated');
checkLGPDConsent();

var LIMITE = 10;
var user = Auth.getUser() || {};
var catAtiva = '';
var pagina = 1;
var total = 0;
var qtd = 0;
var seq = 0;
var buscaTimer = null;
var categorias = ['geral', 'corrida', 'musculacao', 'crossfit', 'funcional', 'ciclismo', 'natacao', 'calistenia', 'luta', 'outros'];
var $ = function(id) { return document.getElementById(id); };

if (user.perfil) document.body.dataset.perfil = Modo.atual(user);
// O feed é a home do modo atleta: no modo instrutor (ou conta de academia) vai para a home do modo
function _homeDoModo(u) {
    var m = Modo.atual(u);
    if (m !== 'atleta') { window.location.replace(Modo.home(m)); return true; }
    return false;
}
if (!_homeDoModo(user)) (function () { // resto da página só no modo atleta

// ── Cabeçalho ─────────────────────────────────────────────────────────────
var _ELO_MAP = { iniciante: 'elo-iniciante', bronze: 'elo-bronze', prata: 'elo-prata', ouro: 'elo-ouro', diamante: 'elo-diamante', elite: 'elo-elite' };

function renderHeader() {
    $('feedNome').textContent = (user.nome || 'Atleta').toUpperCase();
    $('feedAvatar').textContent = (user.nome || 'A').charAt(0).toUpperCase();
    renderSaldo();
    var elo = (user.elo || 'iniciante').toLowerCase();
    var badge = $('feedEloBadge');
    badge.className = 'elo-badge ' + (_ELO_MAP[elo] || 'elo-iniciante');
    badge.textContent = elo.toUpperCase() + ' • ' + (user.pontos || 0) + ' XP';
}

function renderSaldo() {
    var oculto = Privacy.isHidden();
    $('feedSaldo').textContent = oculto ? 'R$ •••' : fmtBRL(user.saldo_centavos);
    $('btnOlho').setAttribute('aria-pressed', oculto ? 'false' : 'true');
    var icone = $('saldoEyeIcon');
    icone.innerHTML = oculto // SVG estático, sem dado de usuário
        ? '<path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94"/><path d="M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19"/><line x1="1" y1="1" x2="23" y2="23"/>'
        : '<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/>';
}
$('btnOlho').addEventListener('click', function() { Privacy.toggle(); });
document.addEventListener('privacyChange', renderSaldo);

renderHeader();
Auth.refresh().then(function(fresh) {
    if (fresh) { if (_homeDoModo(fresh)) return; user = fresh; renderHeader(); verificarOnboarding(); }
});

// ── Onboarding do primeiro acesso (saldo zerado e sem desafios) ──────────
async function verificarOnboarding() {
    var box = $('onboarding');
    try {
        var res = await Promise.all([API.listarMeusDesafios({ limite: 1 }), API.listarAmigos()]);
        var temDesafio = ((res[0].data || {}).total || 0) > 0;
        var temAmigo = (res[1].data || []).some(function(a) { return a.status === 'aceita'; });
        var temSaldo = (user.saldo_centavos || 0) > 0;
        if (temDesafio) { box.hidden = true; return; }
        var passos = [
            { feito: temSaldo, texto: 'Deposite saldo via PIX', href: '/depositar', link: 'Depositar' },
            { feito: temAmigo, texto: 'Adicione um amigo', href: '/amigos', link: 'Buscar amigos' },
            { feito: false, texto: 'Entre num desafio abaixo', href: '#desafiosTitulo', link: 'Ver desafios' }
        ];
        limpar(box).appendChild(h('h2', { id: 'onbTitulo', text: 'Comece por aqui' }));
        box.appendChild(h('p', { text: 'Três passos para entrar no seu primeiro desafio.' }));
        box.appendChild(h('ol', null, passos.map(function(p, i) {
            return h('li', { class: p.feito ? 'feito' : null }, [
                h('span', { class: 'passo', 'aria-hidden': 'true', text: p.feito ? '✓' : String(i + 1) }),
                h('span', { text: p.texto + (p.feito ? ' (feito)' : '') }),
                p.feito ? null : h('a', { href: p.href, text: p.link })
            ]);
        })));
        box.hidden = false;
    } catch (e) { box.hidden = true; }
}
verificarOnboarding();

// ── Categorias e busca (filtros no servidor) ──────────────────────────────
function renderChips() {
    var cont = limpar($('chips'));
    function chip(slug, rotulo) {
        return h('button', { type: 'button', class: 'chip-categoria' + (catAtiva === slug ? ' ativo' : ''), 'aria-pressed': catAtiva === slug ? 'true' : 'false',
            text: rotulo, onClick: function() { catAtiva = slug; renderChips(); carregar(true); } });
    }
    cont.appendChild(chip('', 'Todas'));
    categorias.forEach(function(c) { cont.appendChild(chip(c, rotuloCategoria(c))); });
}
API.categoriasDesafio().then(function(r) { if (r.data && r.data.length) { categorias = r.data; renderChips(); } }).catch(function() {});

$('buscaInput').addEventListener('input', function() {
    clearTimeout(buscaTimer);
    buscaTimer = setTimeout(function() { carregar(true); }, 350);
});
$('btnMais').addEventListener('click', function() { carregar(false); });

// ── Lista de desafios abertos (GET /desafios: envelope {itens,total,pagina,limite}) ──
async function carregar(reiniciar) {
    var mine = ++seq;
    var q = $('buscaInput').value.trim();
    if (reiniciar) {
        pagina = 1; qtd = 0;
        limpar($('feedEstado'));
        Skeleton.cards($('desafiosLista'), 3, 150);
        $('listaMais').hidden = true;
    } else { pagina++; }
    try {
        var resp = await API.listarDesafios({ status: 'aberto', categoria: catAtiva, q: q, pagina: pagina, limite: LIMITE });
        if (mine !== seq) return;
        var dados = resp.data || {};
        var itens = dados.itens || [];
        total = dados.total || 0;
        var lista = $('desafiosLista');
        if (reiniciar) limpar(lista);
        lista.removeAttribute('aria-busy');
        itens.forEach(function(d) { lista.appendChild(cartaoDesafio(d)); });
        qtd += itens.length;
        $('listaMais').hidden = qtd >= total;
        $('desafiosTitulo').textContent = total + (total === 1 ? ' desafio aberto' : ' desafios abertos');
        if (qtd === 0) limpar($('feedEstado')).appendChild(vazio(q));
    } catch (e) {
        if (mine !== seq) return;
        if (!reiniciar) pagina--;
        if (reiniciar) limpar($('desafiosLista'));
        limpar($('feedEstado')).appendChild(estadoVazio({ erro: true, titulo: 'Não foi possível carregar os desafios',
            texto: mensagemErro(e), acao: { texto: 'Tentar de novo', onClick: function() { carregar(true); } } }));
    }
}

// Vazio diferente para "filtro zerou a lista" e "não há desafios"
function vazio(q) {
    if (q || catAtiva) {
        return estadoVazio({ icone: '🔎', titulo: 'Nenhum desafio com esses filtros', texto: 'Tente outra categoria ou outro termo de busca.',
            acao: { texto: 'Limpar filtros', onClick: function() { $('buscaInput').value = ''; catAtiva = ''; renderChips(); carregar(true); } } });
    }
    return estadoVazio({ icone: '📅', titulo: 'Nenhum desafio aberto agora',
        texto: 'As academias publicam desafios com frequência. Enquanto isso, que tal montar um treino?',
        acao: { texto: 'Ir para os treinos', href: '/treinos' } });
}

// ── Sugeridos (rodada 6: score do backend pelo nível + teias; "por quê" e base vêm prontos) ──
// item = { desafio, razao (texto puro: IA validada ou regra), razao_fonte: 'ia'|'regras', base: {...} }
function itensBase(b) {
    if (!b) return [];
    var l = [];
    if (b.sem_dados) l.push('Sem histórico recente');
    if (b.nivel) l.push('Seu nível ' + b.nivel);
    if (b.nivel_min || b.nivel_max) {
        l.push('Faixa ' + (b.nivel_min && b.nivel_max ? b.nivel_min + '–' + b.nivel_max : b.nivel_min ? b.nivel_min + '+' : 'até ' + b.nivel_max));
    }
    if (b.categoria_rotulo && b.valor_categoria != null) l.push(b.categoria_rotulo + ' ' + b.valor_categoria + '/100');
    (b.atributos || []).forEach(function(a) { if (a && a.rotulo) l.push(a.rotulo + ' ' + a.valor + '/100'); });
    if (b.valor_compativel) l.push('Entrada no seu padrão');
    return l;
}

function cartaoRecomendacao(item) {
    var d = item.desafio || {};
    var base = itensBase(item.base);
    var porQue = item.razao || '';
    var rotuloAcess = d.titulo + '. ' + (porQue ? 'Por que: ' + porQue + ' ' : '') + (base.length ? 'Baseado em: ' + base.join(', ') + '.' : '');
    return h('a', { class: 'recom-card', href: '/desafios?id=' + encodeURIComponent(d.id), 'aria-label': rotuloAcess }, [
        h('div', { class: 'recom-titulo', text: d.titulo }),
        h('div', { class: 'recom-valor', text: d.valor_entrada_centavos > 0 ? 'Entrada ' + fmtBRL(d.valor_entrada_centavos) : 'Grátis' }),
        porQue ? h('div', { class: 'recom-razao' }, [
            h('span', { class: 'recom-por-que' }, ['Por que', item.razao_fonte === 'ia' ? h('span', { class: 'recom-selo', title: 'Frase escrita por IA a partir dos seus números', text: 'IA' }) : null]),
            h('span', { text: porQue })
        ]) : null,
        base.length ? h('ul', { class: 'recom-base' }, [h('li', { class: 'recom-base-rot', text: 'Baseado em' })].concat(base.map(function(t) { return h('li', { text: t }); }))) : null
    ]);
}

async function carregarSugeridos() {
    var car = $('recomCarousel');
    Skeleton.cards(car, 3, 150);
    try {
        var lista = (await API.recomendados()).data || [];
        limpar(car);
        car.removeAttribute('aria-busy');
        if (lista.length === 0) {
            $('recomExplica').hidden = true;
            car.appendChild(h('p', { class: 'gs-hint', text: 'Sem sugestões agora: registre treinos e entre em desafios para receber sugestões pelo seu nível e pelos seus gráficos.' }));
            return;
        }
        lista.forEach(function(item) { if (item && item.desafio) car.appendChild(cartaoRecomendacao(item)); });
    } catch (e) {
        limpar(car);
        car.removeAttribute('aria-busy');
        car.appendChild(h('p', { class: 'gs-hint', text: 'Não foi possível carregar as sugestões agora.' }));
    }
}

renderChips();
carregar(true);
carregarSugeridos();
renderBottomNav('/menu');
})();
