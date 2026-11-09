// Script da página perfil.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
if (!Auth.require()) throw new Error('unauthenticated');
checkLGPDConsent();

var user = Auth.getUser();
if (user) document.body.dataset.perfil = Modo.atual(user);

// ── Escala de elos ────────────────────────────────────────────────────────
var _ELOS = [
    { nome:'Iniciante', min:0,    max:99,   cls:'elo-iniciante', dot:'#888'     },
    { nome:'Bronze',    min:100,  max:499,  cls:'elo-bronze',    dot:'#b08d57'  },
    { nome:'Prata',     min:500,  max:999,  cls:'elo-prata',     dot:'#a0a0b0'  },
    { nome:'Ouro',      min:1000, max:2999, cls:'elo-ouro',      dot:'#ffc800'  },
    { nome:'Diamante',  min:3000, max:7999, cls:'elo-diamante',  dot:'#64b4ff'  },
    { nome:'Elite',     min:8000, max:null, cls:'elo-elite',     dot:'#c864ff'  }
];

function getEloInfo(xp) {
    for (var i = _ELOS.length - 1; i >= 0; i--) {
        if (xp >= _ELOS[i].min) return { elo: _ELOS[i], idx: i };
    }
    return { elo: _ELOS[0], idx: 0 };
}

function renderHero() {
    if (!user) return;
    var inicial = (user.nome || '?').charAt(0).toUpperCase();
    document.getElementById('perfilAvatar').textContent = inicial;
    document.getElementById('perfilNome').textContent = ((user.nome || '') + ' ' + (user.sobrenome || '')).trim().toUpperCase();

    var modo = Modo.atual(user);
    var rotulo = modo === 'academia' ? 'Academia desde '
        : modo === 'instrutor' ? 'Instrutor desde '
        : 'Atleta desde ';
    var criadoEm = user.criado_em;
    if (criadoEm) {
        var dt = new Date(criadoEm);
        var meses = ['Jan','Fev','Mar','Abr','Mai','Jun','Jul','Ago','Set','Out','Nov','Dez'];
        document.getElementById('perfilDesde').textContent =
            rotulo + meses[dt.getMonth()] + ' ' + dt.getFullYear();
    }
}

function renderElo() {
    var xp = parseInt(user.pontos || 0);
    var info = getEloInfo(xp);
    var elo  = info.elo;
    var next = info.idx < _ELOS.length - 1 ? _ELOS[info.idx + 1] : null;

    document.getElementById('eloNome').textContent = elo.nome.toUpperCase();
    document.getElementById('xpAtual').textContent = xp + ' XP';
    if (next) {
        document.getElementById('xpProximo').textContent = 'Próximo: ' + next.min + ' XP';
        var pct = Math.min(100, Math.round((xp - elo.min) / (next.min - elo.min) * 100));
        document.getElementById('xpFill').style.width = pct + '%';
    } else {
        document.getElementById('xpProximo').textContent = 'Elo máximo!';
        document.getElementById('xpFill').style.width = '100%';
    }

    // Escala
    var scale = limpar(document.getElementById('eloScale'));
    _ELOS.forEach(function(e) {
        var isAtual = e.nome === elo.nome;
        scale.appendChild(h('div', { class: 'elo-scale-item' + (isAtual ? ' atual' : '') }, [
            h('div', { class: 'elo-scale-dot', style: { background: e.dot } }),
            h('span', { class: 'elo-scale-name elo-badge ' + e.cls, text: e.nome }),
            h('span', { class: 'elo-scale-xp', text: e.min + (e.max ? '–' + e.max : '+') + ' XP' }),
            isAtual ? h('span', { class: 'elo-scale-voce', text: '← VOCÊ' }) : null
        ]));
    });
}

// ── Histórico de conquistas ───────────────────────────────────────────────
var _shareData = null;

async function carregarHistorico() {
    var container = limpar(document.getElementById('historicoLista'));
    try {
        // GET /usuarios/:id/historico (array puro): desafios encerrados com posicao (null fora do pódio), marca e premio_centavos
        var desafios = (await API.historicoDesafios(user.id)).data || [];
        var podios   = desafios.filter(function(d) { return d.posicao != null; });

        document.getElementById('statPodios').textContent   = podios.length;
        document.getElementById('statDesafios').textContent = desafios.length;
        document.getElementById('statGanhos').textContent   = fmtBRL(desafios.reduce(function(s, d) { return s + (d.premio_centavos || 0); }, 0));

        if (desafios.length === 0) {
            container.appendChild(estadoVazio({ titulo: 'Nenhuma participação ainda',
                texto: 'Seus resultados aparecem aqui depois do primeiro desafio.', acao: { texto: 'Ver desafios', href: '/desafios' } }));
            return;
        }

        desafios.slice(0, 10).forEach(function(d) {
            var noPodio = d.posicao != null;
            var meta = [];
            if (d.data_evento) meta.push(new Date(d.data_evento).toLocaleDateString('pt-BR', { month: 'short', year: 'numeric' }));
            if (noPodio) meta.push(d.posicao + 'º lugar');
            else meta.push(d.presente ? 'Participou' : 'Não compareceu');
            if (d.marca) meta.push('marca ' + d.marca);
            if (d.premio_centavos > 0) meta.push('prêmio ' + fmtBRL(d.premio_centavos));
            container.appendChild(h('div', { class: 'historico-item' }, [
                h('div', { class: 'historico-icon', 'aria-hidden': 'true', text: d.vencedor ? '🥇' : (noPodio ? '🏅' : '🏃') }),
                h('div', { class: 'historico-info' }, [
                    h('div', { class: 'historico-titulo', text: d.titulo || 'Desafio' }),
                    h('div', { class: 'historico-meta', text: meta.join(' • ') })
                ]),
                noPodio ? h('button', { type: 'button', class: 'historico-share', 'aria-label': 'Compartilhar conquista: ' + d.titulo, text: '↗',
                    onClick: function() { abrirShare(d); } }) : null
            ]));
        });
    } catch(e) {
        limpar(container).appendChild(estadoVazio({ erro: true, titulo: 'Não foi possível carregar o histórico', texto: mensagemErro(e),
            acao: { texto: 'Tentar de novo', onClick: carregarHistorico } }));
    }
}

function abrirShare(desafio) {
    _shareData = desafio;
    renderCanvas(desafio);
    Modal.abrir(document.getElementById('shareOverlay'));
}
function fecharShare() { Modal.fechar(document.getElementById('shareOverlay')); }

function renderCanvas(desafio) {
    var canvas = document.getElementById('shareCanvas');
    var ctx    = canvas.getContext('2d');
    var w = canvas.width, h = canvas.height;
    var grad = ctx.createLinearGradient(0,0,w,h);
    grad.addColorStop(0,'#0D1117');
    grad.addColorStop(1,'#1a2a1a');
    ctx.fillStyle = grad;
    ctx.fillRect(0,0,w,h);
    ctx.strokeStyle = '#22c55e';
    ctx.lineWidth = 3;
    ctx.beginPath();
    ctx.roundRect(2,2,w-4,h-4,12);
    ctx.stroke();
    ctx.font = 'bold 48px system-ui'; ctx.textAlign = 'center';
    ctx.fillText('🥇', w/2, 72);
    ctx.font = 'bold 14px system-ui'; ctx.fillStyle = '#22c55e';
    ctx.fillText('GymScore', w/2, 106);
    ctx.font = 'bold 18px system-ui'; ctx.fillStyle = '#EEF0F5';
    var titulo = desafio.titulo || '';
    if (titulo.length > 28) titulo = titulo.substring(0,25) + '…';
    ctx.fillText(titulo, w/2, 136);
    ctx.font = '13px system-ui'; ctx.fillStyle = '#7B8499';
    ctx.fillText(desafio.posicao + 'º lugar' + (desafio.premio_centavos > 0 ? ' • ' + fmtBRL(desafio.premio_centavos) : ''), w/2, 158);
    ctx.font = 'bold 13px system-ui'; ctx.fillStyle = '#EEF0F5';
    ctx.fillText((user.nome||'') + ' ' + (user.sobrenome||''), w/2, 185);
}

async function compartilhar() {
    if (!_shareData) return;
    var canvas = document.getElementById('shareCanvas');
    var titulo = _shareData.titulo || 'Conquista';
    var url    = window.location.origin + '/desafios?id=' + _shareData.id;
    var texto  = 'Fiquei em ' + _shareData.posicao + 'º lugar no desafio "' + titulo + '" no GymScore!';
    if (navigator.share) {
        try {
            var blob = await new Promise(function(res){ canvas.toBlob(res,'image/png'); });
            var file = new File([blob],'conquista.png',{type:'image/png'});
            await navigator.share({ title:'GymScore — '+titulo, text:texto, url:url, files:[file] });
        } catch(err) {
            if (err.name !== 'AbortError') {
                await navigator.share({ title:'GymScore — '+titulo, text:texto, url:url }).catch(function(){});
            }
        }
    } else { copiarLink(); }
}

function copiarLink() {
    if (!_shareData) return;
    var url = window.location.origin + '/desafios?id=' + _shareData.id;
    navigator.clipboard.writeText(url)
        .then(function() { showToast('Link copiado!', 'success'); })
        .catch(function() { showToast('Não foi possível copiar o link.', 'error'); });
}

// ── Dados pessoais (com privacidade) ────────────────────────────────────────
var generoMap = { M: 'Masculino', F: 'Feminino', O: 'Outro' };

function formatDate(raw) {
    if (!raw) return '-';
    // Suporta "2005-01-08" e "2005-01-08T00:00:00..."
    var parts = String(raw).split('T')[0].split('-');
    if (parts.length === 3) return parts[2] + '/' + parts[1] + '/' + parts[0];
    return String(raw);
}

// Uma linha do perfil: rótulo + valor (texto puro, mascarável via data-private)
function linha(rotulo, valor, tipoPrivado) {
    var span = h('span', { text: valor, 'data-private': tipoPrivado || null });
    return h('p', null, [h('strong', { text: rotulo }), span]);
}

function renderProfile() {
    var box = limpar(document.getElementById('profileInfo'));
    if (!user) { box.appendChild(h('p', { text: 'Erro ao carregar perfil.' })); return; }
    box.appendChild(linha('Nome', nomeCompleto(user), 'name'));
    box.appendChild(linha('E-mail', user.email || '-', 'email'));
    box.appendChild(linha('CPF', user.cpf || '-', 'cpf'));
    box.appendChild(linha('Nascimento', formatDate(user.data_nascimento), 'date'));
    box.appendChild(linha('Gênero', generoMap[user.genero] || user.genero || '-'));
    box.appendChild(linha('Saldo', fmtBRL(user.saldo_centavos), 'currency'));
    Privacy.applyToDOM();
}

Privacy.renderToggle('privacyToggle');
document.addEventListener('privacyChange', renderProfile);
document.getElementById('logoutLink').addEventListener('click', function(e) {
    e.preventDefault();
    Auth.logout();
});

async function init() {
    user = await Auth.refresh();
    if (user) document.body.dataset.perfil = Modo.atual(user);
    renderHero();
    renderProfile();
    renderPedidoInstrutor();
    if (user && user.perfil !== 'atleta') {
        // Elo/XP/histórico de participações só fazem sentido para atletas
        document.getElementById('atletaSection').style.display = 'none';
        document.getElementById('historicoCard').style.display = 'none';
        document.querySelectorAll('.atleta-only').forEach(function(el) { el.hidden = true; });
        return;
    }
    renderElo();
    carregarHistorico();
}

init();
renderBottomNav('/perfil');

// Botão do pedido de instrutor: "Sou instrutor" sem pedido; com pedido, mostra o estado (o seletor de modo
// fica no topo). Rodada 4: o pedido não muda o tipo de conta.
function renderPedidoInstrutor() {
    var a = document.getElementById('btnSouInstrutor');
    if (!a || !user || user.perfil !== 'atleta') return;
    var st = Modo.pedidoInstrutor(user);
    if (!st) return;
    if (st === 'aprovada') { a.hidden = true; return; }
    a.textContent = st === 'rejeitada' ? 'Pedido de instrutor: recusado' : 'Pedido de instrutor: em análise';
    a.setAttribute('aria-label', a.textContent + ' (ver detalhes)');
}

// Eventos (antes eram atributos on* no HTML)
document.getElementById('btnCompartilhar').addEventListener('click', function() { compartilhar(); });
document.getElementById('btnCopiarLink').addEventListener('click', function() { copiarLink(); });
document.getElementById('btnFecharShare').addEventListener('click', function() { fecharShare(); });
