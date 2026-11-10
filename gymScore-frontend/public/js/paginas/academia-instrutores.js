// Script da página academia-instrutores.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
(async function () {
var user = await Gestao.exigirPerfil('academia');
if (!user) return; // redirecionado para o login/home

var LIMITE = 20;
var pagina = 1, carregados = 0;
var lista = document.getElementById('lista');
var maisBtn = document.getElementById('maisBtn');
var avisoErro = document.getElementById('avisoErro');

function mostrarErro(msg) {
    avisoErro.textContent = msg;
    avisoErro.classList.toggle('show', !!msg);
}

async function desvincular(i) {
    mostrarErro('');
    var nome = nomeCompleto(i);
    var ok = await Modal.confirmar({
        titulo: 'Desvincular ' + nome + '?',
        mensagem: 'Ele volta a ser instrutor autônomo e é avisado. Não é possível desvincular quem é instrutor de um desafio aberto ou em andamento.',
        confirmar: 'Desvincular', perigo: true
    });
    if (!ok) return;
    try {
        await API.desvincularInstrutor(i.id);
        showToast('Instrutor desvinculado.', 'success');
        carregar(true);
    } catch (e) {
        // 409 traz os desafios que bloqueiam; a mensagem fica visível até a próxima ação
        mostrarErro(mensagemErro(e));
        avisoErro.scrollIntoView({ block: 'center' });
    }
}

function cartao(i) {
    var nome = nomeCompleto(i);
    return h('article', { class: 'gx-card', 'aria-label': nome }, [
        h('div', { class: 'gx-linha', style: { alignItems: 'center' } }, [
            h('div', { style: { display: 'flex', gap: '12px', alignItems: 'center', minWidth: 0 } }, [
                h('div', { class: 'gx-avatar', 'aria-hidden': 'true', text: nome.charAt(0).toUpperCase() }),
                h('div', { style: { minWidth: 0 } }, [
                    h('h3', { text: nome, style: { overflowWrap: 'anywhere' } }),
                    h('div', { class: 'gx-atleta-sub', text: 'CREF ' + (i.cref || '-') + (i.especialidade ? ' · ' + i.especialidade : '') }),
                    h('div', { class: 'gx-atleta-sub', text: i.email || '', style: { overflowWrap: 'anywhere' } })
                ])
            ]),
            i.desafios_ativos ? h('span', { class: 'gx-selo-ok', text: i.desafios_ativos + (i.desafios_ativos === 1 ? ' desafio ativo' : ' desafios ativos') }) : null
        ]),
        h('div', { class: 'gx-acoes' }, h('button', { type: 'button', class: 'gx-btn pequeno perigo', 'aria-label': 'Desvincular ' + nome,
            onClick: function() { desvincular(i); } }, 'Desvincular'))
    ]);
}

async function carregar(recomeco) {
    if (recomeco) { pagina = 1; carregados = 0; Skeleton.cards(lista, 2, 100); maisBtn.hidden = true; }
    try {
        var resp = await API.listarInstrutoresVinculados(pagina, LIMITE);
        var dados = resp.data || {};
        var itens = dados.itens || [];
        if (recomeco) { Skeleton.fim(lista); limpar(lista); }
        carregados += itens.length;
        itens.forEach(function(i) { lista.appendChild(cartao(i)); });
        document.getElementById('tituloLista').textContent = 'Instrutores vinculados (' + (dados.total || 0) + ')';
        if (!carregados) {
            lista.appendChild(estadoVazio({ icone: '🧑‍🏫', titulo: 'Nenhum instrutor vinculado ainda',
                texto: 'Informe o e-mail de um instrutor acima. Sem vinculados, as vagas de instrutor do desafio ficam abertas para candidatura.',
                acao: { texto: 'Criar desafio', href: '/academia/criar-desafio' } }));
        }
        maisBtn.hidden = carregados >= (dados.total || 0);
        maisBtn.disabled = false;
    } catch (e) {
        if (!recomeco) { pagina--; maisBtn.disabled = false; showToast(mensagemErro(e), 'error'); return; }
        Gestao.erro(lista, e, function() { carregar(true); });
    }
}

document.getElementById('formVincular').addEventListener('submit', async function(ev) {
    ev.preventDefault();
    mostrarErro('');
    var campo = document.getElementById('email');
    var email = campo.value.trim();
    if (!Valid.email(email)) { erroCampo(campo, 'Informe um e-mail válido.'); campo.focus(); return; }
    erroCampo(campo, '');
    var btn = document.getElementById('btnVincular');
    btnLoading(btn, true);
    try {
        var r = await API.vincularInstrutor(email);
        campo.value = '';
        showToast(r.data && r.data.convite_pendente ? 'Convite enviado! O vínculo vale quando o instrutor aceitar.' : 'Instrutor vinculado!', 'success');
        carregarConvites();
        carregar(true);
    } catch (e) {
        mostrarErro(mensagemErro(e));
    }
    btnLoading(btn, false);
});
// ─── Convites pendentes (academia → instrutor) ─────────────────────────────
async function carregarConvites() {
    var secao = document.getElementById('secaoConvites');
    var box = document.getElementById('convites');
    var itens;
    try { itens = (await API.listarConvitesAcademia()).data || []; }
    catch (e) { secao.hidden = true; return; }
    if (itens && itens.itens) itens = itens.itens;
    limpar(box);
    secao.hidden = !itens.length;
    document.getElementById('tituloConvites').textContent = 'Convites pendentes (' + itens.length + ')';
    itens.forEach(function(i) {
        var nome = nomeCompleto(i);
        var btn = h('button', { type: 'button', class: 'gx-btn pequeno contorno', 'aria-label': 'Cancelar convite de ' + nome, text: 'Cancelar convite' });
        btn.addEventListener('click', async function() {
            var ok = await Modal.confirmar({ titulo: 'Cancelar convite?', mensagem: nome + ' não poderá mais aceitar este convite.', confirmar: 'Cancelar convite', perigo: true });
            if (!ok) return;
            try { await API.cancelarConviteAcademia(i.id); showToast('Convite cancelado.', 'success'); carregarConvites(); }
            catch (e) { mostrarErro(mensagemErro(e)); }
        });
        box.appendChild(h('article', { class: 'gx-card', 'aria-label': nome }, [
            h('h3', { text: nome }),
            h('div', { class: 'gx-atleta-sub', text: 'CREF ' + (i.cref || '-') + (i.convite_em ? ' · convidado em ' + fmtData(i.convite_em) : '') }),
            h('div', { class: 'gx-atleta-sub', text: i.email || '' }),
            h('div', { class: 'gx-acoes' }, btn)
        ]));
    });
}
carregarConvites();

maisBtn.addEventListener('click', function() { maisBtn.disabled = true; pagina++; carregar(false); });

carregar(true);
renderBottomNav('/academia/instrutores');
})();
