// Script da página alterar-senha.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
if (!Auth.require()) throw new Error('unauthenticated');
renderBottomNav('/perfil');

function togglePwd(inputId, btn) {
    var input = document.getElementById(inputId);
    var isText = input.type === 'text';
    input.type = isText ? 'password' : 'text';
    btn.textContent = isText ? '👁' : '🙈';
}

function checkStrength() {
    var val = document.getElementById('novaSenha').value;
    var bar = document.getElementById('strengthBar');
    var label = document.getElementById('strengthLabel');
    var score = 0;
    if (val.length >= 8) score++;
    if (val.length >= 10) score++;
    if (/[A-Z]/.test(val)) score++;
    if (/[0-9]/.test(val)) score++;
    if (/[^A-Za-z0-9]/.test(val)) score++;

    var widths = ['0%', '20%', '40%', '65%', '85%', '100%'];
    var colors = ['#e74c3c', '#e74c3c', '#e67e22', '#f1c40f', '#2ecc71', '#27ae60'];
    var labels = ['', 'Muito fraca', 'Fraca', 'Razoável', 'Boa', 'Forte'];
    bar.style.width = widths[score];
    bar.style.background = colors[score];
    label.textContent = labels[score];
}

document.getElementById('changePasswordForm').addEventListener('submit', async function(e) {
    e.preventDefault();
    var errEl = document.getElementById('errorMsg');
    var okEl = document.getElementById('successMsg');
    errEl.style.display = 'none';
    okEl.style.display = 'none';

    var atual = document.getElementById('senhaAtual').value;
    var nova = document.getElementById('novaSenha').value;
    var confirmar = document.getElementById('confirmarSenha').value;

    if (nova !== confirmar) {
        errEl.textContent = 'As novas senhas não coincidem.';
        errEl.style.display = 'block';
        return;
    }
    if (nova.length < 8) {
        errEl.textContent = 'A nova senha deve ter pelo menos 8 caracteres.';
        errEl.style.display = 'block';
        return;
    }

    try {
        await API.alterarSenha(atual, nova);
        okEl.textContent = 'Senha alterada com sucesso!';
        okEl.style.display = 'block';
        document.getElementById('changePasswordForm').reset();
        document.getElementById('strengthBar').style.width = '0';
        document.getElementById('strengthLabel').textContent = '';
        setTimeout(function() { window.location.href = '/perfil'; }, 2000);
    } catch(err) {
        errEl.textContent = err.message;
        errEl.style.display = 'block';
    }
});

// Eventos (antes eram atributos on* no HTML)
document.querySelectorAll('[data-toggle-pwd]').forEach(function(btn) {
    btn.addEventListener('click', function() { togglePwd(btn.dataset.togglePwd, btn); });
});
document.getElementById('novaSenha').addEventListener('input', checkStrength);
document.getElementById('btnCancelarSenha').addEventListener('click', function() { window.location.href = '/perfil'; });
