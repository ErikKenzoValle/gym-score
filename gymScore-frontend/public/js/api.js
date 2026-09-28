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
    },
    // Rebusca o usuário logado na API e atualiza o cache local (elo/pontos/saldo mudam com o tempo)
    refresh: async function() {
        var cached = Auth.getUser();
        if (!cached || !cached.id) return cached;
        try {
            var resp = await API.buscarUsuario(cached.id);
            if (resp.data) {
                Auth.save(Auth.getToken(), resp.data);
                return resp.data;
            }
        } catch (e) { /* mantém dados em cache se a rebusca falhar */ }
        return cached;
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

// ─── Perfil helpers ──────────────────────────────────────────────────────────
function setPerfilAtual(perfil) {
    document.body.dataset.perfil = perfil;
}

// ─── Bottom nav ───────────────────────────────────────────────────────────────
// SVGs inline 24x24 stroke-based (Feather-like)
var _SVG = {
    feed:       '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 9l9-7 9 7v11a2 2 0 01-2 2H5a2 2 0 01-2-2z"/><polyline points="9 22 9 12 15 12 15 22"/></svg>',
    ingressos:  '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="2" y="7" width="20" height="14" rx="2"/><path d="M16 3l-4 4-4-4"/><line x1="8" y1="12" x2="8" y2="16"/><line x1="16" y1="12" x2="16" y2="16"/></svg>',
    amigos:     '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M17 21v-2a4 4 0 00-4-4H5a4 4 0 00-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 00-3-3.87"/><path d="M16 3.13a4 4 0 010 7.75"/></svg>',
    carteira:   '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="2" y="5" width="20" height="14" rx="2"/><line x1="2" y1="10" x2="22" y2="10"/></svg>',
    perfil:     '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 21v-2a4 4 0 00-4-4H8a4 4 0 00-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>',
    dashboard:  '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/></svg>',
    criar:      '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="10"/><line x1="12" y1="8" x2="12" y2="16"/><line x1="8" y1="12" x2="16" y2="12"/></svg>',
    instrutores:'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M17 21v-2a4 4 0 00-4-4H5a4 4 0 00-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 00-3-3.87"/><path d="M16 3.13a4 4 0 010 7.75"/></svg>',
    auditoria:  '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="8" r="7"/><polyline points="8.21 13.89 7 23 12 20 17 23 15.79 13.88"/></svg>',
    agenda:     '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="3" y="4" width="18" height="18" rx="2"/><line x1="16" y1="2" x2="16" y2="6"/><line x1="8" y1="2" x2="8" y2="6"/><line x1="3" y1="10" x2="21" y2="10"/></svg>',
    checkin:    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="23 4 23 8 19 8"/><polyline points="1 20 1 16 5 16"/><path d="M3.51 9a9 9 0 0114.85-3.36L23 10"/><path d="M20.49 15a9 9 0 01-14.85 3.36L1 14"/></svg>',
    podio:      '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="14 9 9 9 9 4"/><path d="M20 4h-6l-4 5h-6a2 2 0 000 4h3l1 7h8l1-7h3a2 2 0 000-4h-1L20 4z"/></svg>'
};

// Navs por perfil
var _NAV_LINKS = {
    atleta: [
        { href: '/menu',      label: 'Feed',      icon: 'feed'      },
        { href: '/ingressos', label: 'Ingressos', icon: 'ingressos' },
        { href: '/amigos',    label: 'Amigos',    icon: 'amigos'    },
        { href: '/depositar', label: 'Carteira',  icon: 'carteira'  },
        { href: '/perfil',    label: 'Perfil',    icon: 'perfil'    }
    ],
    academia: [
        { href: '/academia',              label: 'Dashboard',  icon: 'dashboard'   },
        { href: '/academia/criar-desafio',label: 'Criar',      icon: 'criar'       },
        { href: '/academia/instrutores',  label: 'Instrutores',icon: 'instrutores' },
        { href: '/academia/podio',        label: 'Auditoria',  icon: 'auditoria'   },
        { href: '/perfil',                label: 'Perfil',     icon: 'perfil'      }
    ],
    instrutor: [
        { href: '/instrutor',         label: 'Agenda',   icon: 'agenda'   },
        { href: '/instrutor/checkin', label: 'Check-in', icon: 'checkin'  },
        { href: '/instrutor/podio',   label: 'Pódio',    icon: 'podio'    },
        { href: '/perfil',            label: 'Perfil',   icon: 'perfil'   }
    ]
};

function renderBottomNav(activePage) {
    var user = Auth.getUser();
    var perfil = (user && user.perfil) ? user.perfil : 'atleta';
    var links = _NAV_LINKS[perfil] || _NAV_LINKS.atleta;

    // Seta data-perfil no body para CSS vars funcionarem
    if (user && user.perfil) document.body.dataset.perfil = user.perfil;

    var nav = document.createElement('nav');
    nav.id = 'bottomNav';
    nav.setAttribute('aria-label', 'Navegação principal');

    links.forEach(function(l) {
        var isActive = activePage && (activePage === l.href || activePage.startsWith(l.href + '?'));
        var a = document.createElement('a');
        a.href = l.href;
        a.setAttribute('aria-label', l.label);
        a.setAttribute('aria-current', isActive ? 'page' : 'false');
        a.innerHTML = (_SVG[l.icon] || '') + '<span>' + l.label + '</span>';
        if (isActive) a.classList.add('active');
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

    // Sessão expirada/token inválido numa rota protegida: desloga e manda pro login
    // (não se aplica a login/cadastro sem token, onde 401 é só "credenciais erradas")
    if (res.status === 401 && token) {
        Auth.clear();
        window.location.href = '/login';
        throw new Error('Sessão expirada. Faça login novamente.');
    }

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
    cadastrarAcademia: function(dados) {
        return _req('/api/academias', { method: 'POST', body: JSON.stringify(dados) });
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
        var user = Auth.getUser();
        return _req('/api/pagamento/pix', {
            method: 'POST',
            body: JSON.stringify({ valor: valor, cpf: cpf, id_usuario: user ? user.id : 0 })
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
    radarTreinos: function() { return _req('/api/treinos/radar'); },

    // Usuários — novos endpoints
    buscarUsuarios: function(q) { return _req('/api/usuarios/buscar?q=' + encodeURIComponent(q)); },
    perfilPublico: function(id) { return _req('/api/usuarios/' + id + '/publico'); },
    historicoDesafios: function(id) { return _req('/api/usuarios/' + id + '/historico'); },
    heartbeat: function() { return _req('/api/usuarios/ultima-vez', { method: 'PATCH' }); },

    // Instrutores
    cadastrarInstrutor: function(dados) {
        return _req('/api/instrutores', { method: 'POST', body: JSON.stringify(dados) });
    },

    // Pagamento — extrato e saque
    extrato: function() { return _req('/api/pagamento/extrato'); },
    solicitarSaque: function(valor, chavePix) {
        return _req('/api/pagamento/saque', {
            method: 'POST',
            body: JSON.stringify({ valor: valor, chave_pix: chavePix })
        });
    },
    listarSaques: function() { return _req('/api/pagamento/saque'); },

    // Academia
    dashboardAcademia: function() { return _req('/api/academia/dashboard'); },
    listarPresentes: function(idDesafio) { return _req('/api/desafios/' + idDesafio + '/presentes'); },
    marcarPresenca: function(idDesafio, idUsuario) {
        return _req('/api/desafios/' + idDesafio + '/presenca', {
            method: 'POST',
            body: JSON.stringify({ id_usuario: idUsuario })
        });
    },
    lancarPodio: function(idDesafio, posicoes) {
        return _req('/api/desafios/' + idDesafio + '/podio', {
            method: 'POST',
            body: JSON.stringify({ posicoes: posicoes })
        });
    },
    consultarPodio: function(idDesafio) { return _req('/api/desafios/' + idDesafio + '/podio'); },
    avaliarDesafio: function(idDesafio, nota) {
        return _req('/api/desafios/' + idDesafio + '/avaliar', {
            method: 'POST',
            body: JSON.stringify({ nota: nota })
        });
    },

    // IA — Recomendados
    recomendados: function() { return _req('/api/desafios/recomendados'); },

    // Ingressos do atleta (Sprint 5)
    ingressos: function() { return _req('/api/usuarios/ingressos'); },

    // Agenda do instrutor (Sprint 5)
    agendaInstrutor: function() { return _req('/api/instrutor/agenda'); },

    // Instrutores da academia (Sprint 5)
    listarInstrutoresAcademia: function() { return _req('/api/instrutores'); }
};
