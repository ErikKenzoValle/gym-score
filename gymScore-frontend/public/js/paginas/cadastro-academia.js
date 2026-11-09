// Script da página cadastro-academia.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
function contarSenha(fieldId, counterId) {
    var len = document.getElementById(fieldId).value.length;
    var counter = document.getElementById(counterId);
    counter.textContent = len + '/8';
    counter.className = 'senha-counter' + (len >= 8 ? ' ok' : '');
}

// Endereço estruturado (rodada 4): CEP com busca automática; todos os campos editáveis
var endereco = Endereco.montar(document.getElementById('enderecoCampos'));

// Máscaras vêm de data-mask (api.js); validação inline ao sair do campo
document.getElementById('cnpj').addEventListener('blur', function() {
    erroCampo(this, this.value && !Valid.cnpj(this.value) ? 'CNPJ inválido. Confira os dígitos.' : '');
});
document.getElementById('telefone').addEventListener('blur', function() {
    erroCampo(this, this.value && !Valid.telefone(this.value) ? 'Telefone inválido. Informe DDD e número.' : '');
});
document.getElementById('email').addEventListener('blur', function() {
    erroCampo(this, this.value && !Valid.email(this.value) ? 'E-mail inválido.' : '');
});

async function cadastrarInstituicao() {
    var btn      = document.getElementById('btnCriar');
    var errorMsg = document.getElementById('errorMsg');
    errorMsg.classList.remove('show');

    var razaoSocial   = document.getElementById('razaoSocial').value.trim();
    var nomeFantasia  = document.getElementById('nomeFantasia').value.trim();
    var cnpj          = document.getElementById('cnpj').value.trim();
    var email         = document.getElementById('email').value.trim();
    var telefone      = document.getElementById('telefone').value.trim();
    var senha         = document.getElementById('senha').value;
    var confirmar     = document.getElementById('confirmarSenha').value;

    if (!razaoSocial || !nomeFantasia || !cnpj || !email || !senha) {
        errorMsg.textContent = 'Preencha todos os campos obrigatórios.';
        errorMsg.classList.add('show');
        return;
    }
    if (!Valid.cnpj(cnpj)) { errorMsg.textContent = 'CNPJ inválido. Confira os dígitos.'; errorMsg.classList.add('show'); return; }
    if (telefone && !Valid.telefone(telefone)) { errorMsg.textContent = 'Telefone inválido. Informe DDD e número.'; errorMsg.classList.add('show'); return; }
    if (!endereco.validar()) {
        errorMsg.textContent = 'Complete o endereço da academia (CEP, UF, logradouro, número, bairro e cidade).';
        errorMsg.classList.add('show');
        return;
    }
    if (senha.length < 8) {
        errorMsg.textContent = 'A senha precisa ter pelo menos 8 caracteres.';
        errorMsg.classList.add('show');
        return;
    }
    if (senha !== confirmar) {
        errorMsg.textContent = 'As senhas não coincidem.';
        errorMsg.classList.add('show');
        return;
    }

    btn.disabled = true;
    btn.textContent = 'Cadastrando...';

    try {
        // `endereco` (texto) é montado pelo backend a partir dos campos estruturados
        await API.cadastrarAcademia(Object.assign({
            nome: nomeFantasia,
            sobrenome: '',
            email: email,
            senha: senha,
            razao_social: razaoSocial,
            nome_fantasia: nomeFantasia,
            cnpj: cnpj,
            telefone: telefone
        }, endereco.valores()));
        // A conta nasce pendente: só opera depois da aprovação da equipe GymScore
        btn.textContent = 'Cadastro enviado!';
        errorMsg.className = 'gs-msg success show';
        errorMsg.setAttribute('role', 'status');
        errorMsg.textContent = 'Cadastro recebido e em análise. Você recebe uma notificação quando a equipe GymScore aprovar; ' +
            'até lá dá para entrar, mas as funções de academia ficam bloqueadas.';
        setTimeout(function() { window.location.href = '/login?next=' + encodeURIComponent('/academia'); }, 5000);
    } catch (err) {
        errorMsg.textContent = mensagemErro(err);
        errorMsg.classList.add('show');
        btn.disabled = false;
        btn.textContent = 'CADASTRAR INSTITUIÇÃO';
    }
}

// Eventos (antes eram atributos on* no HTML)
document.getElementById('senha').addEventListener('input', function() { contarSenha('senha', 'counterSenha'); });
document.getElementById('confirmarSenha').addEventListener('input', function() { contarSenha('confirmarSenha', 'counterConfirmar'); });
document.getElementById('btnCriar').addEventListener('click', function() { cadastrarInstituicao(); });
