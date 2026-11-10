// Script da página instrutor-checkin.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
(async function () {
var user = await Gestao.exigirPerfil('instrutor');
if (!user) return; // redirecionado para o login/home

var idDesafio = Gestao.param('id');
if (!idDesafio) { window.location.href = '/instrutor'; return; }
var conteudo = document.getElementById('conteudo');
var leitorAtivo = null;

async function iniciar(d) {
    var ok = await Modal.confirmar({
        titulo: 'Iniciar "' + d.titulo + '"?',
        mensagem: 'As inscrições fecham e os atletas são avisados. Depois disso você pode lançar o pódio.',
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

function cabecalho(d, podioLiberado) {
    var g = d.instrutor_ganho;
    var acoes = [];
    if (d.pode_iniciar) acoes.push(h('button', { type: 'button', class: 'gx-btn', onClick: function() { iniciar(d); } }, 'Iniciar desafio'));
    if (podioLiberado) acoes.push(h('a', { class: 'gx-btn', href: '/instrutor/podio?id=' + d.id }, 'Lançar pódio →'));
    return h('div', { class: 'gx-card' }, [
        h('div', { class: 'gx-linha' }, [h('h2', { style: { fontSize: '1.05rem' }, text: d.titulo }), Gestao.selo(d.status)]),
        h('div', { class: 'gx-meta' }, [
            d.academia ? h('span', { text: d.academia.nome_fantasia }) : null,
            h('span', { text: Gestao.horario(d) }), Gestao.enderecoCurto(d) ? h('span', { text: Gestao.enderecoCurto(d) }) : null
        ]),
        Gestao.barraVagas(d),
        g ? h('p', { class: 'gx-texto', style: { marginTop: '10px' }, text: 'Você recebe ' + fmtBRL(g.total_previsto_centavos) + ' neste desafio.' }) : null,
        d.status === 'aberto' && !d.pode_iniciar ? h('p', { class: 'gx-texto gx-recuo', text: d.voce_na_equipe && !d.voce_responsavel
            ? 'Só o responsável pela equipe ou a academia inicia o desafio e lança o pódio. Você faz a chamada normalmente.'
            : 'Para iniciar é preciso ao menos um inscrito.' }) : null,
        acoes.length ? h('div', { class: 'gx-acoes' }, acoes) : null
    ]);
}

async function carregar() {
    if (leitorAtivo) { leitorAtivo.parar(); leitorAtivo = null; }
    Skeleton.cards(conteudo, 3, 110);
    try {
        var d = (await API.buscarDesafio(idDesafio)).data;
        document.getElementById('titulo').textContent = d.titulo.toUpperCase();
        Skeleton.fim(conteudo);
        limpar(conteudo);

        if (d.status === 'cancelado' || d.status === 'encerrado') {
            conteudo.appendChild(cabecalho(d, false));
            if (d.status === 'encerrado') conteudo.appendChild(Gestao.podioFinal(d));
            else conteudo.appendChild(estadoVazio({ icone: '🚫', titulo: 'Desafio cancelado', texto: 'Não há chamada para fazer.', acao: { texto: 'Voltar à agenda', href: '/instrutor' } }));
            return;
        }
        var cab = h('div');
        conteudo.appendChild(cab);
        var area = h('div');
        conteudo.appendChild(area);
        function desenharCabecalho() { limpar(cab).appendChild(cabecalho(d, d.pode_lancar_podio)); }
        desenharCabecalho();
        leitorAtivo = Gestao.presenca(area, d, { aoMudar: function(r) {
            d.presentes_count = r.presentes_count;
            d.pode_lancar_podio = d.status === 'em_andamento' && r.presentes_count >= 1;
            desenharCabecalho();
        } });
    } catch (e) {
        Gestao.erro(conteudo, e, carregar);
    }
}

carregar();
renderBottomNav('/instrutor');
})();
