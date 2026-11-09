// Script da página cadastro-instrutor.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
// Rodada 4: o pedido de instrutor NÃO muda a conta: ela continua de atleta e ganha o modo instrutor quando
// a equipe GymScore aprova. Esta tela envia o pedido ou mostra o estado dele.
var logado = Auth.logado();
var usuario = Auth.getUser();
document.getElementById('semLogin').hidden = logado;
document.getElementById('instrutorForm').hidden = true;
if (!logado) document.getElementById('voltar').setAttribute('href', '/login');

// Estado do pedido (em análise / recusado / aprovado) no lugar do formulário
function mostrarPedido(u) {
    var st = Modo.pedidoInstrutor(u);
    var box = limpar(document.getElementById('pedidoStatus'));
    document.getElementById('instrutorForm').hidden = true;
    box.hidden = false;
    var motivo = st === 'rejeitada' && u.instrutor && u.instrutor.motivo_rejeicao;
    var titulo = st === 'aprovada' ? 'Modo instrutor liberado' : st === 'rejeitada' ? 'Pedido recusado' : 'Solicitação em análise';
    var texto = st === 'aprovada'
        ? 'Seu pedido foi aprovado. Use o seletor Atleta/Instrutor no topo do app para trocar de modo quando quiser.'
        : st === 'rejeitada'
            ? 'A equipe GymScore recusou seu pedido de instrutor. Sua conta de atleta continua normalmente.'
            : 'A equipe GymScore vai analisar seu CREF e você recebe uma notificação com o resultado. Enquanto isso, sua conta continua de atleta e você usa o app normalmente.';
    var acoes = [];
    if (st === 'aprovada' && Modo.atual(u) !== 'instrutor') {
        var btn = h('button', { type: 'button', class: 'gs-btn gs-btn-primary', text: 'Trocar para o modo instrutor' });
        btn.addEventListener('click', async function() {
            btnLoading(btn, true);
            try { await Modo.trocar('instrutor'); }
            catch (e) { btnLoading(btn, false); showToast(mensagemErro(e), 'error'); }
        });
        acoes.push(btn);
    } else if (st === 'aprovada') {
        acoes.push(h('a', { class: 'gs-btn gs-btn-primary', href: '/instrutor', text: 'Ir para a agenda' }));
    }
    if (Modo.atual(u) !== 'instrutor') acoes.push(h('a', { class: 'gs-btn ' + (acoes.length ? 'gs-btn-secondary' : 'gs-btn-primary'), href: '/menu', text: 'Continuar como atleta' }));
    if (st !== 'aprovada') acoes.push(h('a', { class: 'gs-btn gs-btn-secondary', href: '/instrutor', text: 'Convites de academia' }));
    box.appendChild(h('section', { class: 'gs-card gs-cadastro-status' + (st === 'rejeitada' ? ' rejeitado' : ''), role: 'status', 'aria-live': 'polite' }, [
        h('span', { class: 'gs-cadastro-icone', 'aria-hidden': 'true', text: st === 'aprovada' ? '✓' : st === 'rejeitada' ? '✕' : '⏳' }),
        h('h2', { text: titulo }),
        h('p', { text: texto }),
        motivo ? h('p', { class: 'gs-cadastro-motivo' }, [h('strong', { text: 'Motivo: ' }), motivo]) : null,
        h('div', { class: 'gs-cadastro-acoes' }, acoes)
    ]));
}

// Tipo de conta e pedido confirmados pelo servidor (/usuarios/me); o cache só preenche a tela enquanto isso
function _aplicarConta(u) {
    document.getElementById('contaInfo').textContent = 'Conta: ' + nomeCompleto(u) + (u.email ? ' (' + u.email + ')' : '');
    if (u.perfil === 'academia') {
        document.getElementById('instrutorForm').hidden = true;
        var aviso = document.getElementById('semLogin');
        aviso.hidden = false;
        limpar(aviso).appendChild(h('div', { class: 'gs-msg error show', role: 'alert', text: 'Contas de academia não podem ser instrutor. Use uma conta de atleta.' }));
        return;
    }
    if (Modo.pedidoInstrutor(u)) { mostrarPedido(u); return; }
    document.getElementById('pedidoStatus').hidden = true;
    document.getElementById('instrutorForm').hidden = false;
}
if (logado && usuario) {
    document.getElementById('contaInfo').textContent = 'Conta: ' + nomeCompleto(usuario);
    Auth.verificar().then(function(u) { if (u) _aplicarConta(u); }, function(e) {
        // Sem confirmação do servidor: mostra o formulário (o backend recusa pedido duplicado)
        document.getElementById('instrutorForm').hidden = false;
        showToast('Não foi possível confirmar sua sessão. ' + mensagemErro(e), 'error');
    });
}

document.getElementById('instrutorForm').addEventListener('submit', async function(e) {
    e.preventDefault();
    var btn = document.getElementById('submitBtn');
    var msgEl = document.getElementById('formMsg');
    var campoCref = document.getElementById('inputCREF');
    msgEl.classList.remove('show');

    var cref = campoCref.value.trim();
    if (!Valid.cref(cref)) { erroCampo(campoCref, 'Informe o CREF no formato 123456-G/SP.'); campoCref.focus(); return; }
    erroCampo(campoCref, '');

    btnLoading(btn, true);
    try {
        // Só CREF e especialidade: a identidade vem do token, e quem vincula à academia é a própria academia
        await API.cadastrarInstrutor({ cref: cref, especialidade: document.getElementById('inputEsp').value.trim() });
        // A conta continua de atleta; /usuarios/me passa a trazer instrutor.status (pendente)
        Auth._verificado = null;
        var u = await Auth.verificar();
        showToast('Solicitação enviada!', 'success');
        mostrarPedido(u || Object.assign({}, Auth.getUser(), { instrutor: { status: 'pendente' } }));
        var t = document.querySelector('#pedidoStatus h2');
        if (t) { t.setAttribute('tabindex', '-1'); t.focus(); }
    } catch (err) {
        msgEl.className = 'gs-msg error show';
        msgEl.textContent = mensagemErro(err);
        btnLoading(btn, false);
    }
});
