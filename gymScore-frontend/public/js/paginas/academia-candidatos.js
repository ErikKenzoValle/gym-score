// Script da página academia-candidatos.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
// Rodada 5: gestão da EQUIPE de instrutores de um desafio (fim do leilão): equipe atual com o
// responsável pelo pódio, indicar vinculados, remover, aceitar/recusar candidaturas.
(async function () {
var user = await Gestao.exigirPerfil('academia');
if (!user) return; // redirecionado para o login/home

var idDesafio = Gestao.param('id');
if (!idDesafio) { window.location.href = '/academia/desafios'; return; }
var conteudo = document.getElementById('conteudo');

// Executa uma ação da equipe com feedback e recarrega a tela
async function acao(btn, fn, msgOk) {
    if (btn) btnLoading(btn, true);
    try {
        await fn();
        showToast(msgOk, 'success');
    } catch (e) {
        showToast(mensagemErro(e), 'error');
    }
    carregar();
}

function resumoDesafio(d) {
    var f = d.financeiro || {};
    var livres = d.vagas_instrutor_abertas || 0;
    return h('div', { class: 'gx-card' }, [
        h('div', { class: 'gx-linha' }, [h('h2', { text: d.titulo }), Gestao.selo(d.status)]),
        h('div', { class: 'gx-meta' }, [h('span', { text: Gestao.horario(d) }), Gestao.enderecoCurto(d) ? h('span', { text: Gestao.enderecoCurto(d) }) : null]),
        h('div', { class: 'gx-kpi-rotulo', text: 'Condições oferecidas' }),
        h('div', { class: 'gx-condicoes', text: Gestao.textoCondicoes(f.instrutor_modo_pagamento, f.instrutor_fixo_centavos, f.instrutor_pct) }),
        Gestao.lista([
            ['Instrutores no evento', String(d.instrutores_vagas || 1)],
            ['Na equipe', String((d.equipe || []).length)],
            ['Vagas abertas', String(livres)],
            ['Reservado do seu saldo', fmtBRL(f.reserva_centavos)]
        ]),
        d.pode_editar ? h('div', { class: 'gx-acoes' }, h('a', { class: 'gx-btn pequeno contorno', href: '/academia/criar-desafio?id=' + d.id }, 'Editar nº de instrutores ou pagamento')) : null
    ]);
}

function secaoEquipe(d) {
    var nome = function(m) { return m.nome || 'instrutor'; };
    var resp = (d.equipe || []).filter(function(m) { return m.responsavel; })[0];
    return h('section', { class: 'gx-card', 'aria-labelledby': 'secEquipe' }, [
        h('h2', { class: 'gx-card-titulo', id: 'secEquipe', text: 'Equipe (' + (d.equipe || []).length + ' de ' + (d.instrutores_vagas || 1) + ')' }),
        h('p', { class: 'gx-texto', text: resp ? 'Todos fazem a chamada; só ' + resp.nome + ', o responsável, lança o pódio.'
            : 'Sem responsável definido: o desafio só pode ser iniciado com ao menos um instrutor e o responsável.' }),
        Gestao.equipe(d, { acoes: function(m) {
            var a = [];
            if (d.pode_definir_responsavel && !m.responsavel) {
                var bResp = h('button', { type: 'button', class: 'gx-btn pequeno contorno', 'aria-label': 'Tornar ' + nome(m) + ' o responsável pelo pódio', text: 'Tornar responsável' });
                bResp.addEventListener('click', async function() {
                    var ok = await Modal.confirmar({ titulo: 'Trocar o responsável?', confirmar: 'Tornar responsável',
                        mensagem: nome(m) + ' passa a ser o único que lança o pódio deste desafio' + (resp ? ' (no lugar de ' + resp.nome + ')' : '') + '. A equipe é avisada.' });
                    if (ok) acao(bResp, function() { return API.definirResponsavel(d.id, m.id); }, nome(m) + ' agora é o responsável.');
                });
                a.push(bResp);
            }
            if (d.pode_gerir_equipe) {
                var bRem = h('button', { type: 'button', class: 'gx-btn pequeno perigo', 'aria-label': 'Remover ' + nome(m) + ' da equipe', text: 'Remover' });
                bRem.addEventListener('click', async function() {
                    var ok = await Modal.confirmar({ titulo: 'Remover ' + nome(m) + '?', perigo: true, confirmar: 'Remover da equipe',
                        mensagem: 'O instrutor é avisado e a vaga volta a ficar aberta.' + (m.responsavel ? ' Como é o responsável, o próximo da equipe assume.' : '') });
                    if (ok) acao(bRem, function() { return API.removerDaEquipe(d.id, m.id); }, nome(m) + ' saiu da equipe.');
                });
                a.push(bRem);
            }
            return a;
        } })
    ]);
}

// Indicar um instrutor vinculado (entra direto na equipe)
function secaoIndicar(d, vinculados) {
    var naEquipe = (d.equipe || []).map(function(m) { return m.id; });
    var disponiveis = vinculados.filter(function(i) { return !i.convite_pendente && naEquipe.indexOf(i.id) < 0; });
    var corpo;
    if (!vinculados.length) {
        corpo = h('p', { class: 'gx-texto' }, ['Você não tem instrutores vinculados. ', h('a', { class: 'gs-link', href: '/academia/instrutores' }, 'Vincular um instrutor')]);
    } else if (!disponiveis.length) {
        corpo = h('p', { class: 'gx-texto', text: 'Todos os seus instrutores vinculados já estão na equipe.' });
    } else {
        var sel = h('select', { id: 'indicarSel', class: 'gs-input gs-input-select' }, [h('option', { value: '', text: '— Escolha o instrutor —' })].concat(
            disponiveis.map(function(i) { return h('option', { value: String(i.id), text: nomeCompleto(i) + (i.cref ? ' · CREF ' + i.cref : '') }); })));
        var btn = h('button', { type: 'submit', class: 'gx-btn pequeno', text: 'Indicar' });
        corpo = h('form', { novalidate: true, onSubmit: function(ev) {
            ev.preventDefault();
            var id = parseInt(sel.value, 10);
            if (!id) { erroCampo(sel, 'Escolha o instrutor.'); sel.focus(); return; }
            erroCampo(sel, '');
            acao(btn, function() { return API.indicarInstrutor(d.id, id); }, 'Instrutor indicado: ele entrou na equipe.');
        } }, [
            h('div', { class: 'gs-field' }, [h('label', { class: 'gs-label', for: 'indicarSel', text: 'Instrutor vinculado' }), sel]),
            btn
        ]);
    }
    return h('section', { class: 'gx-card', 'aria-labelledby': 'secIndicar' }, [
        h('h2', { class: 'gx-card-titulo', id: 'secIndicar', text: 'Indicar instrutor vinculado' }),
        corpo
    ]);
}

function cardCandidato(d, c) {
    var i = c.instrutor || {};
    var nome = i.nome || 'Instrutor';
    var pendente = c.status === 'pendente';
    var acoes = [];
    if (pendente && d.pode_gerir_equipe) {
        var bAceitar = h('button', { type: 'button', class: 'gx-btn pequeno', 'aria-label': 'Aceitar ' + nome + ' na equipe', text: 'Aceitar' });
        var bRecusar = h('button', { type: 'button', class: 'gx-btn pequeno perigo', 'aria-label': 'Recusar a candidatura de ' + nome, text: 'Recusar' });
        var livres = d.vagas_instrutor_abertas || 0;
        bAceitar.disabled = livres < 1;
        bAceitar.addEventListener('click', async function() {
            var ultima = livres === 1;
            var ok = await Modal.confirmar({ titulo: 'Aceitar ' + nome + '?', confirmar: 'Aceitar na equipe',
                mensagem: nome + ' entra na equipe com as condições oferecidas.' + (ultima ? ' É a última vaga: as outras candidaturas pendentes são recusadas e avisadas.' : '') +
                    ((d.equipe || []).length ? '' : ' Como a equipe está vazia, ele vira o responsável pelo pódio.') });
            if (ok) acao(bAceitar, function() { return API.aceitarCandidatura(d.id, c.id); }, nome + ' entrou na equipe.');
        });
        bRecusar.addEventListener('click', async function() {
            var ok = await Modal.confirmar({ titulo: 'Recusar ' + nome + '?', perigo: true, confirmar: 'Recusar', mensagem: 'O instrutor é avisado. Ele pode se candidatar de novo enquanto houver vaga.' });
            if (ok) acao(bRecusar, function() { return API.recusarCandidatura(d.id, c.id); }, 'Candidatura recusada.');
        });
        acoes.push(bAceitar, bRecusar);
    }
    return h('article', { class: 'gx-card' + (c.status === 'escolhida' ? ' destaque' : ''), 'aria-label': 'Candidato ' + nome }, [
        h('div', { class: 'gx-linha' }, [
            h('div', { class: 'gx-ident' }, [
                h('div', { class: 'gx-avatar', 'aria-hidden': 'true', text: nome.charAt(0).toUpperCase() }),
                h('div', null, [
                    h('h3', { text: nome }),
                    h('div', { class: 'gx-atleta-sub', text: 'CREF ' + (i.cref || '-') + (i.especialidade ? ' · ' + i.especialidade : '') })
                ])
            ]),
            h('span', { class: 'gx-status ' + c.status, text: Gestao.ROTULO_CANDIDATURA[c.status] || c.status })
        ]),
        h('div', { class: 'gx-meta' }, [
            i.vinculado ? h('span', { class: 'gx-selo-ok', text: 'Vinculado à academia' }) : h('span', { class: 'gx-selo-esp', text: 'Autônomo' }),
            h('span', { text: 'Candidatou-se em ' + fmtDataHora(c.criado_em) })
        ]),
        c.mensagem ? h('div', { class: 'gx-msg-cand', text: c.mensagem }) : null,
        pendente && d.pode_gerir_equipe && !(d.vagas_instrutor_abertas > 0) ? h('p', { class: 'gx-ajuda', text: 'A equipe está completa: aumente o número de instrutores ou remova alguém para aceitar.' }) : null,
        acoes.length ? h('div', { class: 'gx-acoes' }, acoes) : null
    ]);
}

async function carregar() {
    Skeleton.cards(conteudo, 3, 120);
    try {
        var res = await Promise.all([API.buscarDesafio(idDesafio), API.listarCandidaturas(idDesafio),
            API.listarInstrutoresVinculados(1, 100).catch(function() { return null; })]);
        var d = res[0].data, lista = res[1].data || [];
        var vinculados = (res[2] && res[2].data && res[2].data.itens) || [];
        document.title = 'Equipe — ' + d.titulo + ' — GymScore';
        Skeleton.fim(conteudo);
        limpar(conteudo);
        conteudo.appendChild(resumoDesafio(d));
        conteudo.appendChild(secaoEquipe(d));
        if (d.pode_gerir_equipe && (d.vagas_instrutor_abertas || 0) > 0) conteudo.appendChild(secaoIndicar(d, vinculados));
        else if (d.status !== 'aberto') conteudo.appendChild(h('p', { class: 'gx-texto', text: 'A equipe só pode ser alterada com o desafio aberto. O responsável ainda pode ser trocado enquanto o desafio está em andamento.' }));

        var pendentes = lista.filter(function(c) { return c.status === 'pendente'; }).length;
        conteudo.appendChild(h('h2', { class: 'gx-secao', text: 'Candidaturas' + (lista.length ? ' (' + pendentes + (pendentes === 1 ? ' pendente' : ' pendentes') + ' de ' + lista.length + ')' : '') }));
        if (!lista.length) {
            conteudo.appendChild(estadoVazio({ icone: '⏳', titulo: 'Ainda não há candidaturas',
                texto: (d.vagas_instrutor_abertas || 0) > 0
                    ? 'Instrutores aprovados veem as vagas abertas em "Trabalhos" e se candidatam aceitando as suas condições. Você é avisado a cada candidatura.'
                    : 'A equipe está completa: não há vaga aberta para candidatura.',
                acao: { texto: 'Atualizar', onClick: carregar } }));
        } else {
            lista.forEach(function(c) { conteudo.appendChild(cardCandidato(d, c)); });
        }
        conteudo.appendChild(h('div', { class: 'gx-acoes' }, h('a', { class: 'gx-btn contorno', href: '/academia/podio?id=' + d.id }, 'Gerenciar desafio')));
    } catch (e) {
        Gestao.erro(conteudo, e, carregar);
    }
}

carregar();
renderBottomNav('/academia/desafios');
})();
