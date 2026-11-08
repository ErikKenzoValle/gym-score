// Script da página cadastro-instrutor.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
var logado = Auth.logado();
var usuario = Auth.getUser();
document.getElementById('semLogin').hidden = logado;
document.getElementById('instrutorForm').hidden = !logado;
if (!logado) document.getElementById('voltar').setAttribute('href', '/login');
// Perfil confirmado pelo servidor (/usuarios/me); o cache só preenche a tela enquanto isso
function _aplicarConta(usuario) {
    document.getElementById('contaInfo').textContent = 'Conta: ' + nomeCompleto(usuario) + (usuario.email ? ' (' + usuario.email + ')' : '');
    if (usuario.perfil === 'instrutor') window.location.href = '/instrutor';
    if (usuario.perfil === 'academia') {
        document.getElementById('instrutorForm').hidden = true;
        var aviso = document.getElementById('semLogin');
        aviso.hidden = false;
        aviso.textContent = '';
        aviso.appendChild(h('div', { class: 'gs-msg error show', role: 'alert', text: 'Contas de academia não podem ser instrutor. Use uma conta de atleta.' }));
    }
}
if (logado && usuario) {
    document.getElementById('contaInfo').textContent = 'Conta: ' + nomeCompleto(usuario);
    Auth.verificar().then(function(u) { if (u) _aplicarConta(u); }, function() {});
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
        var resp = await API.cadastrarInstrutor({ cref: cref, especialidade: document.getElementById('inputEsp').value.trim() });
        // O perfil muda na hora no servidor: basta atualizar o cache local, sem novo login
        await Auth.refresh();
        if (resp.data && resp.data.status === 'pendente') {
            // Solicitação vai para a aprovação do admin; /instrutor mostra o aviso (e convites de academia)
            msgEl.className = 'gs-msg success show';
            msgEl.setAttribute('role', 'status');
            msgEl.textContent = 'Solicitação enviada! A equipe GymScore vai analisar seu CREF e você recebe uma notificação com o resultado.';
            setTimeout(function() { window.location.href = '/instrutor'; }, 3500);
        } else {
            showToast('Perfil de instrutor ativado!', 'success');
            setTimeout(function() { window.location.href = '/instrutor'; }, 900);
        }
    } catch (err) {
        msgEl.className = 'gs-msg error show';
        msgEl.textContent = mensagemErro(err);
        btnLoading(btn, false);
    }
});
