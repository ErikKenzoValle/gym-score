// Script da página desafios-detalhado.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
if (!Auth.require()) throw new Error('unauthenticated');

var LIMITE = 12;
var FILTROS_MEUS = [
    { valor: '', rotulo: 'Todos' }, { valor: 'aberto', rotulo: 'Abertos' },
    { valor: 'em_andamento', rotulo: 'Em andamento' }, { valor: 'encerrado', rotulo: 'Encerrados' },
    { valor: 'cancelado', rotulo: 'Cancelados' }
];
var ORDEM_CATEGORIAS = ['geral', 'corrida', 'musculacao', 'crossfit', 'funcional', 'ciclismo', 'natacao', 'calistenia', 'luta', 'outros'];

var currentUser  = Auth.getUser() || {};
var aba          = 'explorar';   // 'explorar' | 'meus'
var filtroStatus = '';           // aba Meus
var filtroCat    = '';           // aba Explorar
var paginaAtual  = 1;
var totalLista   = 0;
var itens        = [];
var ordem        = 'data';
var userCoords   = null;
var listaSeq     = 0;
var detalhe      = null;
var categorias   = ORDEM_CATEGORIAS;
var buscaTimer   = null;

var $ = function(id) { return document.getElementById(id); };

// ── Filtros ───────────────────────────────────────────────────────────────
function chip(rotulo, ativo, aoClicar) {
    return h('button', { type: 'button', class: 'chip-categoria' + (ativo ? ' ativo' : ''), 'aria-pressed': ativo ? 'true' : 'false',
        text: rotulo, onClick: aoClicar });
}
function renderFiltros() {
    var cont = limpar($('filtros'));
    if (aba === 'meus') {
        FILTROS_MEUS.forEach(function(f) {
            cont.appendChild(chip(f.rotulo, f.valor === filtroStatus, function() { filtroStatus = f.valor; renderFiltros(); carregarLista(true); }));
        });
        return;
    }
    cont.appendChild(chip('Todas', filtroCat === '', function() { filtroCat = ''; renderFiltros(); carregarLista(true); }));
    categorias.forEach(function(c) {
        cont.appendChild(chip(rotuloCategoria(c), filtroCat === c, function() { filtroCat = c; renderFiltros(); carregarLista(true); }));
    });
}

function selecionarAba(nova, foco) {
    aba = nova;
    ['explorar', 'meus'].forEach(function(k) {
        var t = $(k === 'explorar' ? 'tabExplorar' : 'tabMeus');
        t.classList.toggle('active', k === nova);
        t.setAttribute('aria-selected', k === nova ? 'true' : 'false');
        t.tabIndex = k === nova ? 0 : -1;
        if (k === nova && foco) t.focus();
    });
    $('painelLista').setAttribute('aria-labelledby', nova === 'explorar' ? 'tabExplorar' : 'tabMeus');
    renderFiltros();
    carregarLista(true);
}

// ── Lista ─────────────────────────────────────────────────────────────────
function filtrosAtuais() {
    var f = { pagina: paginaAtual, limite: LIMITE, q: $('buscaInput').value.trim() };
    if (aba === 'meus') f.status = filtroStatus;
    else f.categoria = filtroCat;
    return f;
}

async function carregarLista(reiniciar) {
    var seq = ++listaSeq;
    if (reiniciar) {
        paginaAtual = 1;
        itens = [];
        limpar($('grade'));
        limpar($('estado'));
        $('listaMais').hidden = true;
        Skeleton.cards($('grade'), 3, 150);
    } else {
        paginaAtual++;
    }
    try {
        var resp = await (aba === 'meus' ? API.listarMeusDesafios : API.listarDesafios)(filtrosAtuais());
        if (seq !== listaSeq) return;
        var dados = resp.data || {};
        itens = itens.concat(dados.itens || []);
        totalLista = dados.total || 0;
        renderLista();
    } catch (err) {
        if (seq !== listaSeq) return;
        if (!reiniciar) paginaAtual--;
        limpar($('grade'));
        $('listaMais').hidden = true;
        limpar($('estado')).appendChild(estadoVazio({ erro: true, titulo: 'Não foi possível carregar os desafios',
            texto: mensagemErro(err), acao: { texto: 'Tentar de novo', onClick: function() { carregarLista(true); } } }));
    }
}

function distanciaKm(lat1, lon1, lat2, lon2) {
    var R = 6371, rad = function(d) { return d * Math.PI / 180; };
    var dLat = rad(lat2 - lat1), dLon = rad(lon2 - lon1);
    var a = Math.sin(dLat / 2) * Math.sin(dLat / 2) + Math.cos(rad(lat1)) * Math.cos(rad(lat2)) * Math.sin(dLon / 2) * Math.sin(dLon / 2);
    return R * 2 * Math.atan2(Math.sqrt(a), Math.sqrt(1 - a));
}
function distDesafio(c) {
    if (!userCoords || c.latitude == null || c.longitude == null) return Infinity;
    return distanciaKm(userCoords.lat, userCoords.lng, Number(c.latitude), Number(c.longitude));
}

function ordenados() {
    var l = itens.slice();
    if (ordem === 'valor_asc') l.sort(function(a, b) { return a.valor_entrada_centavos - b.valor_entrada_centavos; });
    else if (ordem === 'valor_desc') l.sort(function(a, b) { return b.valor_entrada_centavos - a.valor_entrada_centavos; });
    else if (ordem === 'proximidade') l.sort(function(a, b) { return distDesafio(a) - distDesafio(b); });
    return l;
}

function renderLista() {
    var grade = limpar($('grade'));
    var estado = limpar($('estado'));
    $('ordenacao').hidden = itens.length < 2;
    $('listaMais').hidden = itens.length >= totalLista;
    if (itens.length === 0) {
        estado.appendChild(vazioLista());
        return;
    }
    ordenados().forEach(function(c) { grade.appendChild(cartaoDesafio(c)); });
}

// Vazio com próximo passo, diferente por contexto (busca/filtro x lista realmente vazia)
function vazioLista() {
    var filtrando = $('buscaInput').value.trim() || (aba === 'meus' ? filtroStatus : filtroCat);
    if (filtrando) {
        return estadoVazio({ icone: '🔎', titulo: 'Nenhum desafio com esses filtros', texto: 'Tente outra busca ou limpe os filtros.',
            acao: { texto: 'Limpar filtros', onClick: limparFiltros } });
    }
    if (aba === 'meus') {
        return estadoVazio({ icone: '🏆', titulo: 'Você ainda não está em nenhum desafio',
            texto: 'Escolha um desafio aberto, pague a entrada e garanta seu ingresso.',
            acao: { texto: 'Explorar desafios', onClick: function() { selecionarAba('explorar', true); } } });
    }
    return estadoVazio({ icone: '📅', titulo: 'Nenhum desafio aberto agora',
        texto: 'As academias publicam novos desafios com frequência. Volte em breve ou treine enquanto isso.',
        acao: { texto: 'Ir para os treinos', href: '/treinos' } });
}

function limparFiltros() {
    $('buscaInput').value = '';
    $('limparBusca').hidden = true;
    filtroStatus = ''; filtroCat = '';
    renderFiltros();
    carregarLista(true);
}

// ── Detalhe ───────────────────────────────────────────────────────────────
function urlDesafio(id) { return '/desafios?id=' + encodeURIComponent(id); }

async function abrirDetalhe(id) {
    var box = limpar($('detalheConteudo'));
    box.appendChild(h('h2', { id: 'detalheTitulo', text: 'Carregando...' }));
    Skeleton.cards(box, 2, 80);
    Modal.abrir($('detalheModal'), { aoFechar: function() {
        detalhe = null;
        if (new URLSearchParams(location.search).get('id')) history.replaceState(null, '', '/desafios');
    } });
    await recarregarDetalhe(id, false);
}

async function recarregarDetalhe(id, manterFoco) {
    // estado de moderação (suspensão) e modo atleta/instrutor confirmados antes de montar as ações
    try { currentUser = (await Auth.verificar()) || currentUser; } catch (e) {}
    try {
        detalhe = (await API.buscarDesafio(id)).data;
    } catch (err) {
        if (err.status === 403 || err.status === 404) {
            // Sem acesso (não é participante e o desafio não está mais aberto): aviso e mantém a lista
            Modal.fechar($('detalheModal'));
            limpar($('aviso')).appendChild(h('p', { class: 'det-erro', role: 'alert',
                text: 'Este desafio não está mais aberto para inscrições ou você não tem acesso a ele.' }));
            showToast('Este desafio não está mais aberto para inscrições.', 'error');
            return;
        }
        var box = limpar($('detalheConteudo'));
        box.appendChild(h('h2', { id: 'detalheTitulo', text: 'Desafio indisponível' }));
        box.appendChild(estadoVazio({ erro: true, titulo: 'Não foi possível abrir este desafio', texto: mensagemErro(err) }));
        box.appendChild(h('div', { class: 'det-acoes' }, h('button', { type: 'button', class: 'gs-btn gs-btn-secondary', text: 'Fechar',
            onClick: function() { Modal.fechar($('detalheModal')); } })));
        return;
    }
    renderDetalhe(detalhe);
    if (manterFoco) { var t = $('detalheTitulo'); if (t) t.focus(); }
}

function resumoItem(rotulo, valor, largo) {
    return h('div', { class: largo ? 'largo' : null }, [h('small', { text: rotulo }), h('strong', { text: valor })]);
}

// Executa uma ação do desafio: bloqueia o botão, mostra erro inline, recarrega lista e detalhe
async function executar(btn, fn, msgOk) {
    var erro = $('detalheErro');
    if (erro) erro.textContent = '';
    if (btn) btn.disabled = true;
    try {
        await fn();
        if (msgOk) showToast(msgOk, 'success');
        Auth.refresh();
        carregarLista(true);
        if (detalhe) await recarregarDetalhe(detalhe.id, true);
    } catch (err) {
        if (erro) erro.textContent = mensagemErro(err);
        else showToast(mensagemErro(err), 'error');
        if (btn) btn.disabled = false;
    }
}

// A conta logada é o instrutor designado deste desafio (instrutor.id_usuario)
function euSouOInstrutor(d) {
    return !!(d.instrutor && d.instrutor.id_usuario && d.instrutor.id_usuario === currentUser.id);
}

function botao(texto, fn, classe, aria) {
    var b = h('button', { type: 'button', class: 'gs-btn ' + (classe || 'gs-btn-primary'), 'aria-label': aria || null, text: texto });
    b.addEventListener('click', function() { fn(b); });
    return b;
}

function renderDetalhe(d) {
    var box = limpar($('detalheConteudo'));
    var parts = d.participantes || [];
    var lotado = d.vagas > 0 && d.participantes_count >= d.vagas;
    var pct = d.vagas > 0 ? Math.min(100, Math.round(d.participantes_count / d.vagas * 100)) : 0;

    box.appendChild(h('div', { class: 'det-topo' }, [
        h('h2', { id: 'detalheTitulo', tabindex: '-1', text: d.titulo }),
        h('div', { class: 'det-topo-acoes' }, [
            h('button', { type: 'button', class: 'det-icone', text: 'Compartilhar', 'aria-label': 'Compartilhar este desafio',
                onClick: function() { compartilharLink(urlDesafio(d.id), d.titulo, 'Veja este desafio no GymScore'); } }),
            h('button', { type: 'button', class: 'gs-modal-x', 'aria-label': 'Fechar detalhes do desafio', text: '×',
                onClick: function() { Modal.fechar($('detalheModal')); } })
        ])
    ]));
    box.appendChild(h('div', { class: 'det-chips' }, [
        badgeStatus(d.status),
        d.categoria ? h('span', { class: 'chip-tag', text: rotuloCategoria(d.categoria) }) : null,
        d.voce_participa ? h('span', { class: 'tag-participa', text: 'Você está inscrito' }) : null
    ]));
    box.appendChild(linhaTempo(d.status));
    var avisoMod = avisoModeracao(d);
    if (avisoMod) box.appendChild(avisoMod);
    if (d.descricao) box.appendChild(h('p', { class: 'det-desc', text: d.descricao }));

    var contagem = d.status === 'aberto' && d.data_evento ? ' (' + fmtContagem(d.data_evento) + ')' : '';
    box.appendChild(h('div', { class: 'det-resumo' }, [
        resumoItem('Academia', d.academia ? d.academia.nome_fantasia : 'Não informada'),
        resumoItem('Instrutor', d.instrutor ? d.instrutor.nome : (d.vaga_instrutor_aberta ? 'Vaga em aberto' : 'A definir')),
        resumoItem('Quando', (d.data_evento ? fmtDataHora(d.data_evento) : 'A definir') + contagem, true),
        resumoItem('Local', d.local || 'A definir', true),
        resumoItem('Entrada', d.valor_entrada_centavos > 0 ? fmtBRL(d.valor_entrada_centavos) : 'Grátis'),
        resumoItem('Pote das inscrições', fmtBRL(d.pote_centavos)),
        h('div', { class: 'largo' }, [
            h('small', { text: 'Vagas ' + d.participantes_count + '/' + d.vagas + (lotado ? ' (lotado)' : '') }),
            h('div', { class: 'gs-progresso', style: { marginTop: '6px' }, role: 'progressbar', 'aria-label': 'Vagas preenchidas',
                'aria-valuemin': '0', 'aria-valuemax': String(d.vagas), 'aria-valuenow': String(d.participantes_count) },
                h('span', { style: { width: pct + '%' } }))
        ])
    ]));

    box.appendChild(renderPremiacao(d));
    box.appendChild(h('p', { id: 'detalheErro', class: 'det-erro', role: 'alert' }));
    renderAcoes(box, d);

    if (d.voce_participa && d.seu_codigo_ingresso && d.status !== 'cancelado') {
        box.appendChild(h('div', { class: 'det-secao' }, [
            h('h3', { text: 'Seu ingresso' }),
            h('p', { class: 'det-nota', text: d.voce_presente ? 'Presença confirmada pelo instrutor.' : 'Mostre o QR do ingresso ao instrutor para confirmar sua presença.' }),
            h('div', { class: 'det-acoes' }, h('a', { class: 'gs-btn gs-btn-secondary', href: '/ingressos', text: 'Abrir meus ingressos' }))
        ]));
    }
    if (d.status === 'encerrado') box.appendChild(renderPodio(d));
    if (d.status === 'cancelado') {
        box.appendChild(h('p', { class: 'det-nota', text: 'Este desafio foi cancelado. As entradas foram devolvidas à carteira de cada inscrito.' }));
    }
    if (d.pode_avaliar) box.appendChild(renderAvaliar(d));
    renderParticipantes(box, d, parts);
}

// Resultado anulado pela moderação / prêmios na janela de contestação
function avisoModeracao(d) {
    if (d.resultado_anulado) {
        var txt = d.resultado_anulacao_modo === 'relancar_podio'
            ? 'O resultado deste desafio foi anulado pela moderação' + (d.resultado_anulado_em ? ' em ' + fmtDataHora(d.resultado_anulado_em) : '') + '. A academia vai lançar um novo pódio.'
            : 'O resultado deste desafio foi anulado pela moderação' + (d.resultado_anulado_em ? ' em ' + fmtDataHora(d.resultado_anulado_em) : '') + '. As inscrições foram devolvidas integralmente.';
        return h('p', { class: 'md-nota erro', role: 'status' }, [h('strong', { text: 'Resultado anulado. ' }), txt]);
    }
    if (d.status === 'encerrado' && d.pagamentos_retidos) {
        return h('p', { class: 'md-nota alerta', text: 'Os prêmios deste desafio estão retidos: há uma contestação do resultado em análise.' });
    }
    if (d.status === 'encerrado' && d.pagamentos_liberam_em) {
        return h('p', { class: 'md-nota', text: 'Prêmios liberam em ' + fmtDataHora(d.pagamentos_liberam_em) + ', ao fim da janela de contestação.' });
    }
    return null;
}

function linhaTempo(status) {
    if (status === 'cancelado') {
        return h('ol', { class: 'gs-linha-tempo', 'aria-label': 'Etapas do desafio' }, [
            h('li', { class: 'feito', text: 'Aberto' }), h('li', { class: 'cancelado atual', text: 'Cancelado', 'aria-current': 'step' })
        ]);
    }
    var etapas = [['aberto', 'Inscrições'], ['em_andamento', 'Em andamento'], ['encerrado', 'Encerrado']];
    var idx = etapas.map(function(e) { return e[0]; }).indexOf(status);
    return h('ol', { class: 'gs-linha-tempo', 'aria-label': 'Etapas do desafio' }, etapas.map(function(e, i) {
        return h('li', { class: i < idx ? 'feito' : (i === idx ? 'atual' : null), 'aria-current': i === idx ? 'step' : null, text: e[1] });
    }));
}

function renderPremiacao(d) {
    var pos = premiosPorPosicao(d);
    var secao = h('div', { class: 'det-secao' }, h('h3', { text: 'Premiação' }));
    if (!pos.length) {
        secao.appendChild(h('p', { class: 'det-nota', text: 'Premiação a definir.' }));
        return secao;
    }
    var ul = h('ul', { class: 'premios' });
    pos.forEach(function(v, i) {
        ul.appendChild(h('li', null, [h('span', { text: (i + 1) + 'º lugar' }), h('strong', { text: fmtBRL(v) })]));
    });
    ul.appendChild(h('li', null, [h('span', { text: 'Prêmio total' }), h('strong', { class: 'total', text: fmtBRL(premioTotalCentavos(d)) })]));
    secao.appendChild(ul);
    if (d.status === 'aberto') {
        secao.appendChild(h('p', { class: 'det-nota', text: 'Valores previstos: podem crescer a cada nova inscrição. Só quem estiver presente concorre ao pódio.' }));
    }
    return secao;
}

// ── Ações do atleta (flags vêm prontas do backend) ───────────────────────
function renderAcoes(box, d) {
    var acoes = h('div', { class: 'det-acoes' });
    var suspenso = Moderacao.suspensoAte();
    if (d.pode_entrar && suspenso) {
        // Q2: suspenso não entra em desafio (o backend responde 403 conta_suspensa)
        box.appendChild(h('p', { class: 'md-nota alerta', text: 'Sua conta está suspensa até ' + fmtDataHora(suspenso) + ': não é possível entrar em desafios até lá.' }));
        acoes.appendChild(h('button', { type: 'button', class: 'gs-btn gs-btn-primary', disabled: true, text: 'Entrar (conta suspensa)' }));
    } else if (d.pode_entrar) {
        acoes.appendChild(botao('Entrar (' + (d.valor_entrada_centavos > 0 ? fmtBRL(d.valor_entrada_centavos) : 'grátis') + ')',
            function(b) { entrar(b, d); }, 'gs-btn-primary'));
    } else if (!d.pode_entrar && d.motivo_bloqueio_entrada) {
        // F4-4: conflito de interesse (B4-8) — sem "Entrar"; o 409 do backend continua sendo a garantia
        box.appendChild(h('p', { class: 'md-nota alerta', text: d.motivo_bloqueio_entrada }));
    } else if (d.status === 'aberto' && !d.voce_participa && Modo.atual(currentUser) === 'atleta') {
        var motivo = d.vagas > 0 && d.participantes_count >= d.vagas ? 'Vagas esgotadas.' : 'As inscrições deste desafio estão fechadas.';
        box.appendChild(h('p', { class: 'det-nota', text: motivo }));
    } else if (d.status === 'aberto' && !d.voce_participa && Modo.atual(currentUser) === 'instrutor' && !(d.pode_iniciar || d.pode_marcar_presenca || euSouOInstrutor(d))) {
        // Rodada 4: no modo instrutor não há inscrição; a troca para o modo atleta é imediata (sem deslogar)
        box.appendChild(h('p', { class: 'det-nota', text: 'Você está no modo instrutor. Para se inscrever como atleta, troque para o modo atleta.' }));
        acoes.appendChild(botao('Trocar para o modo atleta', async function(b) {
            btnLoading(b, true);
            try { await Modo.trocar('atleta', { destino: location.pathname + location.search }); }
            catch (e) { btnLoading(b, false); showToast(mensagemErro(e), 'error'); }
        }, 'gs-btn-secondary'));
    }
    if (d.pode_sair) acoes.appendChild(botao('Sair do desafio', function(b) { sair(b, d); }, 'gs-btn-secondary'));
    if (acoes.firstChild) box.appendChild(acoes);
    if (d.status === 'em_andamento' && d.voce_participa) {
        box.appendChild(h('p', { class: 'det-nota', text: d.voce_presente
            ? 'Você está presente. O instrutor vai lançar o pódio ao final.'
            : 'O desafio começou. Apresente seu ingresso ao instrutor para ter presença confirmada.' }));
    }
    if (d.pode_sair && d.vagas > d.participantes_count) renderConvidar(box, d);
    // Denunciar: participante (fraude, pódio, presença, suborno) — o backend confere o vínculo
    if (d.voce_participa && d.status !== 'aberto') {
        box.appendChild(h('div', { class: 'det-acoes' }, h('a', { class: 'gs-btn gs-btn-secondary', href: '/denunciar?desafio=' + encodeURIComponent(d.id),
            text: 'Denunciar algo neste desafio' })));
    }
}

async function entrar(btn, d) {
    var entrada = d.valor_entrada_centavos || 0;
    var saldo = (Auth.getUser() || {}).saldo_centavos || 0;
    try { saldo = (await API.carteira()).data.saldo_centavos; } catch (e) {}
    if (saldo < entrada) {
        var volta = encodeURIComponent(urlDesafio(d.id));
        Modal.criar({
            titulo: 'Saldo insuficiente', alerta: true,
            corpo: [
                h('p', { class: 'gs-modal-msg', text: 'Faltam ' + fmtBRL(entrada - saldo) + ' para pagar a entrada deste desafio.' }),
                h('dl', { class: 'gs-modal-linhas' }, [
                    h('div', null, [h('dt', { text: 'Entrada' }), h('dd', { text: fmtBRL(entrada) })]),
                    h('div', null, [h('dt', { text: 'Seu saldo' }), h('dd', { text: fmtBRL(saldo) })])
                ])
            ],
            acoes: [
                { texto: 'Voltar' },
                { texto: 'Depositar saldo', classe: 'gs-btn-primary', onClick: function() { window.location.href = '/depositar?voltar=' + volta; } }
            ]
        });
        return;
    }
    var linhas = entrada > 0
        ? [['Entrada (debitada agora)', fmtBRL(entrada)], ['Saldo atual', fmtBRL(saldo)], ['Saldo após entrar', fmtBRL(saldo - entrada)]]
        : [['Entrada', 'Grátis']];
    var ok = await Modal.confirmar({
        titulo: 'Entrar no desafio?',
        mensagem: entrada > 0 ? 'Se você sair ou o desafio for cancelado antes de começar, a entrada é devolvida.' : 'Inscrição gratuita.',
        linhas: linhas, confirmar: entrada > 0 ? 'Pagar e entrar' : 'Entrar', cancelar: 'Voltar'
    });
    if (ok) executar(btn, function() { return API.entrarDesafio(d.id); }, 'Você entrou no desafio!');
}

async function sair(btn, d) {
    var entrada = d.valor_entrada_centavos || 0;
    var ok = await Modal.confirmar({
        titulo: 'Sair do desafio?',
        mensagem: entrada > 0 ? 'Sua entrada será devolvida à carteira e seu ingresso será descartado.' : 'Seu ingresso será descartado.',
        linhas: entrada > 0 ? [['Valor devolvido', fmtBRL(entrada)]] : [],
        confirmar: 'Sair', cancelar: 'Continuar no desafio', perigo: true
    });
    if (ok) executar(btn, function() { return API.sairDesafio(d.id); }, entrada > 0 ? 'Você saiu. Entrada devolvida.' : 'Você saiu do desafio.');
}

// Convite a amigos (carregado sob demanda)
function renderConvidar(box, d) {
    var lista = h('div', { id: 'convitesLista' });
    var abrir = botao('Convidar amigos', async function(b) {
        b.disabled = true;
        limpar(lista).appendChild(h('p', { class: 'det-nota', text: 'Carregando amigos...' }));
        try {
            var jaNoDesafio = {};
            (d.participantes || []).forEach(function(p) { jaNoDesafio[p.id] = true; });
            var amigos = ((await API.listarAmigos()).data || []).filter(function(a) { return a.status === 'aceita' && !jaNoDesafio[a.id]; });
            limpar(lista);
            if (amigos.length === 0) {
                lista.appendChild(estadoVazio({ titulo: 'Nenhum amigo para convidar', texto: 'Adicione amigos para chamá-los para o desafio.',
                    acao: { texto: 'Ir para amigos', href: '/amigos' } }));
                return;
            }
            amigos.forEach(function(a) {
                var nome = nomeCompleto(a);
                lista.appendChild(h('div', { class: 'convite-linha' }, [
                    h('span', { text: nome }),
                    botao('Convidar', async function(bb) {
                        bb.disabled = true;
                        try {
                            await API.convidarAmigo(d.id, a.id);
                            bb.textContent = 'Convidado';
                            showToast('Convite enviado para ' + nome + '.', 'success');
                        } catch (err) {
                            bb.disabled = false;
                            showToast(mensagemErro(err), 'error');
                        }
                    }, 'gs-btn-secondary', 'Convidar ' + nome)
                ]));
            });
        } catch (err) {
            limpar(lista).appendChild(h('p', { class: 'det-erro', text: mensagemErro(err) }));
            b.disabled = false;
        }
    }, 'gs-btn-secondary');
    box.appendChild(h('div', { class: 'det-secao' }, [abrir, lista]));
}

// ── Pódio e avaliação ─────────────────────────────────────────────────────
function renderPodio(d) {
    var secao = h('div', { class: 'det-secao' }, h('h3', { text: 'Pódio' }));
    var podio = d.podio || [];
    if (!podio.length) {
        secao.appendChild(h('p', { class: 'det-nota', text: 'O pódio ainda não foi publicado.' }));
        return secao;
    }
    var ul = h('ol', { class: 'podio-lista' });
    podio.forEach(function(p) {
        var meu = p.id_participante === currentUser.id;
        ul.appendChild(h('li', null, [
            h('span', { class: 'pos', text: p.posicao + 'º' }),
            h('span', null, [nomeCompleto(p) + (meu ? ' (você)' : ''), p.marca ? h('span', { class: 'marca', text: 'Marca: ' + p.marca }) : null]),
            h('strong', { text: p.premio_centavos > 0 ? fmtBRL(p.premio_centavos) : '-' })
        ]));
    });
    secao.appendChild(ul);
    var meuPremio = podio.filter(function(p) { return p.id_participante === currentUser.id; })[0];
    if (meuPremio && meuPremio.premio_centavos > 0) {
        var txtPremio = d.pagamentos_retidos ? 'Seu prêmio de ' + fmtBRL(meuPremio.premio_centavos) + ' está retido até a contestação ser julgada.'
            : d.pagamentos_liberam_em ? 'Seu prêmio de ' + fmtBRL(meuPremio.premio_centavos) + ' entra na carteira em ' + fmtDataHora(d.pagamentos_liberam_em) + '.'
            : 'Você recebeu ' + fmtBRL(meuPremio.premio_centavos) + ' na sua carteira.';
        secao.appendChild(h('p', { class: 'det-ok', text: txtPremio }));
    }
    else if (d.voce_participa) secao.appendChild(h('p', { class: 'det-nota', text: 'Você não ficou no pódio desta vez.' }));
    return secao;
}

function renderAvaliar(d) {
    var notas = h('div', { class: 'avaliar-notas', role: 'group', 'aria-label': 'Avalie o desafio de 1 a 5' });
    [1, 2, 3, 4, 5].forEach(function(n) {
        notas.appendChild(botao(String(n), function(b) {
            executar(b, function() { return API.avaliarDesafio(d.id, n); }, 'Obrigado pela avaliação!');
        }, 'gs-btn-secondary', 'Nota ' + n + ' de 5'));
    });
    return h('div', { class: 'det-secao' }, [h('h3', { text: 'Como foi o desafio?' }), notas]);
}

// ── Participantes ─────────────────────────────────────────────────────────
function renderParticipantes(box, d, parts) {
    var secao = h('div', { class: 'det-secao' }, h('h3', { text: 'Participantes (' + d.participantes_count + '/' + d.vagas + ')' }));
    if (parts.length === 0) {
        secao.appendChild(h('p', { class: 'det-nota', text: 'Ninguém entrou ainda. Seja o primeiro!' }));
    } else {
        var ul = h('ul', { class: 'part-list' });
        parts.forEach(function(p) {
            var meta = [];
            if (p.premio_centavos > 0) meta.push('prêmio ' + fmtBRL(p.premio_centavos));
            ul.appendChild(h('li', { class: 'part-item' }, [
                h('span', null, [nomeCompleto(p) + (p.id === currentUser.id ? ' (você)' : ''), p.vencedor ? ' - venceu' : '']),
                h('span', { class: 'part-meta' }, [p.presente ? h('span', { class: 'tag-presente', text: 'presente' }) : null, meta.length ? ' ' + meta.join(' · ') : ''])
            ]));
        });
        secao.appendChild(ul);
    }
    box.appendChild(secao);
}

// ── Eventos ───────────────────────────────────────────────────────────────
$('buscaInput').addEventListener('input', function() {
    $('limparBusca').hidden = !this.value;
    clearTimeout(buscaTimer);
    buscaTimer = setTimeout(function() { carregarLista(true); }, 350);
});
$('limparBusca').addEventListener('click', function() {
    $('buscaInput').value = '';
    this.hidden = true;
    $('buscaInput').focus();
    carregarLista(true);
});
$('btnMais').addEventListener('click', function() { carregarLista(false); });
$('tabExplorar').addEventListener('click', function() { selecionarAba('explorar'); });
$('tabMeus').addEventListener('click', function() { selecionarAba('meus'); });
// Setas esquerda/direita entre as abas
document.querySelector('[role="tablist"]').addEventListener('keydown', function(e) {
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
    selecionarAba(aba === 'explorar' ? 'meus' : 'explorar', true);
});
$('ordem').addEventListener('change', function() {
    ordem = this.value;
    if (ordem === 'proximidade' && !userCoords) {
        if (!navigator.geolocation) { showToast('Geolocalização não suportada neste navegador.', 'error'); return; }
        showToast('Obtendo sua localização...', '');
        navigator.geolocation.getCurrentPosition(function(pos) {
            userCoords = { lat: pos.coords.latitude, lng: pos.coords.longitude };
            renderLista();
        }, function() { showToast('Não foi possível obter sua localização.', 'error'); });
    }
    renderLista();
});

// Categorias reais do backend (a lista fixa acima é só o fallback)
API.categoriasDesafio().then(function(r) {
    if (r.data && r.data.length) { categorias = r.data; if (aba === 'explorar') renderFiltros(); }
}).catch(function() {});

renderBottomNav('/desafios');
renderFiltros();
carregarLista(true);

// /desafios?id=N (links de notificação e de compartilhamento) e /desafios?aba=meus
var params = new URLSearchParams(location.search);
if (params.get('aba') === 'meus') selecionarAba('meus');
var idDireto = parseInt(params.get('id'), 10);
if (idDireto > 0) abrirDetalhe(idDireto);
