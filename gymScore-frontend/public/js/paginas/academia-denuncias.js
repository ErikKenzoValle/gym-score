// Denúncias da jurisdição da academia (suborno de instrutor em desafios dela): fila com prazo,
// detalhe sem denunciante, imagens pela rota autenticada, assumir / decidir (com advertência ou
// bloqueio na academia) / encaminhar ao admin. Aba "Punições aplicadas" (escopo da academia).
(async function() {
var user = await Gestao.exigirPerfil('academia');
if (!user) return;

var $ = function(id) { return document.getElementById(id); };
var LIMITE = 10;
var aba = 'fila', filtro = '', pagina = 1, total = 0, ul = null;
var FILTROS = [['', 'Todas'], ['aberta', 'Abertas'], ['em_analise', 'Em análise'], ['procedente', 'Procedentes'], ['improcedente', 'Improcedentes'], ['arquivada', 'Arquivadas']];
var FILTROS_PUN = [['true', 'Vigentes'], ['', 'Todas']];
var filtroPun = 'true';

function emAberto(d) { return d.status === 'aberta' || d.status === 'em_analise'; }

// "Prazo: 2d 4h" / "Prazo vencido" (o job sobe a denúncia ao admin)
function prazo(d) {
    if (!d.prazo_academia_ate || !emAberto(d)) return null;
    var ms = new Date(d.prazo_academia_ate).getTime() - Date.now();
    if (isNaN(ms)) return null;
    if (ms <= 0) return h('span', { class: 'md-prazo vencido', text: 'Prazo vencido: vai para a equipe GymScore' });
    var horas = Math.floor(ms / 3600000), dias = Math.floor(horas / 24);
    var txt = dias > 0 ? dias + 'd ' + (horas % 24) + 'h' : horas > 0 ? horas + 'h ' + Math.floor(ms % 3600000 / 60000) + 'min' : Math.max(1, Math.floor(ms / 60000)) + ' min';
    return h('span', { class: 'md-prazo', title: fmtDataHora(d.prazo_academia_ate), text: 'Prazo: ' + txt + ' (até ' + fmtDataHora(d.prazo_academia_ate) + ')' });
}

function renderFiltros() {
    var box = limpar($('filtros'));
    (aba === 'fila' ? FILTROS : FILTROS_PUN).forEach(function(f) {
        var atual = aba === 'fila' ? filtro : filtroPun;
        box.appendChild(h('button', { type: 'button', class: 'md-chip', 'aria-pressed': String(f[0] === atual), text: f[1], onClick: function() {
            if (aba === 'fila') filtro = f[0]; else filtroPun = f[0];
            renderFiltros();
            carregar(true);
        } }));
    });
}

function selecionarAba(nova) {
    aba = nova;
    [['fila', $('tabFila')], ['bloqueios', $('tabBloqueios')]].forEach(function(t) {
        var at = t[0] === aba;
        t[1].classList.toggle('active', at);
        t[1].setAttribute('aria-selected', String(at));
        t[1].tabIndex = at ? 0 : -1;
    });
    renderFiltros();
    carregar(true);
}

// ── Fila ────────────────────────────────────────────────────────────────────
function cartaoDenuncia(d) {
    return h('li', { class: 'md-item' }, [
        h('div', { class: 'md-item-topo' }, [
            h('div', null, [
                h('div', { class: 'md-item-titulo', text: 'Denúncia #' + d.id + ' · ' + (d.denunciado ? d.denunciado.nome : 'Usuário') }),
                h('div', { class: 'md-item-meta', text: Moderacao.rotuloCategoria(d.categoria) + (d.id_desafio ? ' · Desafio #' + d.id_desafio : '') + ' · ' + fmtDataHora(d.criado_em) })
            ]),
            Moderacao.seloDenuncia(d.status)
        ]),
        prazo(d),
        d.em_analise_comigo ? h('p', { class: 'md-item-meta', text: 'Em análise por você.' }) : null,
        h('p', { class: 'md-item-texto', text: d.descricao.length > 180 ? d.descricao.slice(0, 180) + '…' : d.descricao }),
        h('div', { class: 'gx-acoes' }, h('button', { type: 'button', class: 'gx-btn pequeno', text: 'Abrir', 'aria-label': 'Abrir denúncia #' + d.id,
            onClick: function() { abrirDetalhe(d.id); } }))
    ]);
}

// ── Punições aplicadas pela academia ───────────────────────────────────────
function cartaoPunicao(p) {
    return h('li', { class: 'md-item' }, [
        h('div', { class: 'md-item-topo' }, [
            h('div', null, [
                h('div', { class: 'md-item-titulo', text: (Moderacao.TIPOS_PUNICAO[p.tipo] || p.tipo) + ' · ' + (p.usuario ? p.usuario.nome : 'Usuário #' + p.id_usuario) }),
                h('div', { class: 'md-item-meta', text: 'Desde ' + fmtDataHora(p.inicio) + (p.fim ? ' até ' + fmtDataHora(p.fim) : (p.tipo === 'bloqueio_academia' ? ' (sem prazo)' : '')) +
                    ' · aplicada por ' + (p.aplicada_por === 'academia' ? 'você' : 'equipe GymScore') })
            ]),
            p.revogada_em ? Moderacao.selo('Revogada', 'quitado') : p.vigente ? Moderacao.selo('Vigente', 'vigente') : Moderacao.selo('Encerrada', 'encerrada')
        ]),
        h('p', { class: 'md-item-texto', text: p.justificativa })
    ]);
}

async function carregar(reiniciar) {
    var painel = $('painel');
    if (reiniciar) { pagina = 1; Skeleton.cards(painel, 3, 110); }
    try {
        var r = aba === 'fila'
            ? (await API.academiaDenuncias(filtro, pagina, LIMITE)).data || {}
            : (await API.academiaPunicoes('', filtroPun, pagina, LIMITE)).data || {};
        total = r.total || 0;
        if (reiniciar) {
            Skeleton.fim(painel);
            limpar(painel);
            if (!total) {
                painel.appendChild(aba === 'fila'
                    ? estadoVazio({ icone: '🛡️', titulo: filtro ? 'Nenhuma denúncia com este status' : 'Nenhuma denúncia', texto: 'Denúncias de suborno de instrutor nos seus desafios aparecem aqui.' })
                    : estadoVazio({ icone: '✅', titulo: 'Nenhuma punição', texto: 'Advertências e bloqueios que você aplicar ao decidir denúncias aparecem aqui.' }));
                $('mais').hidden = true;
                return;
            }
            ul = h('ul', { class: 'md-lista' });
            painel.appendChild(ul);
        }
        (r.itens || []).forEach(function(x) { ul.appendChild(aba === 'fila' ? cartaoDenuncia(x) : cartaoPunicao(x)); });
        $('mais').hidden = pagina * LIMITE >= total;
    } catch (e) {
        Gestao.erro(painel, e, function() { carregar(true); });
        $('mais').hidden = true;
    }
}

// ── Detalhe e ações ─────────────────────────────────────────────────────────
var modalAtual = null;
async function abrirDetalhe(id) {
    var corpo = h('div', null, h('p', { class: 'gs-hint', text: 'Carregando…' }));
    modalAtual = Modal.criar({ titulo: 'Denúncia #' + id, corpo: corpo });
    await preencherDetalhe(id, corpo);
}

async function preencherDetalhe(id, corpo) {
    var det;
    try { det = (await API.academiaDenuncia(id)).data; }
    catch (e) {
        limpar(corpo).appendChild(estadoVazio({ erro: true, titulo: 'Não foi possível abrir', texto: mensagemErro(e) }));
        return;
    }
    var d = det.denuncia, hist = det.historico_na_academia || [];
    limpar(corpo);
    corpo.appendChild(h('div', { class: 'md-acoes' }, [Moderacao.seloDenuncia(d.status), prazo(d)]));
    corpo.appendChild(h('dl', { class: 'md-dl' }, [
        h('dt', { text: 'Denunciado' }), h('dd', { text: (d.denunciado ? d.denunciado.nome : '—') + (d.denunciado && d.denunciado.username ? ' (@' + d.denunciado.username + ')' : '') }),
        h('dt', { text: 'Categoria' }), h('dd', { text: Moderacao.rotuloCategoria(d.categoria) }),
        h('dt', { text: 'Desafio' }), h('dd', null, d.id_desafio ? h('a', { class: 'gs-link', href: '/academia/podio?id=' + encodeURIComponent(d.id_desafio), text: '#' + d.id_desafio }) : '—'),
        h('dt', { text: 'Recebida em' }), h('dd', { text: fmtDataHora(d.criado_em) })
    ]));
    corpo.appendChild(h('h3', { class: 'gx-secao', text: 'Relato' }));
    corpo.appendChild(h('p', { class: 'md-item-texto', text: d.descricao }));
    corpo.appendChild(h('h3', { class: 'gx-secao', text: 'Imagens (' + (d.evidencias || []).length + ')' }));
    corpo.appendChild((d.evidencias || []).length
        ? Moderacao.galeria(d.evidencias, function(o) { return API.urlEvidenciaAcademia(d.id, o); })
        : h('p', { class: 'gs-hint', text: 'Sem imagens.' }));
    if (d.decidida_em) {
        corpo.appendChild(h('h3', { class: 'gx-secao', text: 'Decisão · ' + fmtDataHora(d.decidida_em) }));
        corpo.appendChild(h('p', { class: 'md-item-texto', text: d.decisao_texto || '' }));
    }
    corpo.appendChild(h('h3', { class: 'gx-secao', text: 'Histórico do denunciado na sua academia' }));
    corpo.appendChild(hist.length ? h('ul', { class: 'md-lista' }, hist.map(cartaoPunicao)) : h('p', { class: 'gs-hint', text: 'Nenhuma punição anterior nesta academia.' }));

    if (emAberto(d)) {
        var acoes = h('div', { class: 'md-acoes' });
        if (d.status === 'aberta') acoes.appendChild(h('button', { type: 'button', class: 'gs-btn gs-btn-secondary', text: 'Assumir análise', onClick: function(e) { assumir(d, e.currentTarget, corpo); } }));
        acoes.appendChild(h('button', { type: 'button', class: 'gs-btn gs-btn-primary', text: 'Decidir', onClick: function() { decidir(d, corpo); } }));
        acoes.appendChild(h('button', { type: 'button', class: 'gs-btn gs-btn-secondary', text: 'Encaminhar à equipe GymScore', onClick: function() { encaminhar(d); } }));
        corpo.appendChild(acoes);
        corpo.appendChild(h('p', { class: 'gs-hint', text: 'Você pode aplicar advertência ou bloqueio nos seus desafios. Para suspensão, banimento ou anulação do resultado, encaminhe à equipe GymScore.' }));
    }
}

async function assumir(d, btn, corpo) {
    var ok = await Modal.confirmar({ titulo: 'Assumir a denúncia #' + d.id + '?', mensagem: 'A denúncia passa para "em análise" com a sua academia como responsável. O prazo continua correndo.',
        linhas: [['Denunciado', d.denunciado ? d.denunciado.nome : '—'], ['Categoria', Moderacao.rotuloCategoria(d.categoria)]], confirmar: 'Assumir' });
    if (!ok) return;
    btnLoading(btn, true);
    try {
        await API.academiaAssumir(d.id);
        showToast('Denúncia assumida.', 'success');
        await preencherDetalhe(d.id, corpo);
        carregar(true);
    } catch (e) { btnLoading(btn, false); showToast(mensagemErro(e), 'error'); }
}

async function encaminhar(d) {
    var r = await Moderacao.textoModal({
        titulo: 'Encaminhar a denúncia #' + d.id, perigo: true,
        mensagem: 'A denúncia sai da sua fila e vai para a equipe GymScore, que decide. Não é possível desfazer.',
        rotulo: 'Justificativa do encaminhamento', min: 3, max: 2000, confirmar: 'Encaminhar',
        executar: function(texto) { return API.academiaEncaminhar(d.id, texto); }
    });
    if (!r) return;
    showToast('Denúncia encaminhada à equipe GymScore.', 'success');
    if (modalAtual) modalAtual.fechar();
    carregar(true);
}

async function decidir(d, corpo) {
    var nome = 'mdStatus' + d.id;
    var opcoes = [
        ['procedente', 'Procedente', 'A denúncia tem fundamento.'],
        ['improcedente', 'Improcedente', 'Não houve a irregularidade.'],
        ['arquivada', 'Arquivar', 'Duplicada ou sem elementos para analisar.']
    ];
    var radios = h('div', { class: 'md-radios', role: 'radiogroup', 'aria-label': 'Resultado' }, opcoes.map(function(o) {
        return h('label', { class: 'md-radio' }, [h('input', { type: 'radio', name: nome, value: o[0] }), h('span', null, [h('strong', { text: o[1] }), h('small', { text: o[2] })])]);
    }));
    var selPun = h('select', { id: 'mdPun' + d.id, class: 'gs-input' }, [
        h('option', { value: '', text: 'Nenhuma' }),
        h('option', { value: 'advertencia', text: 'Advertência' }),
        h('option', { value: 'bloqueio_academia', text: 'Bloqueio nos desafios da academia' })
    ]);
    var fim = h('input', { id: 'mdFim' + d.id, class: 'gs-input', type: 'date' });
    var campoFim = h('div', { class: 'gs-field', hidden: true }, [h('label', { class: 'gs-label', for: 'mdFim' + d.id, text: 'Bloqueio até (opcional; vazio = sem prazo)' }), fim]);
    var campoPun = h('div', { class: 'gs-field', hidden: true }, [h('label', { class: 'gs-label', for: 'mdPun' + d.id, text: 'Punição ao denunciado' }), selPun]);
    function status() { var r = radios.querySelector('input:checked'); return r ? r.value : ''; }
    function refletir() {
        campoPun.hidden = status() !== 'procedente';
        if (campoPun.hidden) selPun.value = '';
        campoFim.hidden = selPun.value !== 'bloqueio_academia';
    }
    radios.addEventListener('change', refletir);
    selPun.addEventListener('change', refletir);

    var r = await Moderacao.textoModal({
        titulo: 'Decidir a denúncia #' + d.id,
        mensagem: 'A decisão é definitiva e o denunciante é avisado (sem detalhes da punição). O denunciado é avisado se houver punição.',
        linhas: [['Denunciado', d.denunciado ? d.denunciado.nome : '—'], ['Categoria', Moderacao.rotuloCategoria(d.categoria)]],
        extra: h('div', null, [h('div', { class: 'gs-field' }, [h('span', { class: 'gs-label', text: 'Resultado' }), radios]), campoPun, campoFim]),
        rotulo: 'Texto da decisão', min: 3, max: 2000, confirmar: 'Confirmar decisão',
        validar: function() {
            if (!status()) return 'Escolha o resultado.';
            if (selPun.value === 'bloqueio_academia' && fim.value && new Date(fim.value + 'T23:59:59') <= new Date()) return 'A data do fim do bloqueio precisa ser futura.';
            return '';
        },
        executar: function(texto) {
            var corpoReq = { status: status(), decisao_texto: texto };
            if (selPun.value) {
                corpoReq.punicao = { tipo: selPun.value };
                if (selPun.value === 'bloqueio_academia' && fim.value) corpoReq.punicao.fim = new Date(fim.value + 'T23:59:59').toISOString();
            }
            return API.academiaDecidir(d.id, corpoReq);
        }
    });
    if (!r) return;
    showToast('Denúncia decidida.', 'success');
    await preencherDetalhe(d.id, corpo);
    carregar(true);
}

$('tabFila').addEventListener('click', function() { selecionarAba('fila'); });
$('tabBloqueios').addEventListener('click', function() { selecionarAba('bloqueios'); });
document.querySelector('[role="tablist"]').addEventListener('keydown', function(e) {
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return;
    selecionarAba(aba === 'fila' ? 'bloqueios' : 'fila');
    (aba === 'fila' ? $('tabFila') : $('tabBloqueios')).focus();
});
$('btnMais').addEventListener('click', function() { pagina++; carregar(false); });

renderFiltros();
carregar(true);
renderBottomNav('/academia');
})();
