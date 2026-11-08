// Script da página academia-desafios.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
(async function () {
var user = await Gestao.exigirPerfil('academia');
if (!user) return; // redirecionado para o login/home

var FILTROS = [['', 'Todos'], ['aberto', 'Abertos'], ['em_andamento', 'Em andamento'], ['encerrado', 'Encerrados'], ['cancelado', 'Cancelados']];
var LIMITE = 10;
var status = Gestao.param('status') || '';
if (!FILTROS.some(function(f) { return f[0] === status; })) status = '';
var pagina = 1;
var carregados = 0;

var lista = document.getElementById('lista');
var maisBtn = document.getElementById('maisBtn');

function desenharFiltros() {
    var box = limpar(document.getElementById('filtros'));
    FILTROS.forEach(function(f) {
        box.appendChild(h('button', { type: 'button', class: 'gx-aba' + (f[0] === status ? ' ativa' : ''), 'aria-pressed': f[0] === status ? 'true' : 'false',
            onClick: function() { status = f[0]; history.replaceState(null, '', status ? '?status=' + status : location.pathname); desenharFiltros(); carregar(true); },
            text: f[1] }));
    });
}

async function cancelar(d) {
    var f = d.financeiro || {};
    var linhas = [['Inscritos a estornar', String(d.participantes_count || 0)]];
    if (f.reserva_centavos) linhas.push(['Reserva devolvida a você', fmtBRL(f.reserva_centavos)]);
    var ok = await Modal.confirmar({
        titulo: 'Cancelar "' + d.titulo + '"?',
        mensagem: 'Todos os inscritos recebem a entrada de volta, sem taxa, e sua reserva volta ao saldo. Não dá para desfazer.',
        linhas: linhas, confirmar: 'Cancelar desafio', cancelar: 'Manter', perigo: true
    });
    if (!ok) return;
    try {
        await API.cancelarDesafio(d.id);
        showToast('Desafio cancelado e valores devolvidos.', 'success');
        carregar(true);
    } catch (e) { showToast(mensagemErro(e), 'error'); }
}

function acoesDe(d) {
    var a = [{ texto: 'Gerenciar', href: '/academia/podio?id=' + d.id }];
    if (d.status === 'aberto' && d.instrutor_selecao === 'vaga_aberta') {
        a.push({ texto: 'Candidatos', href: '/academia/candidatos?id=' + d.id });
    }
    if (d.pode_editar) a.push({ texto: 'Editar', href: '/academia/criar-desafio?id=' + d.id });
    if (d.pode_cancelar) a.push({ texto: 'Cancelar', classe: 'perigo', onClick: function() { cancelar(d); } });
    return a;
}

async function carregar(recomeco) {
    if (recomeco) { pagina = 1; carregados = 0; Skeleton.cards(lista, 3, 130); maisBtn.hidden = true; }
    try {
        var resp = await API.listarDesafiosAcademia(status, pagina, LIMITE);
        var dados = resp.data || {};
        var itens = dados.itens || [];
        if (recomeco) { Skeleton.fim(lista); limpar(lista); }
        carregados += itens.length;
        itens.forEach(function(d) { lista.appendChild(Gestao.cardDesafio(d, acoesDe(d))); });
        if (!carregados) {
            lista.appendChild(estadoVazio(status ? {
                icone: '🔍', titulo: 'Nenhum desafio ' + Gestao.ROTULO_STATUS[status].toLowerCase(),
                texto: 'Troque o filtro para ver os outros.', acao: { texto: 'Ver todos', onClick: function() { status = ''; desenharFiltros(); carregar(true); } }
            } : {
                icone: '🏆', titulo: 'Você ainda não criou desafios', texto: 'Crie o primeiro e ele aparece no feed dos atletas.',
                acao: { texto: 'Criar desafio', href: '/academia/criar-desafio' }
            }));
        }
        maisBtn.hidden = carregados >= (dados.total || 0);
        maisBtn.disabled = false;
    } catch (e) {
        if (!recomeco) { pagina--; maisBtn.disabled = false; showToast(mensagemErro(e), 'error'); return; }
        Gestao.erro(lista, e, function() { carregar(true); });
    }
}

maisBtn.addEventListener('click', function() { maisBtn.disabled = true; pagina++; carregar(false); });

desenharFiltros();
carregar(true);
renderBottomNav('/academia/desafios');
})();
