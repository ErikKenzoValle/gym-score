// Script da página academia-dashboard.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
(async function () {
var user = await Gestao.exigirPerfil('academia');
if (!user) return; // redirecionado para o login/home
if (user.nome) document.getElementById('academiaNome').textContent = user.nome;

var conteudo = document.getElementById('conteudo');

function kpi(rotulo, valor) {
    return h('div', { class: 'gx-kpi' }, [h('div', { class: 'gx-kpi-rotulo', text: rotulo }), h('div', { class: 'gx-kpi-valor', text: valor })]);
}

// Primeiros passos para quem ainda não tem desafio: o que falta e o próximo clique
function primeirosPassos(dash, saldo) {
    return h('div', { class: 'gx-card destaque' }, [
        h('div', { class: 'gx-card-titulo', text: 'Primeiros passos' }),
        h('ol', { class: 'gx-texto', style: { paddingLeft: '18px', marginBottom: '12px' } }, [
            h('li', { text: saldo > 0 ? 'Saldo na carteira: ' + fmtBRL(saldo) + ' (o prêmio fixo e o instrutor são reservados daqui).'
                : 'Deposite saldo na carteira: o prêmio fixo e o instrutor são reservados dele.' }),
            h('li', { text: dash.instrutores_vinculados > 0 ? 'Você já tem instrutor vinculado.'
                : 'Vincule um instrutor pelo e-mail dele, ou abra a vaga de instrutor ao criar o desafio.' }),
            h('li', { text: 'Crie o desafio e divulgue para os atletas.' })
        ]),
        h('div', { class: 'gx-acoes' }, [
            saldo > 0 ? null : h('a', { class: 'gx-btn contorno', href: '/depositar?voltar=/academia' }, 'Depositar'),
            h('a', { class: 'gx-btn', href: '/academia/criar-desafio' }, 'Criar primeiro desafio')
        ])
    ]);
}

async function carregar() {
    Skeleton.cards(conteudo, 3, 90);
    try {
        var res = await Promise.all([API.dashboardAcademia(), API.carteira().catch(function() { return null; })]);
        var dash = res[0].data || {};
        var saldo = res[1] && res[1].data ? res[1].data.saldo_centavos : 0;
        var st = dash.desafios_por_status || {};
        var ativos = (st.aberto || 0) + (st.em_andamento || 0);
        if (dash.academia && dash.academia.nome_fantasia) document.getElementById('academiaNome').textContent = dash.academia.nome_fantasia;

        Skeleton.fim(conteudo);
        limpar(conteudo);
        conteudo.appendChild(h('div', { class: 'gx-kpis' }, [
            kpi('Eventos ativos', String(ativos)),
            kpi('Inscritos no mês', String(dash.inscritos_mes || 0)),
            kpi('Pote em jogo', fmtBRL(dash.pote_em_jogo_centavos)),
            kpi('Presença', (dash.taxa_presenca || 0).toLocaleString('pt-BR', { maximumFractionDigits: 1 }) + '%'),
            kpi('Avaliação média', dash.avaliacoes_total ? (dash.avaliacao_media || 0).toLocaleString('pt-BR', { minimumFractionDigits: 1, maximumFractionDigits: 1 }) + ' ★' : 'Sem notas'),
            kpi('Saldo na carteira', fmtBRL(saldo))
        ]));
        conteudo.appendChild(h('div', { class: 'gx-acoes', style: { marginTop: '0', marginBottom: '8px' } }, [
            h('a', { class: 'gx-btn', href: '/academia/criar-desafio' }, '+ Criar desafio'),
            h('a', { class: 'gx-btn contorno', href: '/academia/desafios' }, 'Meus desafios (' + (dash.total_desafios || 0) + ')'),
            h('a', { class: 'gx-btn contorno', href: '/academia/instrutores' }, 'Instrutores (' + (dash.instrutores_vinculados || 0) + ')'),
            h('a', { class: 'gx-btn contorno', href: '/academia/denuncias' }, 'Denúncias'),
            h('a', { class: 'gx-btn contorno', href: '/depositar?voltar=/academia' }, 'Depositar')
        ]));

        if (!dash.total_desafios) {
            conteudo.appendChild(primeirosPassos(dash, saldo));
            return;
        }

        conteudo.appendChild(h('h2', { class: 'gx-secao', text: 'Por status' }));
        conteudo.appendChild(h('div', { class: 'gx-abas' }, ['aberto', 'em_andamento', 'encerrado', 'cancelado'].map(function(s) {
            return h('a', { class: 'gx-aba', href: '/academia/desafios?status=' + s, text: Gestao.ROTULO_STATUS[s] + ': ' + (st[s] || 0) });
        })));

        conteudo.appendChild(h('h2', { class: 'gx-secao', text: 'Próximos eventos' }));
        var proximos = dash.proximos_eventos || [];
        if (!proximos.length) {
            conteudo.appendChild(estadoVazio({
                icone: '📅', titulo: 'Nenhum evento futuro aberto',
                texto: 'Crie um desafio para ele aparecer no feed dos atletas.',
                acao: { texto: 'Criar desafio', href: '/academia/criar-desafio' }
            }));
            return;
        }
        proximos.forEach(function(d) {
            var acoes = [{ texto: 'Gerenciar', href: '/academia/podio?id=' + d.id }];
            var pend = d.financeiro ? d.financeiro.candidaturas_pendentes : 0;
            if (pend || !(d.equipe || []).length) acoes.push({ texto: pend ? 'Candidatos (' + pend + ')' : 'Montar equipe', href: '/academia/candidatos?id=' + d.id });
            if (d.pode_editar) acoes.push({ texto: 'Editar', href: '/academia/criar-desafio?id=' + d.id });
            conteudo.appendChild(Gestao.cardDesafio(d, acoes));
        });
    } catch (e) {
        Gestao.erro(conteudo, e, carregar);
    }
}

carregar();
renderBottomNav('/academia');
})();
