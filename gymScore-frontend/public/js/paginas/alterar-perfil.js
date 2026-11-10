// Script da página alterar-perfil.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
if (!Auth.require()) throw new Error('unauthenticated');

var user = Auth.getUser();
var _ehAtleta = !user || !user.perfil || user.perfil === 'atleta';

// Popula os campos com o perfil completo (/usuarios/me; o cache local só tem id/nome/perfil)
function preencher(user) {
    if (!user) return;
    _ehAtleta = !user.perfil || user.perfil === 'atleta';
    document.getElementById('nome').value = user.nome || '';
    document.getElementById('sobrenome').value = user.sobrenome || '';
    if (_ehAtleta) {
        var generoSel = document.getElementById('genero');
        for (var i = 0; i < generoSel.options.length; i++) {
            if (generoSel.options[i].value === user.genero) {
                generoSel.selectedIndex = i;
                break;
            }
        }
    } else {
        // Gênero não se aplica a contas de academia/instrutor
        document.getElementById('generoGroup').style.display = 'none';
    }
}
preencher(user);
Auth.refresh().then(function(u) {
    preencher(u);
    configurarNascimento(u);
    if (u && u.perfil === 'academia') carregarAcademia();
});

// ─── Rodada 6: data de nascimento (só para quem ainda não tem; PATCH recusa troca) ───────────────
var _params = new URLSearchParams(location.search);
var _pedirNascimento = false;
function _isoLocal(dt) {
    return dt.getFullYear() + '-' + String(dt.getMonth() + 1).padStart(2, '0') + '-' + String(dt.getDate()).padStart(2, '0');
}
// Chamado só com o perfil completo de /usuarios/me (o cache local não traz a data)
function configurarNascimento(u) {
    var grupo = document.getElementById('nascimentoGroup');
    _pedirNascimento = !!(u && _ehAtleta && !u.data_nascimento);
    grupo.hidden = !_pedirNascimento;
    if (!_pedirNascimento) return;
    var campo = document.getElementById('dataNascimento');
    var hoje = new Date();
    campo.max = _isoLocal(hoje);
    campo.min = _isoLocal(new Date(hoje.getFullYear() - 120, hoje.getMonth(), hoje.getDate()));
    if (_params.get('nascimento') === '1') campo.focus();
}

// Volta para a página de origem (ex.: o desafio que pediu a data) — só caminhos internos
function destinoDepois() {
    var v = _params.get('voltar') || '';
    return /^\/(?!\/)[^\s\\]*$/.test(v) ? v : '/perfil';
}

async function submitForm(event) {
    event.preventDefault();

    var msgEl   = document.getElementById('formMsg');
    var btn     = document.getElementById('submitBtn');
    var nome    = document.getElementById('nome').value.trim();
    var sobrenome = document.getElementById('sobrenome').value.trim();
    var genero  = _ehAtleta ? document.getElementById('genero').value : '';
    var campoNasc = document.getElementById('dataNascimento');
    var nascimento = _pedirNascimento ? campoNasc.value : '';

    msgEl.className = 'gs-msg';
    erroCampo(campoNasc, '');
    campoNasc.setAttribute('aria-describedby', 'nascimentoAjuda');

    if (!nome || !sobrenome) {
        msgEl.className = 'gs-msg error show';
        msgEl.textContent = 'Nome e sobrenome são obrigatórios.';
        return;
    }
    if (nascimento && (nascimento > campoNasc.max || nascimento < campoNasc.min)) {
        erroCampo(campoNasc, 'Informe uma data de nascimento válida (não pode ser no futuro).');
        campoNasc.focus();
        return;
    }
    if (_params.get('nascimento') === '1' && _pedirNascimento && !nascimento) {
        erroCampo(campoNasc, 'Informe sua data de nascimento para entrar no desafio.');
        campoNasc.focus();
        return;
    }

    btnLoading(btn, true);

    try {
        var dados = { nome: nome, sobrenome: sobrenome, genero: genero };
        if (nascimento) dados.data_nascimento = nascimento;
        var resp = await API.atualizarPerfil(dados);
        var usuarioAtualizado = resp.data || {};

        // Atualiza localStorage mantendo campos que o backend não devolve (saldo, cpf, etc.)
        var userAtual = Auth.getUser() || {};
        var userMerged = Object.assign({}, userAtual, usuarioAtualizado);
        Auth.saveUser(userMerged);

        msgEl.className = 'gs-msg success show';
        var destino = destinoDepois();
        msgEl.textContent = 'Perfil atualizado! ' + (destino === '/perfil' ? 'Redirecionando...' : 'Voltando para o desafio...');

        setTimeout(function() {
            window.location.href = destino;
        }, 1500);
    } catch (err) {
        msgEl.className = 'gs-msg error show';
        msgEl.textContent = mensagemErro(err);
        btnLoading(btn, false);
    }
}

renderBottomNav('/perfil');

// Eventos (antes eram atributos on* no HTML)
document.getElementById('editProfileForm').addEventListener('submit', submitForm);

// ─── Rodada 4: endereço estruturado e telefone da academia (GET/PATCH /api/academia/perfil) ──────────
var _academiaCarregada = false;
async function carregarAcademia() {
    if (_academiaCarregada) return;
    _academiaCarregada = true;
    var secao = document.getElementById('academiaSecao');
    var aviso = document.getElementById('academiaAviso');
    var form = document.getElementById('academiaForm');
    secao.hidden = false;
    aviso.hidden = false;
    aviso.textContent = 'Carregando dados da academia...';
    var a;
    try { a = (await API.academiaPerfil()).data || {}; }
    catch (e) {
        aviso.textContent = e.statusCadastro
            ? 'O endereço pode ser editado depois que a equipe GymScore aprovar o cadastro da academia.'
            : 'Não foi possível carregar os dados da academia. ' + mensagemErro(e);
        return;
    }
    var end = Endereco.montar(document.getElementById('acEndereco'), { prefixo: 'ac', valores: a });
    var tel = document.getElementById('acTelefone');
    Mask.aplicar(tel, 'telefone');
    tel.value = Mask.telefone(a.telefone || '');
    // Academias antigas só têm o texto livre: mostra para ajudar a preencher os campos
    if (!a.cep && a.endereco) aviso.textContent = 'Endereço atual (texto antigo): ' + a.endereco + '. Preencha os campos abaixo para atualizar.';
    else aviso.hidden = true;
    form.hidden = false;

    form.addEventListener('submit', async function(ev) {
        ev.preventDefault();
        var msg = document.getElementById('academiaMsg');
        var btn = document.getElementById('academiaBtn');
        msg.className = 'gs-msg';
        if (tel.value.trim() && !Valid.telefone(tel.value)) { erroCampo(tel, 'Telefone inválido. Informe DDD e número.'); tel.focus(); return; }
        erroCampo(tel, '');
        if (!end.validar()) {
            msg.className = 'gs-msg error show';
            msg.textContent = 'Complete o endereço (CEP, UF, logradouro, número, bairro e cidade).';
            return;
        }
        btnLoading(btn, true);
        try {
            var r = (await API.atualizarAcademiaPerfil(Object.assign({ telefone: tel.value.trim() }, end.valores()))).data || {};
            aviso.hidden = true;
            msg.className = 'gs-msg success show';
            msg.textContent = 'Endereço salvo' + (r.endereco ? ': ' + r.endereco : '.');
        } catch (e) {
            msg.className = 'gs-msg error show';
            msg.textContent = mensagemErro(e);
        } finally {
            btnLoading(btn, false);
        }
    });
}
