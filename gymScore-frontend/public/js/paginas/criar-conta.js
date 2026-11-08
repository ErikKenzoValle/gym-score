// Script da página criar-conta.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
var _genero = '';

function selecionarGenero(val, btn) {
    _genero = val;
    document.getElementById('genero').value = val;
    document.querySelectorAll('.genero-pill').forEach(function(b) {
        b.classList.toggle('selecionado', b.dataset.val === val);
        b.setAttribute('aria-pressed', b.dataset.val === val ? 'true' : 'false');
    });
}

function contarSenha() {
    var len = document.getElementById('senha').value.length;
    var counter = document.getElementById('senhaCounter');
    counter.textContent = len + '/8';
    counter.className = 'senha-counter' + (len >= 8 ? ' ok' : '');
}

document.getElementById('verSenha').addEventListener('change', function() {
    document.getElementById('senha').type = this.checked ? 'text' : 'password';
});

// Validação inline ao sair do campo (a máscara do CPF vem de data-mask em api.js)
document.getElementById('cpf').addEventListener('blur', function() {
    erroCampo(this, this.value && !Valid.cpf(this.value) ? 'CPF inválido. Confira os dígitos.' : '');
});
document.getElementById('email').addEventListener('blur', function() {
    erroCampo(this, this.value && !Valid.email(this.value) ? 'E-mail inválido.' : '');
});

async function criarConta() {
    var btn      = document.getElementById('btnCriar');
    var errorMsg = document.getElementById('errorMsg');
    errorMsg.classList.remove('show');

    var nome      = document.getElementById('nome').value.trim();
    var sobrenome = document.getElementById('sobrenome').value.trim();
    var email     = document.getElementById('email').value.trim();
    var cpf       = document.getElementById('cpf').value.trim();
    var nasc      = document.getElementById('dataNasc').value;
    var senha     = document.getElementById('senha').value;

    if (!nome || !sobrenome || !email || !cpf || !nasc || !senha || !_genero) {
        errorMsg.textContent = !_genero && nome && sobrenome && email && cpf && nasc && senha ? 'Selecione o gênero.' : 'Preencha todos os campos.';
        errorMsg.classList.add('show');
        return;
    }
    if (!Valid.email(email)) { errorMsg.textContent = 'E-mail inválido.'; errorMsg.classList.add('show'); return; }
    if (!Valid.cpf(cpf)) { errorMsg.textContent = 'CPF inválido. Confira os dígitos.'; errorMsg.classList.add('show'); return; }
    if (senha.length < 8) {
        errorMsg.textContent = 'A senha precisa ter pelo menos 8 caracteres.';
        errorMsg.classList.add('show');
        return;
    }

    btn.disabled = true;
    btn.textContent = 'Criando conta...';

    try {
        await API.cadastrar({
            nome: nome,
            sobrenome: sobrenome,
            cpf: cpf,
            email: email,
            senha: senha,
            data_nascimento: nasc,
            genero: _genero
        });
        btn.textContent = 'Conta criada! Redirecionando...';
        setTimeout(function() { window.location.href = '/login'; }, 1400);
    } catch (err) {
        errorMsg.textContent = mensagemErro(err);
        errorMsg.classList.add('show');
        btn.disabled = false;
        btn.textContent = 'CRIAR CONTA E COMPETIR';
    }
}

// Eventos (antes eram atributos on* no HTML)
document.querySelectorAll('.genero-pill').forEach(function(btn) {
    btn.addEventListener('click', function() { selecionarGenero(btn.dataset.val, btn); });
});
document.getElementById('senha').addEventListener('input', function() { contarSenha(); });
document.getElementById('btnCriar').addEventListener('click', function() { criarConta(); });
