// GymScore Admin — login. Sessão pelo cookie HttpOnly admin_jwt; o token nunca vem no corpo.
'use strict';

(function() {
    document.addEventListener('DOMContentLoaded', async function() {
        var form = document.getElementById('admLoginForm');
        var email = document.getElementById('admEmail');
        var senha = document.getElementById('admSenhaLogin');
        var btn = document.getElementById('admEntrar');
        var msg = document.getElementById('admLoginMsg');

        function mostrar(texto, tipo) {
            msg.textContent = texto;
            msg.className = 'gs-msg show ' + (tipo || 'error');
        }

        var q = new URLSearchParams(window.location.search);
        if (q.get('sessao') === 'expirada') mostrar('Sua sessão terminou. Entre de novo.', 'error');

        // Já logado? Vai direto para o painel.
        try {
            await Adm.api('GET', '/auth/me', undefined, { semRedirecionar: true });
            window.location.replace('/admin');
            return;
        } catch (e) { /* sem sessão: fica no login */ }

        form.addEventListener('submit', async function(e) {
            e.preventDefault();
            if (!email.value.trim() || !senha.value) { mostrar('Informe e-mail e senha.'); return; }
            btn.disabled = true;
            btn.classList.add('loading');
            try {
                await Adm.api('POST', '/auth/login', { email: email.value.trim(), senha: senha.value }, { semRedirecionar: true });
                senha.value = '';
                window.location.replace('/admin');
            } catch (err) {
                btn.disabled = false;
                btn.classList.remove('loading');
                senha.value = '';
                senha.focus();
                mostrar(err.status === 401 ? 'E-mail ou senha inválidos.' : err.message);
            }
        });
    });
})();
