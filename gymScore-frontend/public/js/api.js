// GymScore — utilitários de auth, privacidade (LGPD), DOM seguro, dinheiro, notificações e chamadas à API Go

// ─── Escape / criação de elementos (anti-XSS) ────────────────────────────────
// Único ponto de escape do frontend. Escapa também aspas simples e duplas.
function escHtml(s) {
    return String(s == null ? '' : s)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#39;');
}

// h(tag, attrs, children): cria elemento sem usar innerHTML.
// attrs: class, text (textContent), on<Evento> (função), data (objeto -> data-*),
//        style (objeto), demais chaves viram atributos (null/false/undefined são ignorados).
// children: string (vira nó de texto), Node ou array (aninhado, nulos ignorados).
function h(tag, attrs, children) {
    var el = document.createElement(tag);
    if (attrs) {
        Object.keys(attrs).forEach(function(k) {
            var v = attrs[k];
            if (v === null || v === undefined || v === false) return;
            if (k === 'class') el.className = v;
            else if (k === 'text') el.textContent = v;
            else if (k === 'data') Object.keys(v).forEach(function(d) { el.dataset[d] = v[d]; });
            else if (k === 'style') Object.keys(v).forEach(function(s) { el.style[s] = v[s]; });
            else if (k.indexOf('on') === 0 && typeof v === 'function') el.addEventListener(k.slice(2).toLowerCase(), v);
            else el.setAttribute(k, v === true ? '' : v);
        });
    }
    (function add(c) {
        if (c === null || c === undefined || c === false) return;
        if (Array.isArray(c)) { c.forEach(add); return; }
        el.appendChild(c.nodeType ? c : document.createTextNode(String(c)));
    })(children);
    return el;
}

// Remove todos os filhos de um elemento
function limpar(el) {
    while (el.firstChild) el.removeChild(el.firstChild);
    return el;
}

// Nome completo seguro para exibição (texto puro; use sempre com textContent/h())
function nomeCompleto(u) {
    if (!u) return 'Usuário';
    return (((u.nome || '') + ' ' + (u.sobrenome || '')).trim()) || 'Usuário';
}

// ─── Dinheiro (backend fala em centavos; reais só na tela/entrada) ───────────
// 123456 -> "R$ 1.234,56"
function fmtBRL(centavos) {
    var n = Math.round(Number(centavos) || 0);
    var neg = n < 0;
    n = Math.abs(n);
    var inteiro = String(Math.floor(n / 100)).replace(/\B(?=(\d{3})+(?!\d))/g, '.');
    var cents = String(n % 100);
    if (cents.length < 2) cents = '0' + cents;
    return (neg ? '-' : '') + 'R$ ' + inteiro + ',' + cents;
}

// "12,50" | "12.5" | "12" -> 1250. Retorna null se inválido. Sem aritmética de float.
function reaisParaCentavos(texto) {
    var s = String(texto == null ? '' : texto).trim().replace(/\s/g, '');
    if (s.indexOf(',') >= 0) s = s.replace(/\./g, '').replace(',', '.');
    var m = /^(\d+)(?:\.(\d{1,2}))?$/.exec(s);
    if (!m) return null;
    var cents = (m[2] || '') + '00';
    return parseInt(m[1], 10) * 100 + parseInt(cents.slice(0, 2), 10);
}

// ─── Datas ───────────────────────────────────────────────────────────────────
function fmtData(raw) {
    if (!raw) return '-';
    var d = new Date(raw);
    if (isNaN(d.getTime())) return '-';
    return d.toLocaleDateString('pt-BR');
}
function fmtDataHora(raw) {
    if (!raw) return '-';
    var d = new Date(raw);
    if (isNaN(d.getTime())) return '-';
    return d.toLocaleDateString('pt-BR') + ' ' + d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
}

// ─── Auth ────────────────────────────────────────────────────────────────────
// Token em localStorage (header Authorization) + cookie HttpOnly emitido pelo backend.
var Auth = {
    getToken: function() { return localStorage.getItem('gym_token'); },
    getUser: function() {
        try { return JSON.parse(localStorage.getItem('gym_user')); } catch(e) { return null; }
    },
    save: function(token, user) {
        localStorage.setItem('gym_token', token);
        localStorage.setItem('gym_user', JSON.stringify(user));
    },
    // Troca só o token (ex.: após alterar a senha, o token antigo é revogado)
    saveToken: function(token) {
        localStorage.setItem('gym_token', token);
    },
    // Atualiza só os dados do usuário (mantém o token)
    saveUser: function(user) {
        localStorage.setItem('gym_user', JSON.stringify(user));
    },
    clear: function() {
        localStorage.removeItem('gym_token');
        localStorage.removeItem('gym_user');
    },
    // Chama o endpoint (limpa o cookie HttpOnly) e depois limpa o storage
    logout: async function() {
        // Envia o token para o backend revogá-lo (o cookie HttpOnly também é limpo)
        var token = Auth.getToken();
        try {
            await fetch('/api/logout', { method: 'POST', headers: token ? { 'Authorization': 'Bearer ' + token } : {} });
        } catch(e) {}
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
    // Rebusca o perfil completo (saldo, cpf, elo, pontos...) em /usuarios/me e atualiza o cache local
    refresh: async function() {
        try {
            var resp = await API.me();
            if (resp.data) Auth.saveUser(resp.data);
            return resp.data;
        } catch(e) {
            return Auth.getUser();
        }
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
            limpar(el).appendChild(h('button', {
                type: 'button',
                class: 'privacy-toggle-btn ' + (hidden ? 'is-hidden' : 'is-visible'),
                onClick: function() { Privacy.toggle(); },
                text: hidden ? '👁 Mostrar dados' : '🙈 Ocultar dados'
            }));
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
        t.setAttribute('role', 'status');
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

// ─── Bottom nav + sino de notificações ───────────────────────────────────────
// SVGs inline 24x24 stroke-based (Feather-like) + sino (fill-based, estático)
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
    podio:      '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><polyline points="14 9 9 9 9 4"/><path d="M20 4h-6l-4 5h-6a2 2 0 000 4h3l1 7h8l1-7h3a2 2 0 000-4h-1L20 4z"/></svg>',
    sino:       '<svg viewBox="0 0 24 24" fill="currentColor" aria-hidden="true"><path d="M12 22c1.1 0 2-.9 2-2h-4c0 1.1.89 2 2 2zm6-6v-5c0-3.07-1.64-5.64-4.5-6.32V4c0-.83-.67-1.5-1.5-1.5s-1.5.67-1.5 1.5v.68C7.63 5.36 6 7.92 6 11v5l-2 2v1h16v-1l-2-2z"/></svg>'
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

function _navIcon(chave) {
    var span = document.createElement('span');
    span.className = 'nav-icon';
    span.innerHTML = _SVG[chave] || ''; // SVG estático definido acima
    return span;
}

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

    // Sino de notificações (antes de "Perfil")
    var bell = h('button', { type: 'button', id: 'bellBtn', class: 'nav-bell', 'aria-label': 'Notificações', 'aria-haspopup': 'dialog' }, [
        _navIcon('sino'),
        h('span', { id: 'bellBadge', class: 'bell-badge', hidden: true, 'aria-hidden': 'true' }),
        h('span', { text: 'Avisos' })
    ]);
    bell.addEventListener('click', function() { Notificacoes.abrir(); });
    nav.insertBefore(bell, nav.lastChild);

    document.body.appendChild(nav);
    document.body.classList.add('has-bottom-nav');
    Notificacoes.iniciar();
}

// Notificações in-app: contador por polling (30 s) + painel com lista
var Notificacoes = {
    _timer: null,
    _painel: null,
    _naoLidas: 0,

    iniciar: function() {
        if (Notificacoes._timer || !Auth.getToken()) return;
        Notificacoes.atualizarContador();
        Notificacoes._timer = setInterval(function() {
            if (document.hidden) return;
            Notificacoes.atualizarContador();
            if (Notificacoes._painel) Notificacoes.carregarLista();
        }, 30000);
    },

    atualizarContador: async function() {
        try {
            var resp = await API.notificacoesNaoLidas();
            Notificacoes._definirContador((resp.data && resp.data.nao_lidas) || 0);
        } catch(e) { /* silencioso: o sino não deve quebrar a página */ }
    },

    _definirContador: function(n) {
        Notificacoes._naoLidas = n;
        var badge = document.getElementById('bellBadge');
        var btn = document.getElementById('bellBtn');
        if (!badge || !btn) return;
        badge.hidden = n <= 0;
        badge.textContent = n > 99 ? '99+' : String(n);
        btn.setAttribute('aria-label', n > 0 ? 'Notificações (' + n + ' não lidas)' : 'Notificações');
    },

    abrir: function() {
        if (Notificacoes._painel) return;
        var lista = h('ul', { class: 'notif-list', id: 'notifList', 'aria-live': 'polite' });
        var sheet = h('div', { class: 'notif-sheet', role: 'dialog', 'aria-modal': 'true', 'aria-label': 'Notificações' }, [
            h('div', { class: 'notif-head' }, [
                h('h2', { text: 'Notificações' }),
                h('button', { type: 'button', class: 'notif-link', id: 'notifTodas', text: 'Marcar todas como lidas',
                    onClick: function() { Notificacoes.marcarTodas(); } }),
                h('button', { type: 'button', class: 'notif-close', 'aria-label': 'Fechar notificações',
                    onClick: function() { Notificacoes.fechar(); }, text: '×' })
            ]),
            lista
        ]);
        var overlay = h('div', { class: 'notif-overlay', onClick: function(e) { if (e.target === overlay) Notificacoes.fechar(); } }, sheet);
        document.body.appendChild(overlay);
        Notificacoes._painel = overlay;
        Notificacoes._onKey = function(e) { if (e.key === 'Escape') Notificacoes.fechar(); };
        document.addEventListener('keydown', Notificacoes._onKey);
        Notificacoes.carregarLista();
    },

    fechar: function() {
        if (!Notificacoes._painel) return;
        Notificacoes._painel.remove();
        Notificacoes._painel = null;
        document.removeEventListener('keydown', Notificacoes._onKey);
        var btn = document.getElementById('bellBtn');
        if (btn) btn.focus();
    },

    carregarLista: async function() {
        var ul = document.getElementById('notifList');
        if (!ul) return;
        if (!ul.firstChild) ul.appendChild(h('li', { class: 'notif-empty', text: 'Carregando...' }));
        try {
            var resp = await API.notificacoes(1, 30);
            var dados = resp.data || {};
            var itens = dados.itens || [];
            Notificacoes._definirContador(dados.nao_lidas || 0);
            limpar(ul);
            if (itens.length === 0) {
                ul.appendChild(h('li', { class: 'notif-empty', text: 'Nenhuma notificação por enquanto.' }));
                return;
            }
            itens.forEach(function(n) { ul.appendChild(Notificacoes._item(n)); });
        } catch(e) {
            limpar(ul).appendChild(h('li', { class: 'notif-empty', text: 'Erro ao carregar notificações.' }));
        }
    },

    // Só aceita links internos (começam com "/" e não com "//")
    _linkSeguro: function(link) {
        return typeof link === 'string' && link.charAt(0) === '/' && link.charAt(1) !== '/' && link.indexOf('\\') < 0;
    },

    _item: function(n) {
        var li = h('li', { class: 'notif-item' + (n.lida ? '' : ' unread') }, h('button', {
            type: 'button',
            class: 'notif-body',
            onClick: async function() {
                if (!n.lida) { try { await API.marcarNotificacaoLida(n.id); } catch(e) {} }
                if (Notificacoes._linkSeguro(n.link)) window.location.href = n.link;
                else { Notificacoes.atualizarContador(); Notificacoes.carregarLista(); }
            }
        }, [
            h('strong', { text: n.titulo }),
            h('span', { text: n.mensagem }),
            h('small', { text: fmtDataHora(n.criado_em) })
        ]));
        return li;
    },

    marcarTodas: async function() {
        try {
            await API.marcarTodasNotificacoesLidas();
            Notificacoes._definirContador(0);
            Notificacoes.carregarLista();
        } catch(e) { showToast(e.message, 'error'); }
    }
};

// ─── Loading state em botão ───────────────────────────────────────────────────
function btnLoading(btn, isLoading) {
    if (isLoading) {
        btn.disabled = true;
        btn._origNodes = Array.prototype.slice.call(btn.childNodes);
        btn.classList.add('loading');
        limpar(btn);
    } else {
        btn.disabled = false;
        btn.classList.remove('loading');
        if (btn._origNodes) {
            limpar(btn);
            btn._origNodes.forEach(function(n) { btn.appendChild(n); });
            btn._origNodes = null;
        }
    }
}

// ─── Consentimento LGPD ──────────────────────────────────────────────────────
function checkLGPDConsent() {
    if (localStorage.getItem('gym_lgpd_consent') === 'accepted') return;

    var banner = h('div', { id: 'lgpdBanner' }, [
        h('p', { style: { margin: '0', flex: '1', minWidth: '220px' } }, [
            h('strong', { text: 'Seus dados, seu controle.' }),
            ' O GymScore coleta nome, e-mail, CPF e data de nascimento para operação da plataforma, conforme a ',
            h('a', { href: '/privacidade', style: { color: '#4CAF50' }, text: 'Política de Privacidade' }),
            ' (LGPD). Dados sensíveis ficam ',
            h('strong', { text: 'ocultos por padrão' }),
            '.'
        ]),
        h('div', { style: { display: 'flex', gap: '10px', flexShrink: '0' } }, [
            h('button', { type: 'button', onClick: acceptLGPD, text: 'Entendi e aceito',
                style: { background: '#4CAF50', color: '#fff', border: 'none', padding: '8px 20px', borderRadius: '6px', cursor: 'pointer', fontWeight: 'bold' } }),
            h('a', { href: '/privacidade', text: 'Ver política',
                style: { background: 'transparent', color: '#4CAF50', border: '1px solid #4CAF50', padding: '8px 14px', borderRadius: '6px', textDecoration: 'none', fontSize: '0.85em' } })
        ])
    ]);
    banner.style.cssText = [
        'position:fixed', 'bottom:0', 'left:0', 'right:0', 'z-index:9999',
        'background:#1a1a1a', 'color:#f4f4f4', 'padding:18px 24px',
        'border-top:2px solid #4CAF50', 'font-size:0.9em',
        'display:flex', 'gap:16px', 'align-items:center', 'flex-wrap:wrap'
    ].join(';');
    document.body.appendChild(banner);
}

function acceptLGPD() {
    localStorage.setItem('gym_lgpd_consent', 'accepted');
    var banner = document.getElementById('lgpdBanner');
    if (banner) banner.remove();
}

// ─── HTTP helper ─────────────────────────────────────────────────────────────
// Respostas do backend: {success, message, data} ou {success:false, error}
async function _req(path, options) {
    options = options || {};
    var token = Auth.getToken();
    var headers = Object.assign({}, options.headers || {});
    // FormData define o próprio Content-Type (com boundary)
    if (!(options.body instanceof FormData) && !headers['Content-Type']) headers['Content-Type'] = 'application/json';
    if (token) headers['Authorization'] = 'Bearer ' + token;

    var res = await fetch(path, Object.assign({}, options, { headers: headers }));
    var json = null;
    try { json = await res.json(); } catch(e) { json = null; }

    if (!res.ok) {
        // Sessão expirada em rota protegida: volta para o login
        // (não dispara em rotas públicas de auth nem em páginas públicas, evitando loop de redirecionamento)
        if (res.status === 401 && token && !_rotaPublica(path) && !_paginaPublica()) {
            Auth.clear();
            window.location.href = '/login';
        }
        var msg = (json && (json.error || json.message)) ||
            (res.status === 429 ? 'Muitas tentativas. Aguarde um instante.' :
             res.status === 413 ? 'Arquivo grande demais.' : 'Erro na requisição');
        var err = new Error(msg);
        err.status = res.status;
        throw err;
    }
    return json || {};
}

function _rotaPublica(path) {
    return path === '/api/login' || path === '/api/logout' || path.indexOf('/api/auth/') === 0;
}
function _paginaPublica() {
    return ['/login', '/signup', '/esqueci-senha', '/redefinir-senha', '/privacidade', '/'].indexOf(window.location.pathname) >= 0;
}

function _post(path, body) {
    return _req(path, { method: 'POST', body: body === undefined ? undefined : JSON.stringify(body) });
}

// ─── API ─────────────────────────────────────────────────────────────────────
// Identidade sempre vem do token (JWT): nenhum método envia id do usuário logado.
var API = {
    // Auth
    login: function(email, senha) { return _post('/api/login', { email: email, senha: senha }); },
    cadastrar: function(dados) { return _post('/api/usuarios', dados); },
    cadastrarAcademia: function(dados) { return _post('/api/academias', dados); },
    // O backend revoga o token usado e devolve um novo em data.token: salva antes de qualquer outra chamada
    alterarSenha: async function(senhaAtual, novaSenha) {
        var resp = await _req('/api/usuarios/senha', {
            method: 'PATCH',
            body: JSON.stringify({ senha_atual: senhaAtual, nova_senha: novaSenha })
        });
        if (resp.data && resp.data.token) Auth.saveToken(resp.data.token);
        return resp;
    },
    esqueciSenha: function(email) { return _post('/api/auth/esqueci-senha', { email: email }); },
    redefinirSenha: function(token, novaSenha) {
        return _post('/api/auth/redefinir-senha', { token: token, nova_senha: novaSenha });
    },

    // Usuários
    me: function() { return _req('/api/usuarios/me'); },
    // Busca por nome, excluindo o próprio usuário logado (tela de adicionar amigo)
    buscarUsuarios: function(q) { return _req('/api/usuarios/busca?q=' + encodeURIComponent(q)); },
    atualizarPerfil: function(dados) {
        return _req('/api/usuarios/perfil', { method: 'PATCH', body: JSON.stringify(dados) });
    },

    // Desafios
    listarDesafios: function(status, pagina, limite) {
        return _req('/api/desafios' + _qs({ status: status, pagina: pagina, limite: limite }));
    },
    listarMeusDesafios: function(status, pagina, limite) {
        return _req('/api/desafios/meus' + _qs({ status: status, pagina: pagina, limite: limite }));
    },
    buscarDesafio: function(id) { return _req('/api/desafios/' + encodeURIComponent(id)); },
    criarDesafio: function(dados) { return _post('/api/desafios', dados); },
    entrarDesafio: function(id) { return _post('/api/desafios/' + encodeURIComponent(id) + '/entrar'); },
    sairDesafio: function(id) { return _post('/api/desafios/' + encodeURIComponent(id) + '/sair'); },
    cancelarDesafio: function(id) { return _post('/api/desafios/' + encodeURIComponent(id) + '/cancelar'); },
    iniciarDesafio: function(id) { return _post('/api/desafios/' + encodeURIComponent(id) + '/iniciar'); },
    abrirVotacao: function(id) { return _post('/api/desafios/' + encodeURIComponent(id) + '/abrir-votacao'); },
    votarDesafio: function(id, idVotado) {
        return _post('/api/desafios/' + encodeURIComponent(id) + '/votar', { id_votado: idVotado });
    },
    apurarDesafio: function(id) { return _post('/api/desafios/' + encodeURIComponent(id) + '/apurar'); },
    convidarAmigo: function(id, idAmigo) {
        return _post('/api/desafios/' + encodeURIComponent(id) + '/convidar', { id_amigo: idAmigo });
    },
    listarCheckins: function(id) { return _req('/api/desafios/' + encodeURIComponent(id) + '/checkins'); },
    // formData: texto?, latitude?, longitude?, foto?
    fazerCheckin: function(id, formData) {
        return _req('/api/desafios/' + encodeURIComponent(id) + '/checkins', { method: 'POST', body: formData });
    },

    // PIX / Depósito (valor em centavos)
    gerarPix: function(valorCentavos, cpf) { return _post('/api/pagamento/pix', { valor_centavos: valorCentavos, cpf: cpf }); },
    consultarPagamento: function(asaasId) { return _req('/api/pagamento/pix/' + encodeURIComponent(asaasId)); },
    simularPagamento: function(asaasId) { return _post('/api/pagamento/pix/' + encodeURIComponent(asaasId) + '/simular'); },

    // Carteira
    carteira: function() { return _req('/api/carteira'); },
    extrato: function(pagina, limite) { return _req('/api/carteira/extrato' + _qs({ pagina: pagina, limite: limite })); },
    sacar: function(valorCentavos, chavePix, tipoChave) {
        return _post('/api/carteira/saque', { valor_centavos: valorCentavos, chave_pix: chavePix, tipo_chave: tipoChave });
    },
    consultarSaque: function(id) { return _req('/api/carteira/saque/' + encodeURIComponent(id)); },

    // Notificações
    notificacoes: function(pagina, limite) { return _req('/api/notificacoes' + _qs({ pagina: pagina, limite: limite })); },
    notificacoesNaoLidas: function() { return _req('/api/notificacoes/nao-lidas'); },
    marcarNotificacaoLida: function(id) { return _post('/api/notificacoes/' + encodeURIComponent(id) + '/lida'); },
    marcarTodasNotificacoesLidas: function() { return _post('/api/notificacoes/lidas'); },

    // Ranking
    rankingAmigos: function(dias) { return _req('/api/ranking/amigos' + _qs({ dias: dias })); },

    // Amigos (sempre os do usuário logado; id_amigo = o outro lado)
    listarAmigos: function() { return _req('/api/amigos'); },
    adicionarAmigo: function(idAmigo) { return _post('/api/amigos/adicionar', { id_amigo: idAmigo }); },
    aceitarAmizade: function(idAmigo) { return _post('/api/amigos/aceitar', { id_amigo: idAmigo }); },
    recusarAmizade: function(idAmigo) { return _post('/api/amigos/recusar', { id_amigo: idAmigo }); },
    removerAmigo: function(idAmigo) { return _post('/api/amigos/remover', { id_amigo: idAmigo }); },

    // Treinos
    listarTreinos: function() { return _req('/api/treinos'); },
    criarTreino: function(dados) { return _post('/api/treinos', dados); },
    buscarTreino: function(id) { return _req('/api/treinos/' + encodeURIComponent(id)); },
    deletarTreino: function(id) { return _req('/api/treinos/' + encodeURIComponent(id), { method: 'DELETE' }); },
    importarTreino: function(codigo) { return _post('/api/treinos/importar', { codigo: codigo }); },
    concluirExercicio: function(idTreino, grupoMuscular) {
        return _post('/api/treinos/concluir', { id_treino: idTreino, grupo_muscular: grupoMuscular });
    },
    radarTreinos: function() { return _req('/api/treinos/radar'); },

    // Usuários — novos endpoints
    // Busca geral por nome/sobrenome/username, não exclui ninguém (sem caller hoje; disponível para telas futuras)
    buscarPorTermo: function(q) { return _req('/api/usuarios/buscar?q=' + encodeURIComponent(q)); },
    perfilPublico: function(id) { return _req('/api/usuarios/' + id + '/publico'); },
    historicoDesafios: function(id) { return _req('/api/usuarios/' + id + '/historico'); },
    heartbeat: function() { return _req('/api/usuarios/ultima-vez', { method: 'PATCH' }); },

    // Instrutores
    cadastrarInstrutor: function(dados) {
        return _req('/api/instrutores', { method: 'POST', body: JSON.stringify(dados) });
    },

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

// Monta querystring ignorando valores vazios
function _qs(params) {
    var parts = [];
    Object.keys(params).forEach(function(k) {
        var v = params[k];
        if (v === undefined || v === null || v === '') return;
        parts.push(encodeURIComponent(k) + '=' + encodeURIComponent(v));
    });
    return parts.length ? '?' + parts.join('&') : '';
}
