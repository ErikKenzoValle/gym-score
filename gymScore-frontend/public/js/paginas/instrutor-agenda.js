// Script da página instrutor-agenda.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
(async function () {
var user = await Gestao.exigirPerfil('instrutor');
if (!user) return; // redirecionado para o login/home
if (user.nome) document.getElementById('instNome').textContent = nomeCompleto(user);

var aba = location.pathname === '/instrutor/vagas' ? 'vagas' : (location.pathname === '/instrutor/ganhos' ? 'ganhos' : 'agenda');
Array.prototype.forEach.call(document.querySelectorAll('.gx-aba[data-aba]'), function(a) {
    if (a.dataset.aba === aba) a.setAttribute('aria-current', 'page');
});

// Convite de vínculo / desvínculo da academia (só na aba Agenda)
if (aba === 'agenda') Gestao.vinculo(document.getElementById('vinculoBox'));

var conteudo = document.getElementById('conteudo');
var maisBtn = document.getElementById('maisBtn');
var filtros = document.getElementById('filtros');
var LIMITE = 10;
var pagina = 1, carregados = 0;

// ─── Agenda ──────────────────────────────────────────────────────────────
var FILTROS_AGENDA = [['', 'Próximos'], ['encerrado', 'Encerrados'], ['cancelado', 'Cancelados']];
var filtroAgenda = '';

function desenharFiltros() {
    limpar(filtros);
    FILTROS_AGENDA.forEach(function(f) {
        filtros.appendChild(h('button', { type: 'button', class: 'gx-aba' + (f[0] === filtroAgenda ? ' ativa' : ''), 'aria-pressed': f[0] === filtroAgenda ? 'true' : 'false',
            onClick: function() { filtroAgenda = f[0]; desenharFiltros(); carregarAgenda(true); }, text: f[1] }));
    });
}

function acoesAgenda(d) {
    var a = [];
    if (d.status === 'aberto' || d.status === 'em_andamento') a.push({ texto: d.pode_iniciar ? 'Chamada e início' : 'Chamada', classe: 'solido', href: '/instrutor/checkin?id=' + d.id });
    if (d.pode_lancar_podio) a.push({ texto: 'Lançar pódio', classe: 'contorno', href: '/instrutor/podio?id=' + d.id });
    if (d.status === 'encerrado') a.push({ texto: 'Ver pódio', href: '/instrutor/podio?id=' + d.id });
    return a.map(function(x) { if (x.classe === 'solido') x.classe = ''; return x; });
}

function cartaoAgenda(d) {
    var card = Gestao.cardDesafio(d, acoesAgenda(d));
    var ganho = d.instrutor_ganho;
    if (ganho) {
        var meta = card.querySelectorAll('.gx-meta')[1];
        meta.appendChild(h('span', { text: (d.status === 'encerrado' ? 'Recebido: ' : 'Você recebe: ') + fmtBRL(ganho.total_previsto_centavos) }));
    }
    if (d.academia) card.querySelector('.gx-meta').insertBefore(h('span', { text: d.academia.nome_fantasia }), card.querySelector('.gx-meta').firstChild);
    return card;
}

async function carregarAgenda(recomeco) {
    filtros.hidden = false;
    if (recomeco) { pagina = 1; carregados = 0; Skeleton.cards(conteudo, 3, 130); maisBtn.hidden = true; }
    try {
        var resp = await API.agendaInstrutor(filtroAgenda, pagina, LIMITE);
        var dados = resp.data || {};
        var itens = dados.itens || [];
        if (recomeco) { Skeleton.fim(conteudo); limpar(conteudo); }
        carregados += itens.length;
        itens.forEach(function(d) { conteudo.appendChild(cartaoAgenda(d)); });
        if (!carregados) {
            conteudo.appendChild(estadoVazio(filtroAgenda ? {
                icone: '🗂️', titulo: 'Nada por aqui', texto: 'Nenhum desafio ' + (filtroAgenda === 'encerrado' ? 'encerrado' : 'cancelado') + ' na sua agenda.',
                acao: { texto: 'Ver próximos', onClick: function() { filtroAgenda = ''; desenharFiltros(); carregarAgenda(true); } }
            } : {
                icone: '📅', titulo: 'Sua agenda está vazia',
                texto: 'Você aparece aqui quando uma academia te designa ou escolhe seu lance numa vaga aberta.',
                acao: { texto: 'Ver vagas abertas', href: '/instrutor/vagas' }
            }));
        }
        maisBtn.hidden = carregados >= (dados.total || 0);
        maisBtn.disabled = false;
    } catch (e) {
        if (!recomeco) { pagina--; maisBtn.disabled = false; showToast(mensagemErro(e), 'error'); return; }
        Gestao.erro(conteudo, e, function() { carregarAgenda(true); });
    }
}

// ─── Vagas ───────────────────────────────────────────────────────────────
function textoPagamento(v) {
    var teto = fmtBRL(v.teto_centavos);
    if (v.modo_pagamento === 'percentual') return v.pct + '% das inscrições (sem lance)';
    if (v.modo_pagamento === 'fixo_percentual') return 'Seu lance, até ' + teto + ', + ' + v.pct + '% das inscrições';
    return 'Seu lance, até ' + teto;
}

function rotuloStatus(s) {
    return { pendente: 'Candidatura enviada', escolhida: 'Você foi escolhido', recusada: 'Não escolhido', retirada: 'Candidatura retirada' }[s] || s;
}

async function retirar(d) {
    var ok = await Modal.confirmar({ titulo: 'Retirar candidatura?', mensagem: 'Você sai da disputa por "' + d.titulo + '". Pode se candidatar de novo enquanto a vaga estiver aberta.', confirmar: 'Retirar', perigo: true });
    if (!ok) return;
    try {
        await API.retirarCandidatura(d.id);
        showToast('Candidatura retirada.', 'success');
        carregarVagas(true);
    } catch (e) { showToast(mensagemErro(e), 'error'); }
}

// Modal de lance: moeda mascarada (até o teto) e mensagem opcional
function abrirLance(d) {
    var v = d.vaga, minha = v.minha_candidatura;
    var semLance = v.modo_pagamento === 'percentual';
    var campo = h('input', { type: 'text', id: 'lanceCampo', class: 'gs-input', 'data-mask': 'moeda', 'data-max-centavos': String(v.teto_centavos), placeholder: '0,00' });
    var msg = h('textarea', { id: 'lanceMsg', class: 'gs-input', rows: '3', maxlength: '500', placeholder: 'Conte por que você é a pessoa certa (opcional)', style: { resize: 'vertical' } });
    var erro = h('p', { class: 'gs-msg error', role: 'alert' });
    Mask.aplicar(campo, 'moeda');
    if (minha && minha.status === 'pendente') { Mask.definirCentavos(campo, minha.lance_centavos); msg.value = minha.mensagem || ''; }

    var corpo = [
        h('p', { class: 'gx-texto', text: d.titulo + (d.academia ? ' · ' + d.academia.nome_fantasia : '') }),
        h('p', { class: 'gx-texto', style: { margin: '6px 0 12px' }, text: 'Pagamento: ' + textoPagamento(v) }),
        semLance ? null : h('div', { class: 'gs-field' }, [
            h('label', { class: 'gs-label', for: 'lanceCampo', text: 'Seu lance (R$), no máximo ' + fmtBRL(v.teto_centavos) }),
            h('div', { class: 'gx-prefixo' }, [h('span', { 'aria-hidden': 'true', text: 'R$' }), campo]),
            h('p', { class: 'gx-ajuda', text: 'Menor lance chama mais atenção, mas a academia escolhe quem quiser.' })
        ]),
        h('div', { class: 'gs-field' }, [h('label', { class: 'gs-label', for: 'lanceMsg', text: 'Mensagem' }), msg]),
        erro
    ];
    Modal.criar({
        titulo: minha && minha.status === 'pendente' ? 'Atualizar candidatura' : 'Candidatar-se à vaga', corpo: corpo,
        acoes: [
            { texto: 'Cancelar' },
            { texto: minha && minha.status === 'pendente' ? 'Salvar lance' : 'Enviar candidatura', classe: 'gs-btn-primary', onClick: async function(fechar) {
                erro.classList.remove('show');
                var lance = semLance ? 0 : Mask.centavos(campo);
                if (lance > v.teto_centavos) { erro.textContent = 'O lance passa do teto de ' + fmtBRL(v.teto_centavos) + '.'; erro.classList.add('show'); return; }
                try {
                    await API.candidatar(d.id, lance, msg.value.trim());
                    fechar();
                    showToast('Candidatura enviada!', 'success');
                    carregarVagas(true);
                } catch (e) { erro.textContent = mensagemErro(e); erro.classList.add('show'); }
            } }
        ]
    });
}

function cartaoVaga(d) {
    var v = d.vaga || {};
    var minha = v.minha_candidatura;
    var acoes = [];
    if (d.pode_candidatar) {
        acoes.push({ texto: minha && minha.status === 'pendente' ? 'Mudar lance' : 'Candidatar-se', classe: '', onClick: function() { abrirLance(d); } });
    }
    if (minha && minha.status === 'pendente') acoes.push({ texto: 'Retirar', classe: 'perigo', onClick: function() { retirar(d); } });
    return h('article', { class: 'gx-card', 'aria-label': d.titulo }, [
        h('div', { class: 'gx-linha' }, [h('h3', { text: d.titulo }), minha ? h('span', { class: 'gx-status ' + minha.status, text: rotuloStatus(minha.status) }) : h('span', { class: 'gx-status aberto', text: 'Vaga aberta' })]),
        h('div', { class: 'gx-meta' }, [
            d.academia ? h('span', { text: d.academia.nome_fantasia }) : null,
            h('span', { text: Gestao.quando(d.data_evento) }),
            d.local ? h('span', { text: d.local }) : null
        ]),
        Gestao.lista([
            ['Pagamento', textoPagamento(v)],
            ['Inscritos', d.participantes_count + ' de ' + d.vagas],
            minha && minha.status !== 'retirada' ? ['Seu lance', fmtBRL(minha.lance_centavos)] : null
        ]),
        acoes.length ? h('div', { class: 'gx-acoes' }, acoes.map(function(a) {
            return h('button', { type: 'button', class: 'gx-btn pequeno ' + (a.classe || ''), onClick: a.onClick, text: a.texto });
        })) : null
    ]);
}

async function carregarVagas(recomeco) {
    if (recomeco) { pagina = 1; carregados = 0; Skeleton.cards(conteudo, 3, 140); maisBtn.hidden = true; }
    try {
        var resp = await API.listarVagas(pagina, LIMITE);
        var dados = resp.data || {};
        var itens = dados.itens || [];
        if (recomeco) { Skeleton.fim(conteudo); limpar(conteudo); }
        carregados += itens.length;
        itens.forEach(function(d) { conteudo.appendChild(cartaoVaga(d)); });
        if (!carregados) {
            conteudo.appendChild(estadoVazio({ icone: '🔎', titulo: 'Nenhuma vaga aberta agora',
                texto: 'Quando uma academia abrir vaga de instrutor, ela aparece aqui. Você é avisado quando for escolhido.',
                acao: { texto: 'Atualizar', onClick: function() { carregarVagas(true); } } }));
        }
        maisBtn.hidden = carregados >= (dados.total || 0);
        maisBtn.disabled = false;
    } catch (e) {
        if (!recomeco) { pagina--; maisBtn.disabled = false; showToast(mensagemErro(e), 'error'); return; }
        Gestao.erro(conteudo, e, function() { carregarVagas(true); });
    }
}

// ─── Ganhos ──────────────────────────────────────────────────────────────
// Soma o instrutor_ganho dos desafios em que sou o designado: previstos (abertos/em andamento)
// e recebidos (encerrados). Percorre todas as páginas.
async function todas(status) {
    var itens = [], p = 1, total = 0;
    do {
        var dados = (await API.agendaInstrutor(status, p, 100)).data || {};
        itens = itens.concat(dados.itens || []);
        total = dados.total || 0;
        p++;
    } while (itens.length < total && p <= 20);
    return itens;
}
function soma(lista) {
    return lista.reduce(function(a, d) { return a + (d.instrutor_ganho ? d.instrutor_ganho.total_previsto_centavos : 0); }, 0);
}
function linhaGanho(d) {
    var g = d.instrutor_ganho || {};
    return h('div', { class: 'gx-atleta' }, [
        h('div', { class: 'gx-atleta-info' }, [
            h('div', { class: 'gx-atleta-nome', text: d.titulo }),
            h('div', { class: 'gx-atleta-sub', text: (d.academia ? d.academia.nome_fantasia + ' · ' : '') + Gestao.quando(d.data_evento) +
                (g.fixo_centavos ? ' · fixo ' + fmtBRL(g.fixo_centavos) : '') + (g.pct ? ' · ' + g.pct + '% (' + fmtBRL(g.pct_centavos) + ')' : '') })
        ]),
        h('span', { class: 'gx-premio', text: fmtBRL(g.total_previsto_centavos) })
    ]);
}
async function carregarGanhos() {
    Skeleton.cards(conteudo, 3, 90);
    try {
        var res = await Promise.all([todas('aberto'), todas('em_andamento'), todas('encerrado')]);
        var previstos = res[0].concat(res[1]), recebidos = res[2];
        Skeleton.fim(conteudo);
        limpar(conteudo);
        conteudo.appendChild(h('div', { class: 'gx-kpis' }, [
            h('div', { class: 'gx-kpi' }, [h('div', { class: 'gx-kpi-rotulo', text: 'A receber (previsto)' }), h('div', { class: 'gx-kpi-valor', text: fmtBRL(soma(previstos)) })]),
            h('div', { class: 'gx-kpi' }, [h('div', { class: 'gx-kpi-rotulo', text: 'Já recebido' }), h('div', { class: 'gx-kpi-valor', text: fmtBRL(soma(recebidos)) })])
        ]));
        conteudo.appendChild(h('p', { class: 'gx-texto', text: 'O previsto muda com o número de inscritos quando o pagamento tem percentual. O pagamento cai na sua carteira quando o pódio é lançado.' }));
        conteudo.appendChild(h('h2', { class: 'gx-secao', text: 'A receber (' + previstos.length + ')' }));
        if (!previstos.length) conteudo.appendChild(estadoVazio({ icone: '💸', titulo: 'Nada previsto ainda', texto: 'Candidate-se a vagas abertas para ter desafios na agenda.', acao: { texto: 'Ver vagas', href: '/instrutor/vagas' } }));
        else conteudo.appendChild(h('div', { class: 'gx-card' }, previstos.map(linhaGanho)));
        conteudo.appendChild(h('h2', { class: 'gx-secao', text: 'Recebidos (' + recebidos.length + ')' }));
        if (!recebidos.length) conteudo.appendChild(estadoVazio({ icone: '🧾', titulo: 'Nenhum pagamento recebido', texto: 'Os desafios encerrados aparecem aqui com o valor pago.', acao: { texto: 'Ver carteira', href: '/carteira' } }));
        else conteudo.appendChild(h('div', { class: 'gx-card' }, recebidos.map(linhaGanho)));
    } catch (e) {
        Gestao.erro(conteudo, e, carregarGanhos);
    }
}

maisBtn.addEventListener('click', function() { maisBtn.disabled = true; pagina++; (aba === 'vagas' ? carregarVagas : carregarAgenda)(false); });

if (aba === 'vagas') carregarVagas(true);
else if (aba === 'ganhos') carregarGanhos();
else { desenharFiltros(); carregarAgenda(true); }
renderBottomNav(location.pathname);
})();
