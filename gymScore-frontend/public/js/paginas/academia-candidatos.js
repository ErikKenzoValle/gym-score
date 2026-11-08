// Script da página academia-candidatos.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
(async function () {
var user = await Gestao.exigirPerfil('academia');
if (!user) return; // redirecionado para o login/home

var idDesafio = Gestao.param('id');
if (!idDesafio) { window.location.href = '/academia/desafios'; return; }
var conteudo = document.getElementById('conteudo');

var MODOS = { fixo: 'Valor fixo (o lance)', percentual: 'Percentual das inscrições', fixo_percentual: 'Fixo (o lance) + percentual' };

function resumoDesafio(d) {
    var f = d.financeiro || {};
    var teto = f.instrutor_teto_centavos;
    return h('div', { class: 'gx-card' }, [
        h('div', { class: 'gx-linha' }, [h('h2', { style: { fontSize: '1rem' }, text: d.titulo }), Gestao.selo(d.status)]),
        h('div', { class: 'gx-meta' }, [h('span', { text: Gestao.quando(d.data_evento) }), d.local ? h('span', { text: d.local }) : null]),
        Gestao.lista([
            ['Pagamento do instrutor', MODOS[f.instrutor_modo_pagamento] || '-'],
            teto !== null && teto !== undefined ? ['Teto do lance', fmtBRL(teto)] : null,
            f.instrutor_pct ? ['Percentual do instrutor', f.instrutor_pct + '%'] : null,
            ['Reservado do seu saldo', fmtBRL(f.reserva_centavos)]
        ]),
        d.instrutor ? h('p', { class: 'gx-confirma ok', text: 'Instrutor escolhido: ' + d.instrutor.nome }) : null
    ]);
}

async function escolher(d, c) {
    var f = d.financeiro || {};
    var teto = f.instrutor_teto_centavos || 0;
    var devolvido = Math.max(0, teto - c.lance_centavos);
    var ok = await Modal.confirmar({
        titulo: 'Escolher ' + c.instrutor.nome + '?',
        mensagem: 'A escolha é definitiva. Os outros candidatos são avisados e a vaga fecha. Para trocar depois, só cancelando o desafio.',
        linhas: [['Lance (parte fixa)', fmtBRL(c.lance_centavos)], ['Devolvido ao seu saldo', fmtBRL(devolvido)]],
        confirmar: 'Escolher este instrutor'
    });
    if (!ok) return;
    try {
        await API.escolherCandidatura(d.id, c.id);
        showToast('Instrutor escolhido!', 'success');
        carregar();
    } catch (e) { showToast(mensagemErro(e), 'error'); carregar(); }
}

function cardCandidato(d, c) {
    var i = c.instrutor || {};
    var pode = d.pode_escolher_instrutor && c.status === 'pendente';
    return h('article', { class: 'gx-card' + (c.status === 'escolhida' ? ' destaque' : ''), 'aria-label': 'Candidato ' + (i.nome || '') }, [
        h('div', { class: 'gx-linha' }, [
            h('div', { style: { display: 'flex', gap: '12px', alignItems: 'center', minWidth: 0 } }, [
                h('div', { class: 'gx-avatar', 'aria-hidden': 'true', text: (i.nome || '?').charAt(0).toUpperCase() }),
                h('div', { style: { minWidth: 0 } }, [
                    h('h3', { text: i.nome || 'Instrutor', style: { overflowWrap: 'anywhere' } }),
                    h('div', { class: 'gx-atleta-sub', text: 'CREF ' + (i.cref || '-') + (i.especialidade ? ' · ' + i.especialidade : '') })
                ])
            ]),
            h('span', { class: 'gx-status ' + c.status, text: c.status })
        ]),
        h('div', { class: 'gx-linha', style: { alignItems: 'center', marginTop: '10px' } }, [
            h('div', null, [h('div', { class: 'gx-kpi-rotulo', text: 'Lance' }), h('div', { class: 'gx-lance', text: fmtBRL(c.lance_centavos) })]),
            i.vinculado ? h('span', { class: 'gx-selo-ok', text: 'Vinculado à academia' }) : h('span', { class: 'gx-selo-esp', text: 'Autônomo' })
        ]),
        c.mensagem ? h('div', { class: 'gx-msg-cand', text: c.mensagem }) : null,
        pode ? h('div', { class: 'gx-acoes' }, h('button', { type: 'button', class: 'gx-btn', onClick: function() { escolher(d, c); } }, 'Escolher este instrutor')) : null
    ]);
}

async function carregar() {
    Skeleton.cards(conteudo, 3, 120);
    try {
        var res = await Promise.all([API.buscarDesafio(idDesafio), API.listarCandidaturas(idDesafio)]);
        var d = res[0].data, lista = res[1].data || [];
        Skeleton.fim(conteudo);
        limpar(conteudo);
        conteudo.appendChild(resumoDesafio(d));

        if (d.instrutor_selecao !== 'vaga_aberta') {
            conteudo.appendChild(estadoVazio({ icone: 'ℹ️', titulo: 'Este desafio não tem vaga de instrutor aberta', texto: 'O instrutor foi designado na criação.',
                acao: { texto: 'Gerenciar desafio', href: '/academia/podio?id=' + d.id } }));
            return;
        }
        conteudo.appendChild(h('h2', { class: 'gx-secao', text: lista.length + (lista.length === 1 ? ' candidato' : ' candidatos') + ' · do menor para o maior lance' }));
        if (!lista.length) {
            conteudo.appendChild(estadoVazio({ icone: '⏳', titulo: 'Ainda não há candidatos', texto: 'Os instrutores veem a vaga na aba Vagas. Você é avisado a cada nova candidatura.',
                acao: { texto: 'Atualizar', onClick: carregar }, secundaria: { texto: 'Editar o teto', href: '/academia/criar-desafio?id=' + d.id } }));
            return;
        }
        lista.forEach(function(c) { conteudo.appendChild(cardCandidato(d, c)); });
        if (d.instrutor) conteudo.appendChild(h('div', { class: 'gx-acoes' }, h('a', { class: 'gx-btn', href: '/academia/podio?id=' + d.id }, 'Gerenciar desafio')));
    } catch (e) {
        Gestao.erro(conteudo, e, carregar);
    }
}

carregar();
renderBottomNav('/academia/desafios');
})();
