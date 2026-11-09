// Script da página amigos.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
if (!Auth.require()) throw new Error('unauthenticated');

var user = Auth.getUser();
if (user) document.body.dataset.perfil = Modo.atual(user);

var searchTimeout = null;
var searchSeq = 0; // descarta respostas antigas da busca
var _pendenteAberto = false;

// Cores de avatar por ID mod 8
var _avColors = ['av-0','av-1','av-2','av-3','av-4','av-5','av-6','av-7'];

function avatarColor(id) { return _avColors[(id || 0) % _avColors.length]; }

// ELO badge
var _eloClass = {
    'Bronze':'elo-bronze','Prata':'elo-prata','Ouro':'elo-ouro',
    'Diamante':'elo-diamante','Elite':'elo-elite','Iniciante':'elo-iniciante'
};


function togglePendentes() {
    _pendenteAberto = !_pendenteAberto;
    document.getElementById('pendentesList').style.display = _pendenteAberto ? 'block' : 'none';
}

// ── Carrega lista de amigos ───────────────────────────────────────────────
// Identidade sempre vem do token: a API não aceita (nem precisa de) id_usuario no corpo.
async function loadFriends() {
    var statusEl = document.getElementById('statusMsg');
    statusEl.className = 'gs-msg show';
    statusEl.textContent = 'Carregando...';
    document.getElementById('emptyState').style.display = 'none';

    try {
        var resp  = await API.listarAmigos();
        var dados = resp.data || [];
        statusEl.classList.remove('show');

        var aceitos   = dados.filter(function(a){ return a.status === 'aceita'; });
        var recebidas = dados.filter(function(a){ return a.status === 'pendente' && a.recebida; });
        var enviadas  = dados.filter(function(a){ return a.status === 'pendente' && !a.recebida; });

        // Contador
        document.getElementById('conexoesCount').textContent = aceitos.length + ' conexões ativas';

        // Pendentes recebidas
        if (recebidas.length > 0) {
            document.getElementById('pendenteCard').style.display = 'block';
            document.getElementById('pendenteText').textContent =
                recebidas.length + ' SOLICITAÇÃO' + (recebidas.length > 1 ? 'S' : '') + ' PENDENTE' + (recebidas.length > 1 ? 'S' : '');
            var rl = document.getElementById('requestsList');
            limpar(rl);
            recebidas.forEach(function(a) { rl.appendChild(buildFriendItem(a, 'request')); });
        } else {
            document.getElementById('pendenteCard').style.display = 'none';
        }

        // Enviadas
        if (enviadas.length > 0) {
            document.getElementById('sentSection').style.display = 'block';
            var sl = document.getElementById('sentList');
            limpar(sl);
            enviadas.forEach(function(a) { sl.appendChild(buildFriendItem(a, 'sent')); });
        } else {
            document.getElementById('sentSection').style.display = 'none';
        }

        // Aceitos
        if (aceitos.length > 0) {
            document.getElementById('friendsSection').style.display = 'block';
            var fl = document.getElementById('friendsList');
            limpar(fl);
            aceitos.forEach(function(a) { fl.appendChild(buildFriendItem(a, 'friend')); });
        } else {
            document.getElementById('friendsSection').style.display = 'none';
        }

        if (dados.length === 0) {
            document.getElementById('emptyState').style.display = 'block';
        }
    } catch(err) {
        statusEl.className = 'gs-msg error show';
        statusEl.textContent = 'Erro ao carregar amigos.';
    }
}

function buildFriendItem(a, tipo) {
    var nome     = ((a.nome || '') + ' ' + (a.sobrenome || '')).trim() || 'Usuário';
    var username = '@' + (a.nome || 'usuario').toLowerCase().replace(/\s+/g, '_');
    var elo      = a.elo || 'Iniciante';
    var eloClass = _eloClass[elo] || 'elo-iniciante';
    var inicial  = (a.nome || '?').charAt(0).toUpperCase();

    var actions = null;
    if (tipo === 'request') {
        actions = h('div', { style: { flexShrink: '0' } }, [
            h('button', { type: 'button', class: 'btn-aceitar', 'aria-label': 'Aceitar solicitação de ' + nome, text: 'Aceitar',
                onClick: function() { acceptFriend(a.id, this); } }),
            h('button', { type: 'button', class: 'btn-recusar', 'aria-label': 'Recusar solicitação de ' + nome, text: 'Recusar',
                onClick: function() { removeFriend(a.id, this); } })
        ]);
    } else if (tipo === 'sent') {
        actions = h('div', { style: { flexShrink: '0' } }, h('span', { class: 'tag-enviado', text: 'Enviado' }));
    }
    // tipo 'friend' — sem botão de ação (já amigos)

    return h('div', { class: 'friend-item' }, [
        h('div', { class: 'friend-avatar ' + avatarColor(a.id), 'aria-hidden': 'true', text: inicial }),
        h('a', { class: 'friend-info', href: '/perfil-publico?id=' + encodeURIComponent(a.id), style: { color: 'inherit', textDecoration: 'none' } }, [
            h('div', { class: 'friend-name', text: nome }),
            h('div', { class: 'friend-username', text: username }),
            h('div', { class: 'friend-elo-row' }, [
                h('span', { class: 'elo-badge ' + eloClass, text: elo }),
                a.pontos ? h('span', { style: { fontSize: '.75rem', color: 'var(--text-muted)' }, text: a.pontos + ' XP' }) : null
            ])
        ]),
        actions
    ]);
}

// ── Aceitar ───────────────────────────────────────────────────────────────
// Identidade sempre vem do token: a API só recebe o id do outro lado (idAmigo).
async function acceptFriend(idAmigo, btn) {
    btn.disabled = true;
    try {
        await API.aceitarAmizade(idAmigo);
        showToast('Amizade aceita!', 'success');
        loadFriends();
    } catch(err) {
        showToast(mensagemErro(err), 'error');
        btn.disabled = false;
    }
}

// ── Remover / Recusar ─────────────────────────────────────────────────────
// RemoverAmigo desfaz a amizade ou cancela/recusa a solicitação pendente, em qualquer direção.
async function removeFriend(idAmigo, btn) {
    btn.disabled = true;
    try {
        await API.removerAmigo(idAmigo);
        showToast('Amigo removido.', 'success');
        loadFriends();
    } catch(err) {
        showToast(mensagemErro(err), 'error');
        btn.disabled = false;
    }
}

// ── Busca (debounce 400ms) ─────────────────────────────────────────────────
function onSearchInput() {
    clearTimeout(searchTimeout);
    var q = document.getElementById('searchInput').value.trim();
    document.getElementById('clearSearchBtn').style.display = q ? 'block' : 'none';
    if (q.length < 2) { hideSearchResults(); return; }
    searchTimeout = setTimeout(function() { buscarUsuarios(q); }, 400);
}

function clearSearch() {
    document.getElementById('searchInput').value = '';
    document.getElementById('clearSearchBtn').style.display = 'none';
    document.getElementById('searchMsg').className = 'gs-msg';
    hideSearchResults();
}

async function buscarUsuarios(q) {
    var resultsEl = document.getElementById('searchResults');
    var seq = ++searchSeq;
    resultsEl.style.display = 'block';
    limpar(resultsEl).appendChild(h('div', { class: 'search-loading', text: 'Buscando...' }));
    try {
        // /api/usuarios/busca já exclui o próprio usuário logado na resposta
        var resp  = await API.buscarUsuarios(q);
        if (seq !== searchSeq) return;
        var lista = (resp.data || []).filter(function(u){ return u.id !== user.id; });
        renderSearchResults(lista);
    } catch(e) {
        if (seq !== searchSeq) return;
        limpar(resultsEl).appendChild(h('div', { class: 'search-empty', text: 'Erro ao buscar. ' + mensagemErro(e) }));
    }
}

function renderSearchResults(lista) {
    var resultsEl = limpar(document.getElementById('searchResults'));
    if (lista.length === 0) {
        resultsEl.appendChild(h('div', { class: 'search-empty', text: 'Nenhum resultado.' }));
        return;
    }
    lista.forEach(function(u) {
        var nome     = ((u.nome || '') + ' ' + (u.sobrenome || '')).trim();
        var elo      = u.elo || 'Iniciante';
        var eloClass = _eloClass[elo] || 'elo-iniciante';
        resultsEl.appendChild(h('div', { class: 'search-result-item' }, [
            h('div', { class: 'friend-avatar ' + avatarColor(u.id), 'aria-hidden': 'true', style: { width: '36px', height: '36px', fontSize: '.88rem' },
                text: (u.nome || '?').charAt(0).toUpperCase() }),
            h('a', { href: '/perfil-publico?id=' + encodeURIComponent(u.id), style: { flex: '1', color: 'inherit', textDecoration: 'none' } }, [
                h('div', { style: { fontWeight: '600', fontSize: '.88rem' }, text: nome }),
                h('span', { class: 'elo-badge ' + eloClass, style: { marginTop: '3px' }, text: elo })
            ]),
            h('button', { type: 'button', class: 'btn-adicionar', 'aria-label': 'Adicionar ' + nome, text: 'ADICIONAR',
                onClick: function() { addFriend(u.id, this, nome); } })
        ]));
    });
}

function hideSearchResults() {
    var el = document.getElementById('searchResults');
    el.style.display = 'none';
    limpar(el);
}

document.addEventListener('click', function(e) {
    if (!e.target.closest('.busca-wrap')) hideSearchResults();
});

// ── Adicionar amigo ───────────────────────────────────────────────────────
async function addFriend(idAmigo, btn, nome) {
    btn.disabled = true;
    btn.textContent = '...';
    var msgEl = document.getElementById('searchMsg');
    msgEl.className = 'gs-msg';
    try {
        await API.adicionarAmigo(idAmigo);
        showToast('Solicitação enviada para ' + nome + '!', 'success');
        hideSearchResults();
        document.getElementById('searchInput').value = '';
        document.getElementById('clearSearchBtn').style.display = 'none';
        loadFriends();
    } catch(err) {
        msgEl.className = 'gs-msg error show';
        msgEl.textContent = mensagemErro(err);
        btn.disabled = false;
        btn.textContent = 'ADICIONAR';
    }
}

// ── Abas Amigos / Ranking ─────────────────────────────────────────────────
function selecionarAba(aba) {
    var ranking = aba === 'ranking';
    document.getElementById('painelAmigos').hidden = ranking;
    document.getElementById('painelRanking').hidden = !ranking;
    [['tabAmigos', !ranking], ['tabRanking', ranking]].forEach(function(t) {
        var el = document.getElementById(t[0]);
        el.classList.toggle('active', t[1]);
        el.setAttribute('aria-selected', t[1] ? 'true' : 'false');
    });
    if (ranking) loadRanking();
}

// ── Ranking entre amigos ──────────────────────────────────────────────────
async function loadRanking() {
    var msgEl = document.getElementById('rankingMsg');
    var listEl = limpar(document.getElementById('rankingList'));
    var emptyEl = document.getElementById('rankingEmpty');
    emptyEl.hidden = true;
    msgEl.className = 'gs-msg show';
    msgEl.textContent = 'Carregando ranking...';
    try {
        var resp = await API.rankingAmigos(document.getElementById('rankingDias').value);
        var itens = resp.data || [];
        msgEl.classList.remove('show');
        var temPontos = itens.some(function(r) { return r.pontos > 0; });
        emptyEl.hidden = temPontos;
        if (!temPontos) return;
        itens.forEach(function(r) {
            var nome = nomeCompleto(r) + (r.voce ? ' (você)' : '');
            listEl.appendChild(h('li', { class: 'ranking-item' + (r.voce ? ' voce' : '') }, [
                h('span', { class: 'ranking-pos', text: r.posicao + 'º' }),
                h('div', { class: 'ranking-info' }, [
                    h('span', { class: 'friend-name', text: nome }),
                    h('span', { class: 'friend-email',
                        text: r.exercicios + ' exercício(s) · ' + r.checkins + ' check-in(s)' })
                ]),
                h('span', { class: 'ranking-pontos', text: r.pontos + ' pts' })
            ]));
        });
    } catch (err) {
        msgEl.className = 'gs-msg error show';
        msgEl.textContent = 'Erro ao carregar o ranking.';
    }
}

document.getElementById('tabAmigos').addEventListener('click', function() { selecionarAba('amigos'); });
document.getElementById('tabRanking').addEventListener('click', function() { selecionarAba('ranking'); });
document.getElementById('rankingDias').addEventListener('change', loadRanking);

loadFriends();
renderBottomNav('/amigos');

// Eventos (antes eram atributos on* no HTML)
document.getElementById('searchInput').addEventListener('input', function() { onSearchInput(); });
document.getElementById('clearSearchBtn').addEventListener('click', function() { clearSearch(); });
document.getElementById('btnVerPendentes').addEventListener('click', function() { togglePendentes(); });
document.getElementById('btnBuscarAmigos').addEventListener('click', function() { document.getElementById('searchInput').focus(); });
