// Script da página academia-podio.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
(async function () {
var user = await Gestao.exigirPerfil('academia');
if (!user) return; // redirecionado para o login/home

var idDesafio = Gestao.param('id');
if (!idDesafio) { window.location.href = '/academia/desafios'; return; }
var conteudo = document.getElementById('conteudo');
var leitorAtivo = null;

async function iniciar(d) {
    var ok = await Modal.confirmar({
        titulo: 'Iniciar "' + d.titulo + '"?',
        mensagem: 'As inscrições fecham e os atletas são avisados. Depois disso o pódio pode ser lançado.',
        linhas: [['Inscritos', String(d.participantes_count)], ['Presentes até agora', String(d.presentes_count)]],
        confirmar: 'Iniciar desafio'
    });
    if (!ok) return;
    try {
        await API.iniciarDesafio(d.id);
        showToast('Desafio iniciado!', 'success');
        carregar();
    } catch (e) { showToast(mensagemErro(e), 'error'); }
}

async function cancelar(d) {
    var f = d.financeiro || {};
    var ok = await Modal.confirmar({
        titulo: 'Cancelar "' + d.titulo + '"?',
        mensagem: 'Todos os inscritos recebem a entrada de volta, sem taxa, e sua reserva volta ao saldo. Não dá para desfazer.',
        linhas: [['Inscritos a estornar', String(d.participantes_count)], ['Reserva devolvida a você', fmtBRL(f.reserva_centavos)]],
        confirmar: 'Cancelar desafio', cancelar: 'Manter', perigo: true
    });
    if (!ok) return;
    try {
        await API.cancelarDesafio(d.id);
        showToast('Desafio cancelado e valores devolvidos.', 'success');
        carregar();
    } catch (e) { showToast(mensagemErro(e), 'error'); }
}

function resumo(d) {
    var acoes = [];
    if (d.pode_iniciar) acoes.push(h('button', { type: 'button', class: 'gx-btn', onClick: function() { iniciar(d); } }, 'Iniciar desafio'));
    if (d.status === 'aberto' && d.instrutor_selecao === 'vaga_aberta') {
        acoes.push(h('a', { class: 'gx-btn contorno', href: '/academia/candidatos?id=' + d.id }, 'Candidatos'));
    }
    if (d.pode_editar) acoes.push(h('a', { class: 'gx-btn contorno', href: '/academia/criar-desafio?id=' + d.id }, 'Editar'));
    if (d.pode_cancelar) acoes.push(h('button', { type: 'button', class: 'gx-btn perigo', onClick: function() { cancelar(d); } }, 'Cancelar'));
    var aviso = null;
    if (d.status === 'aberto' && !d.pode_iniciar) {
        aviso = d.vaga_instrutor_aberta ? 'Escolha um instrutor entre os candidatos para poder iniciar.'
            : (d.participantes_count < 1 ? 'Aguardando a primeira inscrição para poder iniciar.' : null);
    }
    return h('div', { class: 'gx-card' }, [
        h('div', { class: 'gx-linha' }, [h('h2', { style: { fontSize: '1.05rem' }, text: d.titulo }), Gestao.selo(d.status)]),
        h('div', { class: 'gx-meta' }, [
            h('span', { text: Gestao.quando(d.data_evento) }), d.local ? h('span', { text: d.local }) : null,
            h('span', { text: Gestao.textoInstrutor(d) })
        ]),
        Gestao.barraVagas(d),
        aviso ? h('p', { class: 'gx-texto', style: { marginTop: '10px' }, text: aviso }) : null,
        acoes.length ? h('div', { class: 'gx-acoes' }, acoes) : null
    ]);
}

// Anulação do resultado (Q3: só a academia dona relança) e janela de contestação dos pagamentos
function avisoModeracao(d) {
    if (d.aguardando_relancamento) {
        return h('div', { class: 'md-nota erro', role: 'status' }, [
            h('strong', { text: 'Resultado anulado pela moderação' + (d.resultado_anulado_em ? ' em ' + fmtDataHora(d.resultado_anulado_em) : '') + '. ' }),
            'O desafio voltou para "em andamento": confira a chamada e lance o novo pódio abaixo. Se não for relançado, o desafio é cancelado automaticamente após a carência.'
        ]);
    }
    if (d.resultado_anulado) {
        return h('p', { class: 'md-nota erro', text: 'Resultado anulado pela moderação' + (d.resultado_anulado_em ? ' em ' + fmtDataHora(d.resultado_anulado_em) : '') +
            ': as inscrições foram devolvidas integralmente aos atletas e a sua reserva voltou ao saldo.' });
    }
    if (d.status === 'encerrado' && d.pagamentos_retidos) return h('p', { class: 'md-nota alerta', text: 'Pagamentos do pódio retidos: há uma denúncia sobre este desafio em análise.' });
    if (d.status === 'encerrado' && d.pagamentos_liberam_em) return h('p', { class: 'md-nota', text: 'Os pagamentos do pódio entram nas carteiras em ' + fmtDataHora(d.pagamentos_liberam_em) + ' (janela de contestação).' });
    return null;
}

async function carregar() {
    if (leitorAtivo) { leitorAtivo.parar(); leitorAtivo = null; }
    Skeleton.cards(conteudo, 3, 110);
    try {
        var d = (await API.buscarDesafio(idDesafio)).data;
        document.getElementById('titulo').textContent = d.titulo.toUpperCase();
        document.title = d.titulo + ' — GymScore';
        Skeleton.fim(conteudo);
        limpar(conteudo);
        conteudo.appendChild(resumo(d));
        var aviso = avisoModeracao(d);
        if (aviso) conteudo.appendChild(aviso);

        if (d.financeiro) {
            conteudo.appendChild(h('div', { class: 'gx-card' }, [
                h('div', { class: 'gx-card-titulo', text: d.status === 'encerrado' ? 'Financeiro (valores pagos)' : 'Financeiro previsto' }),
                Gestao.quadroFinanceiro(d.financeiro)
            ]));
        }
        if (d.status === 'cancelado') {
            conteudo.appendChild(estadoVazio({ icone: '🚫', titulo: 'Desafio cancelado', texto: 'As inscrições e a reserva foram devolvidas.' }));
            return;
        }
        if (d.status === 'encerrado') {
            conteudo.appendChild(Gestao.podioFinal(d));
            return;
        }
        conteudo.appendChild(h('h2', { class: 'gx-secao', text: 'Chamada' }));
        var areaPresenca = h('div');
        conteudo.appendChild(areaPresenca);
        conteudo.appendChild(h('h2', { class: 'gx-secao', text: 'Pódio' }));
        var areaPodio = h('div');
        conteudo.appendChild(areaPodio);

        // O pódio depende dos presentes: a cada presença nova o formulário é remontado com o placar atual
        function montarPodio() { Gestao.podio(areaPodio, d, { aoFim: carregar }); }
        leitorAtivo = Gestao.presenca(areaPresenca, d, { aoMudar: function(r) {
            d.presentes_count = r.presentes_count;
            d.pode_lancar_podio = d.status === 'em_andamento' && r.presentes_count >= 1;
            montarPodio();
        } });
        montarPodio();
    } catch (e) {
        Gestao.erro(conteudo, e, carregar);
    }
}

carregar();
renderBottomNav('/academia/desafios');
})();
