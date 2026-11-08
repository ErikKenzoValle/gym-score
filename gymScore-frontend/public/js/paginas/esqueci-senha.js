// Script da página esqueci-senha.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
document.getElementById('recoverForm').addEventListener('submit', async function(e) {
    e.preventDefault();
    var errEl = document.getElementById('errorMsg');
    var okEl = document.getElementById('successMsg');
    var btn = e.target.querySelector('input[type="submit"]');
    errEl.style.display = 'none';
    okEl.style.display = 'none';
    btn.disabled = true;

    try {
        await API.esqueciSenha(document.getElementById('email').value.trim());
        // Resposta sempre genérica: não revela se o e-mail existe
        okEl.textContent = 'Se o e-mail estiver cadastrado, você receberá um link para redefinir a senha em instantes.';
        okEl.style.display = 'block';
    } catch(err) {
        errEl.textContent = err.message;
        errEl.style.display = 'block';
    }
    btn.disabled = false;
});
