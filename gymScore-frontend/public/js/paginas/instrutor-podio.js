// Script da página instrutor-podio.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
(async function () {
var user = await Gestao.exigirPerfil('instrutor');
if (!user) return; // redirecionado para o login/home

var idDesafio = Gestao.param('id');
if (!idDesafio) { window.location.href = '/instrutor'; return; }
document.getElementById('voltar').href = '/instrutor/checkin?id=' + encodeURIComponent(idDesafio);
var conteudo = document.getElementById('conteudo');

async function carregar() {
    Skeleton.cards(conteudo, 3, 110);
    try {
        var d = (await API.buscarDesafio(idDesafio)).data;
        document.getElementById('titulo').textContent = d.titulo.toUpperCase();
        Skeleton.fim(conteudo);
        limpar(conteudo);
        conteudo.appendChild(h('div', { class: 'gx-card' }, [
            h('div', { class: 'gx-linha' }, [h('span', { class: 'gx-texto', text: Gestao.horario(d) + (Gestao.enderecoCurto(d) ? ' · ' + Gestao.enderecoCurto(d) : '') }), Gestao.selo(d.status)]),
            h('p', { class: 'gx-texto', style: { marginTop: '8px' }, text: 'Presentes: ' + d.presentes_count + ' de ' + d.participantes_count +
                ' · Prêmio total: ' + fmtBRL(d.premiacao ? d.premiacao.premio_total_centavos : 0) })
        ]));
        var area = h('div');
        conteudo.appendChild(area);
        Gestao.podio(area, d, { aoFim: function() { document.getElementById('sub').textContent = 'Resultado registrado. Os atletas foram notificados.'; carregar(); } });
        if (d.status === 'encerrado') conteudo.appendChild(h('div', { class: 'gx-acoes' }, h('a', { class: 'gx-btn', href: '/instrutor' }, 'Voltar à agenda')));
        if (!d.pode_lancar_podio && d.status !== 'encerrado') {
            area.appendChild(h('div', { class: 'gx-acoes' }, h('a', { class: 'gx-btn', href: '/instrutor/checkin?id=' + encodeURIComponent(d.id) }, 'Ir para a chamada')));
        }
    } catch (e) {
        Gestao.erro(conteudo, e, carregar);
    }
}

carregar();
renderBottomNav('/instrutor');
})();
