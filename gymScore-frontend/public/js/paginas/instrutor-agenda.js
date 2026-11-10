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
    var metas = card.querySelectorAll('.gx-meta');
    if (ganho) {
        metas[1].appendChild(h('span', { text: (d.status === 'encerrado' ? 'Recebido: ' : 'Você recebe: ') + fmtBRL(ganho.total_previsto_centavos) }));
    }
    if (d.academia) metas[0].insertBefore(h('span', { text: d.academia.nome_fantasia }), metas[0].firstChild);
    // Equipe com o responsável pelo pódio (rodada 5)
    var papel = d.voce_responsavel ? 'Você é o responsável: faz a chamada e lança o pódio.'
        : (d.voce_na_equipe ? 'Você está na equipe: faz a chamada; o pódio é do responsável.' : '');
    var bloco = h('details', { class: 'gx-detalhe-equipe' }, [
        h('summary', { text: 'Equipe (' + (d.equipe || []).length + ' de ' + (d.instrutores_vagas || 1) + ')' + (d.voce_responsavel ? ' · você é o responsável' : '') }),
        papel ? h('p', { class: 'gx-ajuda', text: papel }) : null,
        Gestao.equipe(d, { voce: user.id })
    ]);
    var barra = card.querySelector('.gs-progresso');
    card.insertBefore(bloco, barra ? barra.parentNode.nextSibling : null);
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
                texto: 'Você aparece aqui quando uma academia te indica para a equipe de um desafio ou aceita sua candidatura.',
                acao: { texto: 'Ver trabalhos oferecidos', href: '/instrutor/vagas' }
            }));
        }
        maisBtn.hidden = carregados >= (dados.total || 0);
        maisBtn.disabled = false;
    } catch (e) {
        if (!recomeco) { pagina--; maisBtn.disabled = false; showToast(mensagemErro(e), 'error'); return; }
        Gestao.erro(conteudo, e, function() { carregarAgenda(true); });
    }
}

// ─── Trabalhos oferecidos (rodada 5: sem lance; candidatar = aceitar as condições) ─────
function rotuloStatus(s) {
    return { pendente: 'Candidatura enviada', escolhida: 'Você está na equipe', recusada: 'Não aceita', retirada: 'Candidatura retirada', removida: 'Removido da equipe' }[s] || s;
}

function linhasCondicoes(d) {
    var v = d.vaga || {};
    return [
        ['Pagamento', Gestao.textoCondicoes(v.modo_pagamento, v.fixo_centavos, v.pct)],
        v.pct ? ['Previsão do percentual hoje', fmtBRL(v.pct_previsto_centavos) + ' (com ' + d.participantes_count + (d.participantes_count === 1 ? ' inscrito)' : ' inscritos)')] : null,
        ['Equipe', (d.equipe || []).length + ' de ' + (v.instrutores_vagas || d.instrutores_vagas || 1) + ' · ' + (v.vagas_abertas || 0) + ((v.vagas_abertas || 0) === 1 ? ' vaga aberta' : ' vagas abertas')],
        ['Atletas inscritos', d.participantes_count + ' de ' + d.vagas]
    ];
}

async function retirar(d) {
    var ok = await Modal.confirmar({ titulo: 'Retirar candidatura?', mensagem: 'Você desiste de "' + d.titulo + '". Pode se candidatar de novo enquanto houver vaga.', confirmar: 'Retirar', perigo: true });
    if (!ok) return;
    try {
        await API.retirarCandidatura(d.id);
        showToast('Candidatura retirada.', 'success');
        carregarVagas(true);
    } catch (e) { showToast(mensagemErro(e), 'error'); }
}

// Candidatura: mostra as condições, o aceite e uma mensagem opcional
function abrirCandidatura(d) {
    var v = d.vaga || {}, minha = v.minha_candidatura;
    var atualizando = minha && minha.status === 'pendente';
    var msg = h('textarea', { id: 'candMsg', class: 'gs-input gx-textarea', rows: '3', maxlength: '500', placeholder: 'Conte por que você é a pessoa certa (opcional)' });
    var erro = h('p', { class: 'gs-msg error', role: 'alert' });
    if (atualizando) msg.value = minha.mensagem || '';

    var corpo = [
        h('p', { class: 'gx-texto', text: d.titulo + (d.academia ? ' · ' + d.academia.nome_fantasia : '') }),
        h('p', { class: 'gx-texto', text: Gestao.horario(d) }),
        Gestao.lista(linhasCondicoes(d)),
        h('p', { class: 'gx-aceite', text: 'Ao se candidatar você aceita estas condições. Se a academia aceitar, você entra na equipe e faz a chamada no dia; o pódio é lançado pelo responsável que ela definir.' }),
        h('div', { class: 'gs-field' }, [h('label', { class: 'gs-label', for: 'candMsg', text: 'Mensagem para a academia' }), msg]),
        erro
    ];
    Modal.criar({
        titulo: atualizando ? 'Atualizar candidatura' : 'Candidatar-se ao trabalho', corpo: corpo,
        acoes: [
            { texto: 'Cancelar' },
            { texto: atualizando ? 'Salvar mensagem' : 'Aceitar condições e candidatar', classe: 'gs-btn-primary', onClick: async function(fechar) {
                erro.classList.remove('show');
                try {
                    await API.candidatar(d.id, msg.value.trim());
                    fechar();
                    showToast(atualizando ? 'Mensagem atualizada.' : 'Candidatura enviada!', 'success');
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
        acoes.push({ texto: minha && minha.status === 'pendente' ? 'Editar mensagem' : 'Candidatar-se', classe: '', onClick: function() { abrirCandidatura(d); } });
    }
    if (minha && minha.status === 'pendente') acoes.push({ texto: 'Retirar', classe: 'perigo', onClick: function() { retirar(d); } });
    var status = d.voce_na_equipe ? h('span', { class: 'gx-status escolhida', text: d.voce_responsavel ? 'Responsável' : 'Na equipe' })
        : (minha ? h('span', { class: 'gx-status ' + minha.status, text: rotuloStatus(minha.status) }) : h('span', { class: 'gx-status aberto', text: 'Vaga aberta' }));
    var end = Gestao.enderecoLinhas(d.endereco, d.local);
    return h('article', { class: 'gx-card', 'aria-label': d.titulo }, [
        h('div', { class: 'gx-linha' }, [h('h3', { text: d.titulo }), status]),
        h('div', { class: 'gx-meta' }, [
            d.academia ? h('span', { text: d.academia.nome_fantasia }) : null,
            h('span', { text: Gestao.horario(d) }),
            d.categoria ? h('span', { text: d.categoria }) : null
        ]),
        end.length ? h('address', { class: 'gx-endereco' }, end.map(function(l) { return h('span', { text: l }); })) : null,
        h('div', { class: 'gx-condicoes', text: Gestao.textoCondicoes(v.modo_pagamento, v.fixo_centavos, v.pct) }),
        Gestao.lista(linhasCondicoes(d).slice(1)),
        minha && minha.mensagem && minha.status === 'pendente' ? h('div', { class: 'gx-msg-cand', text: 'Sua mensagem: ' + minha.mensagem }) : null,
        acoes.length ? h('div', { class: 'gx-acoes' }, acoes.map(function(a) {
            return h('button', { type: 'button', class: 'gx-btn pequeno ' + (a.classe || ''), onClick: a.onClick, text: a.texto });
        })) : null
    ]);
}

async function carregarVagas(recomeco) {
    if (recomeco) { pagina = 1; carregados = 0; Skeleton.cards(conteudo, 3, 140); maisBtn.hidden = true; }
    try {
        var resp = await API.listarTrabalhos(pagina, LIMITE);
        var dados = resp.data || {};
        var itens = dados.itens || [];
        if (recomeco) { Skeleton.fim(conteudo); limpar(conteudo); }
        carregados += itens.length;
        itens.forEach(function(d) { conteudo.appendChild(cartaoVaga(d)); });
        if (!carregados) {
            conteudo.appendChild(estadoVazio({ icone: '🔎', titulo: 'Nenhum trabalho oferecido agora',
                texto: 'Quando uma academia abrir vaga na equipe de um desafio, ela aparece aqui com as condições. Você é avisado quando for aceito.',
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
// Soma o instrutor_ganho dos desafios em que estou na equipe: previstos (abertos/em andamento)
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
        if (!previstos.length) conteudo.appendChild(estadoVazio({ icone: '💸', titulo: 'Nada previsto ainda', texto: 'Candidate-se a trabalhos oferecidos para ter desafios na agenda.', acao: { texto: 'Ver trabalhos', href: '/instrutor/vagas' } }));
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
