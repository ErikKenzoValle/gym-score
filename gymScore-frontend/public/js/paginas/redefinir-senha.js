// Script da página redefinir-senha.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
// Token vem do link enviado por e-mail (?token=...); sai da URL logo após a leitura
var resetToken = new URLSearchParams(window.location.search).get('token') || '';
if (resetToken) history.replaceState(null, '', window.location.pathname);

var form = document.getElementById('resetForm');
var errEl = document.getElementById('errorMsg');
var okEl = document.getElementById('successMsg');

if (!resetToken) {
    form.hidden = true;
    errEl.textContent = 'Link inválido. Peça um novo link de recuperação.';
    errEl.style.display = 'block';
}

form.addEventListener('submit', async function(e) {
    e.preventDefault();
    errEl.style.display = 'none';
    okEl.style.display = 'none';

    var nova = document.getElementById('novaSenha').value;
    var confirmar = document.getElementById('confirmarSenha').value;
    if (nova.length < 8) {
        errEl.textContent = 'A senha deve ter pelo menos 8 caracteres.';
        errEl.style.display = 'block';
        return;
    }
    if (nova !== confirmar) {
        errEl.textContent = 'As senhas não coincidem.';
        errEl.style.display = 'block';
        return;
    }

    var btn = form.querySelector('input[type="submit"]');
    btn.disabled = true;
    try {
        await API.redefinirSenha(resetToken, nova);
        Auth.clear(); // o backend revogou todas as sessões do usuário
        okEl.textContent = 'Senha redefinida! Redirecionando para o login...';
        okEl.style.display = 'block';
        setTimeout(function() { window.location.href = '/login'; }, 2000);
    } catch(err) {
        errEl.textContent = err.message;
        errEl.style.display = 'block';
        btn.disabled = false;
    }
});
