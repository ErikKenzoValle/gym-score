// GymScore — utilitários de auth, privacidade (LGPD) e chamadas à API Go

// ─── Auth ────────────────────────────────────────────────────────────────────
var Auth = {
    getToken: function() { return localStorage.getItem('gym_token'); },
    getUser: function() {
        try { return JSON.parse(localStorage.getItem('gym_user')); } catch(e) { return null; }
    },
    save: function(token, user) {
        localStorage.setItem('gym_token', token);
        localStorage.setItem('gym_user', JSON.stringify(user));
    },
    clear: function() {
        localStorage.removeItem('gym_token');
        localStorage.removeItem('gym_user');
    },
    logout: function() {
        Auth.clear();
        window.location.href = '/login';
    },
    require: function() {
        if (!Auth.getToken()) {
            window.location.href = '/login';
            return false;
        }
        return true;
    }
};

// ─── Privacidade (LGPD) ───────────────────────────────────────────────────────
var Privacy = {
    // true = dados ocultos (padrão LGPD — optar por mostrar, não por esconder)
    isHidden: function() {
        return localStorage.getItem('gym_privacy') !== 'visible';
    },
    setVisible: function(visible) {
        localStorage.setItem('gym_privacy', visible ? 'visible' : 'hidden');
        document.dispatchEvent(new CustomEvent('privacyChange', { detail: { hidden: !visible } }));
    },
    toggle: function() {
        Privacy.setVisible(Privacy.isHidden());
    },

    // Mascara um valor conforme o tipo
    mask: function(value, type) {
        if (!Privacy.isHidden()) return value;
        switch(type) {
            case 'cpf':      return '***.***.***-**';
            case 'email':    return '****@****.***';
            case 'date':     return '**/**/****';
            case 'currency': return 'R$ **,**';
            case 'name':     return value ? value.split(' ')[0] + ' ***' : '***';
            default:         return '***';
        }
    },

    // Atualiza todos os elementos com data-private no DOM
    applyToDOM: function() {
        document.querySelectorAll('[data-private]').forEach(function(el) {
            var original = el.getAttribute('data-original') || el.textContent;
            el.setAttribute('data-original', original);
            el.textContent = Privacy.mask(original, el.getAttribute('data-private'));
        });
    },

    // Renderiza o botão de toggle de privacidade em um elemento
    renderToggle: function(containerId) {
        var el = document.getElementById(containerId);
        if (!el) return;
        function render() {
            var hidden = Privacy.isHidden();
            el.innerHTML = '<button onclick="Privacy.toggle()" style="background:' +
                (hidden ? '#555' : '#27ae60') +
                ';padding:6px 14px;border:none;border-radius:6px;color:#fff;cursor:pointer;font-size:0.85em;">' +
                (hidden ? '👁 Mostrar dados' : '🙈 Ocultar dados') + '</button>';
        }
        render();
        document.addEventListener('privacyChange', function() { render(); Privacy.applyToDOM(); });
    }
};

// ─── Toast ───────────────────────────────────────────────────────────────────
function showToast(msg, type) {
    var t = document.getElementById('gsToast');
    if (!t) {
        t = document.createElement('div');
        t.id = 'gsToast';
        document.body.appendChild(t);
    }
    t.textContent = msg;
    t.className = type || '';
    t.classList.add('show');
    clearTimeout(t._timer);
    t._timer = setTimeout(function() { t.classList.remove('show'); }, 2800);
}

// ─── Bottom nav ───────────────────────────────────────────────────────────────
var _NAV_SVG = {
    '/menu':     '<svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M10 20v-6h4v6h5v-8h3L12 3 2 12h3v8z"/></svg>',
    '/desafios': '<svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M19 5h-2V3H7v2H5c-1.1 0-2 .9-2 2v1c0 2.55 1.92 4.63 4.39 4.94.56 2.29 2.28 4.09 4.61 4.55V19H9v2h6v-2h-2v-2.51c2.33-.46 4.05-2.26 4.61-4.55C19.08 11.63 21 9.55 21 7V6c0-1.1-.9-2-2-2zM5 8V7h2v3.82C5.84 10.4 5 9.3 5 8zm14 0c0 1.3-.84 2.4-2 2.82V7h2v1z"/></svg>',
    '/amigos':   '<svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M16 11c1.66 0 3-1.34 3-3s-1.34-3-3-3-3 1.34-3 3 1.34 3 3 3zM8 11c1.66 0 3-1.34 3-3S9.66 5 8 5 5 6.34 5 8s1.34 3 3 3zm0 2c-2.33 0-7 1.17-7 3.5V19h14v-2.5c0-2.33-4.67-3.5-7-3.5zm8 0c-.29 0-.62.02-.97.05 1.16.84 1.97 1.97 1.97 3.45V19h6v-2.5c0-2.33-4.67-3.5-7-3.5z"/></svg>',
    '/treinos':  '<svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M20.57 14.86L22 13.43 20.57 12 17 15.57 8.43 7 12 3.43 10.57 2 9.14 3.43 7.71 2 5.57 4.14 4.14 2.71 2.71 4.14l1.43 1.43L2 7.71l1.43 1.43L2 10.57 3.43 12 7 8.43 15.57 17 12 20.57 13.43 22l1.43-1.43L16.29 22l2.14-2.14 1.43 1.43 1.43-1.43-1.43-1.43L22 16.29z"/></svg>',
    '/perfil':   '<svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 12c2.21 0 4-1.79 4-4s-1.79-4-4-4-4 1.79-4 4 1.79 4 4 4zm0 2c-2.67 0-8 1.34-8 4v2h16v-2c0-2.66-5.33-4-8-4z"/></svg>'
};

function renderBottomNav(activePage) {
    var nav = document.createElement('nav');
    nav.id = 'bottomNav';
    nav.setAttribute('aria-label', 'Navegação principal');
    var links = [
        { href:'/menu',     label:'Início'   },
        { href:'/desafios', label:'Desafios' },
        { href:'/amigos',   label:'Amigos'   },
        { href:'/treinos',  label:'Treinos'  },
        { href:'/perfil',   label:'Perfil'   },
    ];
    links.forEach(function(l) {
        var a = document.createElement('a');
        a.href = l.href;
        a.setAttribute('aria-label', l.label);
        a.setAttribute('aria-current', l.href === activePage ? 'page' : 'false');
        a.innerHTML = (_NAV_SVG[l.href] || '') + '<span>' + l.label + '</span>';
        if (l.href === activePage) a.classList.add('active');
        nav.appendChild(a);
    });
    document.body.appendChild(nav);
    document.body.classList.add('has-bottom-nav');
}

// ─── Loading state em botão ───────────────────────────────────────────────────
function btnLoading(btn, isLoading) {
    if (isLoading) {
        btn.disabled = true;
        btn._origText = btn.innerHTML;
        btn.classList.add('loading');
        btn.innerHTML = '';
    } else {
        btn.disabled = false;
        btn.classList.remove('loading');
        if (btn._origText !== undefined) btn.innerHTML = btn._origText;
    }
}

// ─── Consentimento LGPD ──────────────────────────────────────────────────────
function checkLGPDConsent() {
    if (localStorage.getItem('gym_lgpd_consent') === 'accepted') return;

    var banner = document.createElement('div');
    banner.id = 'lgpdBanner';
    banner.style.cssText = [
        'position:fixed', 'bottom:0', 'left:0', 'right:0', 'z-index:9999',
        'background:#1a1a1a', 'color:#f4f4f4', 'padding:18px 24px',
        'border-top:2px solid #4CAF50', 'font-size:0.9em',
        'display:flex', 'gap:16px', 'align-items:center', 'flex-wrap:wrap'
    ].join(';');

    banner.innerHTML =
        '<p style="margin:0;flex:1;min-width:220px;">' +
        '<strong>Seus dados, seu controle.</strong> O GymScore coleta nome, e-mail, CPF e ' +
        'data de nascimento para operação da plataforma, conforme a ' +
        '<a href="/privacidade" style="color:#4CAF50;">Política de Privacidade</a> (LGPD). ' +
        'Dados sensíveis ficam <strong>ocultos por padrão</strong>.</p>' +
        '<div style="display:flex;gap:10px;flex-shrink:0;">' +
        '<button onclick="acceptLGPD()" style="background:#4CAF50;color:#fff;border:none;' +
        'padding:8px 20px;border-radius:6px;cursor:pointer;font-weight:bold;">Entendi e aceito</button>' +
        '<a href="/privacidade" style="background:transparent;color:#4CAF50;border:1px solid #4CAF50;' +
        'padding:8px 14px;border-radius:6px;cursor:pointer;text-decoration:none;font-size:0.85em;">' +
        'Ver política</a>' +
        '</div>';

    document.body.appendChild(banner);
}

function acceptLGPD() {
    localStorage.setItem('gym_lgpd_consent', 'accepted');
    var banner = document.getElementById('lgpdBanner');
    if (banner) banner.remove();
}

// ─── HTTP helper ─────────────────────────────────────────────────────────────
async function _req(path, options) {
    options = options || {};
    var token = Auth.getToken();
    var headers = Object.assign({ 'Content-Type': 'application/json' }, options.headers || {});
    if (token) headers['Authorization'] = 'Bearer ' + token;

    var res = await fetch(path, Object.assign({}, options, { headers: headers }));
    var json = await res.json();

    if (!res.ok) throw new Error(json.error || json.message || 'Erro na requisição');
    return json;
}

// ─── API ─────────────────────────────────────────────────────────────────────
var API = {
    // Auth
    login: function(email, senha) {
        return _req('/api/login', { method: 'POST', body: JSON.stringify({ email: email, senha: senha }) });
    },
    cadastrar: function(dados) {
        return _req('/api/usuarios', { method: 'POST', body: JSON.stringify(dados) });
    },
    alterarSenha: function(senhaAtual, novaSenha) {
        return _req('/api/usuarios/senha', {
            method: 'PATCH',
            body: JSON.stringify({ senha_atual: senhaAtual, nova_senha: novaSenha })
        });
    },
    recuperarSenha: function(email, cpf, novaSenha) {
        return _req('/api/usuarios/recuperar-senha', {
            method: 'POST',
            body: JSON.stringify({ email: email, cpf: cpf, nova_senha: novaSenha })
        });
    },

    // Usuários
    buscarUsuario: function(id) { return _req('/api/usuarios/' + id); },
    listarUsuarios: function() { return _req('/api/usuarios'); },
    atualizarPerfil: function(dados) {
        return _req('/api/usuarios/perfil', { method: 'PATCH', body: JSON.stringify(dados) });
    },

    // Desafios
    listarDesafios: function() { return _req('/api/desafios/view'); },
    listarDesafiosUsuario: function(id) { return _req('/api/desafios/' + id); },
    criarDesafio: function(dados) {
        return _req('/api/desafios', { method: 'POST', body: JSON.stringify(dados) });
    },
    aceitarDesafio: function(id_desafio, id_usuario) {
        return _req('/api/desafios/aceitar_desafio', {
            method: 'POST',
            body: JSON.stringify({ id_desafio: id_desafio, id_usuario: id_usuario })
        });
    },
    cancelarDesafio: function(id_desafio) {
        return _req('/api/desafios/cancelar', {
            method: 'POST',
            body: JSON.stringify({ id_desafio: id_desafio })
        });
    },
    iniciarDesafio: function(id_desafio) {
        return _req('/api/desafios/iniciar', {
            method: 'POST',
            body: JSON.stringify({ id_desafio: id_desafio })
        });
    },
    encerrarDesafio: function(id_desafio, id_vencedor) {
        return _req('/api/desafios/encerrar', {
            method: 'POST',
            body: JSON.stringify({ id_desafio: id_desafio, id_vencedor: id_vencedor })
        });
    },

    // PIX / Depósito
    gerarPix: function(valor, cpf) {
        return _req('/api/pagamento/pix', {
            method: 'POST',
            body: JSON.stringify({ valor: valor, cpf: cpf })
        });
    },
    consultarPagamento: function(asaasId) {
        return _req('/api/pagamento/pix/' + asaasId);
    },
    simularPagamento: function(asaasId) {
        return _req('/api/pagamento/pix/' + asaasId + '/simular', { method: 'POST' });
    },

    // Amigos
    listarAmigos: function(id) { return _req('/api/amigos/' + id); },
    adicionarAmigo: function(id_usuario, id_amigo) {
        return _req('/api/amigos/adicionar', {
            method: 'POST',
            body: JSON.stringify({ id_usuario: id_usuario, id_amigo: id_amigo })
        });
    },
    aceitarAmizade: function(id_usuario, id_amigo) {
        return _req('/api/amigos/aceitar', {
            method: 'POST',
            body: JSON.stringify({ id_usuario: id_usuario, id_amigo: id_amigo })
        });
    },
    removerAmigo: function(id_usuario, id_amigo) {
        return _req('/api/amigos/remover', {
            method: 'POST',
            body: JSON.stringify({ id_usuario: id_usuario, id_amigo: id_amigo })
        });
    },

    // Treinos
    listarTreinos: function() { return _req('/api/treinos'); },
    criarTreino: function(dados) {
        return _req('/api/treinos', { method: 'POST', body: JSON.stringify(dados) });
    },
    buscarTreino: function(id) { return _req('/api/treinos/' + id); },
    deletarTreino: function(id) {
        return _req('/api/treinos/' + id, { method: 'DELETE' });
    },
    importarTreino: function(codigo) {
        return _req('/api/treinos/importar', { method: 'POST', body: JSON.stringify({ codigo: codigo }) });
    },
    concluirExercicio: function(id_treino, grupo_muscular) {
        return _req('/api/treinos/concluir', {
            method: 'POST',
            body: JSON.stringify({ id_treino: id_treino, grupo_muscular: grupo_muscular })
        });
    },
    radarTreinos: function() { return _req('/api/treinos/radar'); }
};
