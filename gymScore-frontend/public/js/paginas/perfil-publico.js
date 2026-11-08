// Script da página perfil-publico.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
if (!Auth.require()) throw new Error('unauthenticated');

var idAlvo = new URLSearchParams(location.search).get('id');
if (!idAlvo) window.location.href = '/amigos';

var currentUser = Auth.getUser();

async function carregarPerfil() {
    var loadMsg = document.getElementById('loadMsg');
    loadMsg.className = 'gs-msg'; loadMsg.textContent = 'Carregando...'; loadMsg.style.display = 'block';
    try {
        var [respPerfil, respHist] = await Promise.all([
            API.perfilPublico(idAlvo),
            API.historicoDesafios(idAlvo)
        ]);
        var u = respPerfil.data;
        var hist = respHist.data || [];
        loadMsg.style.display = 'none';
        document.getElementById('profileContent').hidden = false;

        var nome = (u.nome || '') + ' ' + (u.sobrenome || '');
        document.getElementById('pubName').textContent = nome.trim();
        document.getElementById('pubAvatar').textContent = (u.nome || '?').charAt(0).toUpperCase();

        var elo = u.elo || 'Bronze';
        var eloColor = {'Bronze':'#CD7F32','Prata':'#A0A0A0','Ouro':'#FFD700','Diamante':'#B9F2FF','Elite':'#C77DFF'}[elo] || '#888';
        var eloEl = document.getElementById('pubElo');
        eloEl.textContent = elo + ' ★';
        eloEl.style.color = eloColor;

        // Denunciar (não aparece no próprio perfil)
        if (!currentUser || String(currentUser.id) !== String(idAlvo)) {
            document.getElementById('btnDenunciar').href = '/denunciar?id=' + encodeURIComponent(idAlvo);
            document.getElementById('btnDenunciar').setAttribute('aria-label', 'Denunciar ' + nome.trim());
            document.getElementById('pubAcoes').hidden = false;
        }

        document.getElementById('pubPontos').textContent = u.pontos || 0;
        document.getElementById('pubDesafios').textContent = hist.length;

        if (hist.length === 0) {
            document.getElementById('histEmpty').hidden = false;
        } else {
            var ul = document.getElementById('histList');
            hist.forEach(function(d) {
                // Historico (Sprint 6): posicao (null fora do pódio), marca, data_evento
                var data = d.data_evento || d.criado_em || '';
                var meta = [];
                if (data) meta.push(fmtData(data));
                if (d.academia) meta.push(d.academia.nome_fantasia);
                if (d.marca) meta.push('marca ' + d.marca);
                ul.appendChild(h('li', { class: 'hist-item' }, [
                    h('div', { class: 'hist-titulo', text: d.titulo }),
                    h('div', { class: 'hist-meta', text: meta.join(' • ') }),
                    d.posicao != null ? h('span', { class: 'hist-pos p' + Math.min(d.posicao, 3), text: d.posicao + 'º lugar' }) : null
                ]));
            });
        }
    } catch (err) {
        loadMsg.className = 'gs-msg error'; loadMsg.textContent = mensagemErro(err); loadMsg.style.display = 'block';
    }
}

carregarPerfil();
renderBottomNav('/amigos');

// Eventos (antes eram atributos on* no HTML)
document.getElementById('btnVoltar').addEventListener('click', function() {
    if (history.length > 1) history.back(); else window.location.href = '/amigos';
});
