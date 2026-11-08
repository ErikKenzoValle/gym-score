// Script da página login.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
// Redireciona se já autenticado: quem decide é o cookie (/usuarios/me), não o cache local
if (Auth.logado()) {
    Auth.verificar().then(function(u) {
        if (u) window.location.href = Auth.destinoPosLogin(u.perfil);
    }, function() {});
}
checkLGPDConsent();

// ?next=/academia ou /instrutor pré-seleciona a aba do perfil (aplicado depois que switchTab existir)
var _nextAba = (function() {
    var n = new URLSearchParams(location.search).get('next') || '';
    return n.indexOf('/academia') === 0 ? 'academia' : n.indexOf('/instrutor') === 0 ? 'instrutor' : null;
})();

var _activeTab = 'atleta';

var _tabConfig = {
    atleta:    { emailLabel: 'E-MAIL', emailPlaceholder: 'email@exemplo.com', cadastroHref: '/signup',            showCadastro: true },
    academia:  { emailLabel: 'E-MAIL', emailPlaceholder: 'email@empresa.com', cadastroHref: '/signup-academia',   showCadastro: true },
    instrutor: { emailLabel: 'E-MAIL', emailPlaceholder: 'email@exemplo.com', cadastroHref: '/signup', showCadastro: true, dica: 'Instrutor começa com uma conta de atleta: crie a conta, entre e ative o perfil de instrutor com o seu CREF.' }
};

function switchTab(tab) {
    _activeTab = tab;
    // Atualiza data-perfil do body para CSS vars mudarem
    document.body.dataset.perfil = tab;

    document.querySelectorAll('.tab-pill').forEach(function(b) {
        var isActive = b.dataset.tab === tab;
        b.classList.toggle('ativo', isActive);
        b.setAttribute('aria-selected', isActive);
    });

    var cfg = _tabConfig[tab];
    document.getElementById('emailLabel').textContent = cfg.emailLabel;
    document.getElementById('email').placeholder = cfg.emailPlaceholder;

    // Botão cadastrar
    var btnCad = document.getElementById('btnCadastrar');
    if (cfg.showCadastro) {
        btnCad.style.display = '';
        btnCad.href = cfg.cadastroHref;
    } else {
        btnCad.style.display = 'none';
    }

    var dica = document.getElementById('dicaTab');
    dica.textContent = cfg.dica || '';
    dica.hidden = !cfg.dica;
    btnCad.textContent = tab === 'instrutor' ? 'CRIAR CONTA' : 'CADASTRAR';

    document.getElementById('errorMsg').classList.remove('show');
}

function toggleSenha() {
    var input = document.getElementById('password');
    var svg   = document.getElementById('eyeIcon');
    var showing = input.type === 'text';
    input.type = showing ? 'password' : 'text';
    // Alterna entre olho aberto e riscado
    svg.innerHTML = showing
        ? '<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/>'
        : '<path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94"/>' +
          '<path d="M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19"/>' +
          '<line x1="1" y1="1" x2="23" y2="23"/>';
}

document.getElementById('loginForm').addEventListener('submit', async function(e) {
    e.preventDefault();
    var btn      = document.getElementById('submitBtn');
    var errorMsg = document.getElementById('errorMsg');
    errorMsg.classList.remove('show');
    btn.disabled = true;
    btn.textContent = 'Entrando...';

    try {
        var resp = await API.login(
            document.getElementById('email').value.trim(),
            document.getElementById('password').value
        );
        var usuario = resp.data.usuario;
        var perfil  = usuario.perfil || 'atleta';

        if (_activeTab !== perfil) {
            // O backend já emitiu o cookie jwt: revoga a sessão antes de recusar
            try { await fetch('/api/logout', { method: 'POST', credentials: 'same-origin' }); } catch (e2) {}
            throw new Error('Esta conta é do tipo "' + perfil + '". Selecione a aba correta.');
        }

        // O token fica só no cookie HttpOnly; guarda o mínimo para exibição
        Auth.saveUser(usuario);
        window.location.href = Auth.destinoPosLogin(perfil);
    } catch (err) {
        if (err.codigo === 'conta_banida') {
            btn.disabled = false;
            btn.textContent = 'ENTRAR';
            mostrarBanido(err.data || {}, document.getElementById('email').value.trim(), document.getElementById('password').value);
            return;
        }
        errorMsg.textContent = mensagemErro(err);
        errorMsg.classList.add('show');
        btn.disabled = false;
        btn.textContent = 'ENTRAR';
    }
});

if (_nextAba) switchTab(_nextAba);

// Eventos (antes eram atributos on* no HTML)
document.querySelectorAll('.tab-pill[data-tab]').forEach(function(btn) {
    btn.addEventListener('click', function() { switchTab(btn.dataset.tab); });
});
document.getElementById('btnToggleSenha').addEventListener('click', function() { toggleSenha(); });

// ─── Conta banida (Q1) ─────────────────────────────────────────────────────
// O login recusado devolve 403 conta_banida com {id_punicao, motivo, banido_em, pode_recorrer, recorrer_ate, recurso}.
// O recurso vai por POST /api/recursos/banimento com e-mail e senha (reconferidos), sem abrir sessão.
// E-mail e senha ficam só nesta closure enquanto o cartão está aberto.
function mostrarBanido(info, email, senha) {
    var box = document.getElementById('banidoBox');
    var esconder = [document.getElementById('loginForm'), document.querySelector('.tab-pills'), document.getElementById('btnsSecundarios'), document.getElementById('dicaTab')];
    esconder.forEach(function(el) { if (el) el.hidden = true; });
    box.hidden = false;

    function voltar() {
        email = senha = null;
        limpar(box);
        box.hidden = true;
        esconder.forEach(function(el) { if (el && el.id !== 'dicaTab') el.hidden = false; });
        document.getElementById('password').value = '';
        document.getElementById('password').focus();
    }

    function render(i) {
        limpar(box);
        box.appendChild(h('h2', { tabindex: '-1', id: 'banidoTitulo', text: 'Conta banida' }));
        box.appendChild(h('p', { text: 'Esta conta foi banida' + (i.banido_em ? ' em ' + fmtDataHora(i.banido_em) : '') + ' e não pode entrar no GymScore.' }));
        if (i.motivo) box.appendChild(h('dl', { class: 'md-dl' }, [h('dt', { text: 'Motivo' }), h('dd', { text: i.motivo })]));
        var r = i.recurso;
        if (r) {
            box.appendChild(h('div', { class: 'md-acoes' }, [Moderacao.selo('Recurso ' + (Moderacao.STATUS_RECURSO[r.status] || r.status).toLowerCase(), r.status)]));
            box.appendChild(h('p', { class: 'md-item-meta', text: 'Recurso enviado em ' + fmtDataHora(r.criado_em) + '.' }));
            if (r.julgado_em) box.appendChild(h('p', { class: 'md-nota ' + (r.status === 'deferido' ? 'ok' : 'erro'),
                text: (r.status === 'deferido' ? 'Recurso deferido: o banimento foi revogado. Você já pode entrar.' : 'Recurso indeferido.') + (r.decisao_texto ? ' ' + r.decisao_texto : '') }));
            else box.appendChild(h('p', { class: 'md-nota', text: 'A equipe GymScore está analisando seu recurso. Volte aqui com seu e-mail e senha para ver a resposta.' }));
        } else if (i.pode_recorrer) {
            box.appendChild(h('p', { text: 'Você pode recorrer' + (i.recorrer_ate ? ' até ' + fmtDataHora(i.recorrer_ate) : '') + '. Só é possível um recurso.' }));
            box.appendChild(h('div', { class: 'md-acoes' }, h('button', { type: 'button', class: 'gs-btn gs-btn-primary', text: 'Recorrer do banimento', onClick: recorrer })));
        } else {
            box.appendChild(h('p', { class: 'md-nota', text: 'O prazo para recorrer terminou. Se precisar, fale com o suporte.' }));
        }
        box.appendChild(h('div', { class: 'md-acoes' }, [
            r && !r.julgado_em ? h('button', { type: 'button', class: 'gs-btn gs-btn-secondary', text: 'Atualizar andamento', onClick: consultar }) : null,
            h('button', { type: 'button', class: 'gs-btn gs-btn-secondary', text: 'Voltar', onClick: voltar })
        ]));
        var t = document.getElementById('banidoTitulo');
        if (t) t.focus();
    }

    async function recorrer() {
        var r = await Moderacao.textoModal({
            titulo: 'Recorrer do banimento',
            mensagem: 'Explique por que o banimento deve ser revisto. Seu e-mail e senha são conferidos de novo no envio; nenhuma sessão é aberta.',
            rotulo: 'Seu recurso', min: 20, max: 2000, confirmar: 'Enviar recurso',
            executar: function(texto) { return API.recursoBanimento(email, senha, texto); }
        });
        if (!r) return;
        showToast('Recurso enviado.', 'success');
        render(r.data || {});
    }

    async function consultar(e) {
        var b = e && e.currentTarget;
        if (b) btnLoading(b, true);
        try { render((await API.recursoBanimento(email, senha)).data || {}); }
        catch (err) {
            if (b) btnLoading(b, false);
            if (err.status === 409) { showToast('O banimento não está mais vigente. Tente entrar.', 'success'); voltar(); return; }
            showToast(mensagemErro(err), 'error');
        }
    }

    render(info);
}

// Sessão derrubada por banimento (_req manda para /login?banido=1)
if (new URLSearchParams(location.search).get('banido') === '1') {
    var _msgBan = document.getElementById('errorMsg');
    _msgBan.textContent = 'Sua conta foi banida. Entre com seu e-mail e senha para ver o motivo e recorrer.';
    _msgBan.classList.add('show');
}
