// GymScore Admin — casca do painel: sessão (/auth/me), navegação por abas (hash) e logout.
'use strict';

(function() {
    var h = Adm.h;
    var TELAS = [
        { id: 'painel', rotulo: 'Painel' },
        { id: 'transacoes', rotulo: 'Transações' },
        { id: 'saques', rotulo: 'Saques' },
        { id: 'usuarios', rotulo: 'Usuários' },
        { id: 'desafios', rotulo: 'Desafios' },
        { id: 'denuncias', rotulo: 'Denúncias' },
        { id: 'punicoes', rotulo: 'Punições' },
        { id: 'recursos', rotulo: 'Recursos' },
        { id: 'pagamentos', rotulo: 'Pagamentos pendentes' },
        { id: 'debitos', rotulo: 'Débitos' },
        { id: 'cadastros', rotulo: 'Academias / Instrutores' },
        { id: 'receita', rotulo: 'Receita' },
        { id: 'administradores', rotulo: 'Administradores', superadmin: true },
        { id: 'auditoria', rotulo: 'Auditoria' }
    ];
    var ctx = { admin: null, superadmin: false };

    function rotaAtual() {
        var bruto = window.location.hash.replace(/^#/, '');
        var i = bruto.indexOf('?');
        var id = i >= 0 ? bruto.slice(0, i) : bruto;
        var params = {};
        new URLSearchParams(i >= 0 ? bruto.slice(i + 1) : '').forEach(function(v, k) { params[k] = v; });
        var tela = TELAS.filter(function(t) { return t.id === id && (!t.superadmin || ctx.superadmin); })[0] || TELAS[0];
        return { tela: tela, params: tela.id === id ? params : {} };
    }

    function renderizar() {
        var r = rotaAtual();
        document.querySelectorAll('.adm-nav a').forEach(function(a) {
            var atual = a.dataset.tela === r.tela.id;
            a.classList.toggle('ativo', atual);
            if (atual) a.setAttribute('aria-current', 'page'); else a.removeAttribute('aria-current');
        });
        var raiz = Adm.limpar(document.getElementById('admConteudo'));
        document.title = r.tela.rotulo + ' — GymScore Admin';
        Adm.telas[r.tela.id](raiz, r.params, ctx);
    }
    Adm.renderizar = renderizar;

    function montarNav() {
        var nav = Adm.limpar(document.getElementById('admNav'));
        TELAS.forEach(function(t) {
            if (t.superadmin && !ctx.superadmin) return;
            nav.appendChild(h('li', null, h('a', { href: '#' + t.id, data: { tela: t.id }, text: t.rotulo })));
        });
    }

    async function sair() {
        try { await Adm.api('POST', '/auth/logout', {}, { semRedirecionar: true }); } catch (e) {}
        window.location.replace('/admin/login');
    }

    async function iniciar() {
        try {
            ctx.admin = await Adm.api('GET', '/auth/me', undefined, { semRedirecionar: true });
        } catch (err) {
            if (err.status === 401) { window.location.replace('/admin/login'); return; }
            var c = Adm.limpar(document.getElementById('admConteudo'));
            c.appendChild(Adm.estadoErro(err, iniciar));
            return;
        }
        ctx.superadmin = ctx.admin.papel === 'superadmin';
        document.getElementById('admNome').textContent = ctx.admin.nome;
        var papel = document.getElementById('admPapel');
        papel.textContent = ctx.superadmin ? 'Superadmin' : 'Admin';
        papel.className = 'adm-badge adm-b-' + (ctx.superadmin ? 'info' : 'neutro');
        document.body.classList.toggle('adm-super', ctx.superadmin);
        montarNav();
        window.addEventListener('hashchange', renderizar);
        renderizar();
    }

    document.addEventListener('DOMContentLoaded', function() {
        document.getElementById('admSair').addEventListener('click', sair);
        document.getElementById('admSenha').addEventListener('click', function() { Adm.trocarMinhaSenha(); });
        var menu = document.getElementById('admMenuBtn');
        menu.addEventListener('click', function() {
            var aberto = document.body.classList.toggle('adm-nav-aberta');
            menu.setAttribute('aria-expanded', aberto ? 'true' : 'false');
        });
        document.getElementById('admNav').addEventListener('click', function(e) {
            if (e.target.closest('a')) { document.body.classList.remove('adm-nav-aberta'); menu.setAttribute('aria-expanded', 'false'); }
        });
        iniciar();
    });
})();
