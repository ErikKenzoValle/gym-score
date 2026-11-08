// GymScore Admin — núcleo do painel: DOM seguro, formatação, chamadas à API admin, modais,
// tabelas, filtros e paginação. Sem innerHTML: todo dado da API entra por textContent (h()).
// A CSP de /admin proíbe script/estilo inline: nada de onclick, <script> inline nem atributo style.
'use strict';

var Adm = window.Adm = { telas: {} };

// ─── DOM ─────────────────────────────────────────────────────────────────────
// h(tag, attrs, filhos): attrs aceita class, text, data (data-*), on<Evento> (função) e atributos comuns.
Adm.h = function(tag, attrs, filhos) {
    var el = document.createElement(tag);
    if (attrs) {
        Object.keys(attrs).forEach(function(k) {
            var v = attrs[k];
            if (v === null || v === undefined || v === false) return;
            if (k === 'class') el.className = v;
            else if (k === 'text') el.textContent = v;
            else if (k === 'data') Object.keys(v).forEach(function(d) { el.dataset[d] = v[d]; });
            else if (k.indexOf('on') === 0 && typeof v === 'function') el.addEventListener(k.slice(2).toLowerCase(), v);
            else if (k === 'value') el.value = v;
            else el.setAttribute(k, v === true ? '' : v);
        });
    }
    (function add(c) {
        if (c === null || c === undefined || c === false) return;
        if (Array.isArray(c)) { c.forEach(add); return; }
        el.appendChild(c.nodeType ? c : document.createTextNode(String(c)));
    })(filhos);
    return el;
};
var h = Adm.h;

Adm.limpar = function(el) {
    while (el.firstChild) el.removeChild(el.firstChild);
    return el;
};

// ─── Formatação ──────────────────────────────────────────────────────────────
var _BRL = new Intl.NumberFormat('pt-BR', { style: 'currency', currency: 'BRL' });
var _NUM = new Intl.NumberFormat('pt-BR');
var _DATA = new Intl.DateTimeFormat('pt-BR', { day: '2-digit', month: '2-digit', year: 'numeric' });
var _DATAHORA = new Intl.DateTimeFormat('pt-BR', { day: '2-digit', month: '2-digit', year: 'numeric', hour: '2-digit', minute: '2-digit' });

// Centavos (inteiro) -> "R$ 1.234,56". A divisão só acontece na exibição.
Adm.reais = function(centavos) {
    if (centavos === null || centavos === undefined || centavos === '') return '—';
    return _BRL.format(Number(centavos) / 100);
};
// Valor assinado com classe de cor (crédito/débito)
Adm.valor = function(centavos) {
    var n = Number(centavos) || 0;
    return h('span', { class: 'adm-valor ' + (n > 0 ? 'pos' : n < 0 ? 'neg' : ''), text: Adm.reais(n) });
};
Adm.num = function(n) { return _NUM.format(Number(n) || 0); };
Adm.data = function(raw) {
    if (!raw) return '—';
    var d = new Date(raw);
    return isNaN(d.getTime()) ? '—' : _DATA.format(d);
};
Adm.dataHora = function(raw) {
    if (!raw) return '—';
    var d = new Date(raw);
    return isNaN(d.getTime()) ? '—' : _DATAHORA.format(d);
};
Adm.nome = function(u) {
    if (!u) return '—';
    return ((u.nome || '') + ' ' + (u.sobrenome || '')).trim() || '—';
};
Adm.texto = function(v) { return (v === null || v === undefined || v === '') ? '—' : String(v); };

// ─── Rótulos ─────────────────────────────────────────────────────────────────
Adm.TIPOS_TRANSACAO = [
    ['deposito', 'Depósito'], ['saque', 'Saque'], ['tarifa_saque', 'Tarifa de saque'],
    ['entrada_desafio', 'Inscrição em desafio'], ['estorno_desafio', 'Estorno de inscrição'],
    ['premio_desafio', 'Prêmio de desafio'], ['reserva_desafio', 'Reserva de desafio'],
    ['estorno_reserva', 'Estorno de reserva'], ['pagamento_instrutor', 'Pagamento de instrutor'],
    ['receita_academia', 'Receita da academia'], ['ajuste', 'Ajuste'],
    ['estorno_anulacao', 'Estorno por anulação'], ['abatimento_debito', 'Abatimento de débito']
];
Adm.STATUS_TRANSACAO = [['pending', 'Pendente'], ['received', 'Recebido'], ['concluido', 'Concluído'], ['refunded', 'Estornado'], ['failed', 'Falhou']];
Adm.STATUS_DESAFIO = [['aberto', 'Aberto'], ['em_andamento', 'Em andamento'], ['encerrado', 'Encerrado'], ['cancelado', 'Cancelado']];
Adm.STATUS_CADASTRO = [['pendente', 'Pendente'], ['aprovada', 'Aprovado'], ['rejeitada', 'Rejeitado']];
Adm.PERFIS = [['atleta', 'Atleta'], ['academia', 'Academia'], ['instrutor', 'Instrutor']];
Adm.ORIGENS_RECEITA = [['taxa_inscricao', 'Taxa de inscrição'], ['tarifa_saque', 'Tarifa de saque'], ['estorno_taxa', 'Estorno de taxa (anulação)']];
// Moderação
Adm.CATEGORIAS_DENUNCIA = [['suborno_instrutor', 'Suborno de instrutor'], ['manipulacao_podio', 'Manipulação de pódio'], ['fraude_presenca', 'Fraude de presença'],
    ['conduta_abusiva', 'Conduta abusiva'], ['perfil_falso', 'Perfil falso'], ['outro', 'Outro']];
Adm.STATUS_DENUNCIA = [['aberta', 'Aberta'], ['em_analise', 'Em análise'], ['procedente', 'Procedente'], ['improcedente', 'Improcedente'], ['arquivada', 'Arquivada']];
Adm.JURISDICAO = [['admin', 'Admin'], ['academia', 'Academia']];
Adm.TIPOS_PUNICAO = [['advertencia', 'Advertência'], ['suspensao', 'Suspensão'], ['banimento', 'Banimento'], ['bloqueio_academia', 'Bloqueio na academia'], ['anulacao_resultado', 'Anulação de resultado']];
Adm.STATUS_RECURSO = [['pendente', 'Pendente'], ['deferido', 'Deferido'], ['indeferido', 'Indeferido']];
Adm.STATUS_PENDENTE = [['a_liberar', 'A liberar'], ['retido', 'Retido'], ['liberado', 'Liberado'], ['cancelado', 'Cancelado']];
Adm.TIPOS_PENDENTE = [['premio_desafio', 'Prêmio'], ['pagamento_instrutor', 'Instrutor'], ['receita_academia', 'Receita da academia'], ['taxa_inscricao', 'Taxa da plataforma']];
Adm.MODOS_ANULACAO = [['relancar_podio', 'Relançar pódio'], ['devolver_inscricoes', 'Devolver inscrições']];

Adm.rotulo = function(lista, valor) {
    for (var i = 0; i < lista.length; i++) if (lista[i][0] === valor) return lista[i][1];
    return Adm.texto(valor);
};

// Cor do badge por status
var _CLASSE_STATUS = {
    pending: 'aviso', pendente: 'aviso', aberto: 'info', em_andamento: 'aviso',
    received: 'ok', concluido: 'ok', aprovada: 'ok', encerrado: 'neutro', ativo: 'ok',
    refunded: 'neutro', failed: 'erro', cancelado: 'erro', rejeitada: 'erro', inativo: 'erro', bloqueado: 'erro',
    superadmin: 'info', admin: 'neutro',
    aberta: 'aviso', em_analise: 'info', procedente: 'erro', improcedente: 'ok', arquivada: 'neutro',
    deferido: 'ok', indeferido: 'erro', a_liberar: 'aviso', retido: 'erro', liberado: 'ok',
    suspenso: 'aviso', banido: 'erro', vigente: 'erro', revogada: 'neutro', encerrada: 'neutro', quitado: 'ok', em_aberto: 'erro'
};
Adm.badge = function(texto, chave) {
    return h('span', { class: 'adm-badge adm-b-' + (_CLASSE_STATUS[chave] || 'neutro'), text: texto });
};
Adm.badgeStatus = function(lista, valor) { return Adm.badge(Adm.rotulo(lista, valor), valor); };

// ─── HTTP ────────────────────────────────────────────────────────────────────
// Sessão pelo cookie HttpOnly admin_jwt (o navegador manda sozinho). Nada fica em localStorage.
// Respostas: {success, message, data} | {success:false, error}. 401 -> volta para o login.
Adm.api = async function(metodo, caminho, corpo, opts) {
    opts = opts || {};
    var init = { method: metodo, credentials: 'same-origin', headers: { 'Accept': 'application/json' } };
    if (corpo !== undefined) {
        init.headers['Content-Type'] = 'application/json';
        init.body = JSON.stringify(corpo);
    }
    var res;
    try {
        res = await fetch('/api/admin' + caminho, init);
    } catch (e) {
        var errRede = new Error('Sem conexão com o servidor. Tente de novo.');
        errRede.status = 0;
        throw errRede;
    }
    var json = null;
    try { json = await res.json(); } catch (e) { json = null; }
    if (res.status === 401 && !opts.semRedirecionar) {
        window.location.replace('/admin/login?sessao=expirada');
        var err401 = new Error('Sessão encerrada.');
        err401.status = 401;
        throw err401;
    }
    if (!res.ok || (json && json.success === false)) {
        var msg = (json && (json.error || json.message)) ||
            (res.status === 429 ? 'Muitas tentativas. Aguarde um minuto.' :
             res.status === 403 ? 'Acesso negado.' :
             res.status >= 500 ? 'Erro no servidor (' + res.status + ').' : 'Erro na requisição (' + res.status + ').');
        var err = new Error(msg);
        err.status = res.status;
        if (json && json.codigo) { err.codigo = json.codigo; err.data = json.data || null; }
        throw err;
    }
    return json ? json.data : null;
};
Adm.get = function(caminho, params) { return Adm.api('GET', caminho + Adm.qs(params)); };
Adm.post = function(caminho, corpo) { return Adm.api('POST', caminho, corpo === undefined ? {} : corpo); };

// Querystring ignorando vazios
Adm.qs = function(params) {
    if (!params) return '';
    var partes = [];
    Object.keys(params).forEach(function(k) {
        var v = params[k];
        if (v === undefined || v === null || v === '') return;
        partes.push(encodeURIComponent(k) + '=' + encodeURIComponent(v));
    });
    return partes.length ? '?' + partes.join('&') : '';
};
Adm.id = function(v) { return encodeURIComponent(String(v)); };

// ─── Toast ───────────────────────────────────────────────────────────────────
Adm.toast = function(msg, tipo) {
    var t = document.getElementById('gsToast');
    if (!t) {
        t = h('div', { id: 'gsToast', role: 'status', 'aria-live': 'polite' });
        document.body.appendChild(t);
    }
    t.textContent = msg;
    t.className = (tipo || '') + ' show';
    clearTimeout(t._timer);
    t._timer = setTimeout(function() { t.classList.remove('show'); }, 3200);
};

// ─── Modal ───────────────────────────────────────────────────────────────────
// Pilha de modais (detalhe -> ação). Esc fecha o do topo, Tab fica preso nele, o resto da página
// vira inert e o foco volta para quem abriu.
var _pilha = [];
var _seq = 0;
var _FOCAVEIS = 'a[href],button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])';

function _onTecla(e) {
    var m = _pilha[_pilha.length - 1];
    if (!m) return;
    if (e.key === 'Escape') {
        e.stopPropagation();
        if (!m.ocupado) m.fechar();
        return;
    }
    if (e.key !== 'Tab') return;
    var itens = Array.prototype.filter.call(m.caixa.querySelectorAll(_FOCAVEIS), function(el) { return el.offsetParent !== null; });
    if (!itens.length) { e.preventDefault(); m.caixa.focus(); return; }
    var primeiro = itens[0], ultimo = itens[itens.length - 1];
    if (e.shiftKey && (document.activeElement === primeiro || document.activeElement === m.caixa)) { e.preventDefault(); ultimo.focus(); }
    else if (!e.shiftKey && document.activeElement === ultimo) { e.preventDefault(); primeiro.focus(); }
}

// o: {titulo, corpo (Node|array), rodape (Node|array), largo, aoFechar}
// -> {caixa, corpo, rodape, fechar(), definirCorpo(n), ocupado}
Adm.modal = function(o) {
    var idTitulo = 'admModal' + (++_seq);
    var corpo = h('div', { class: 'adm-modal-corpo' }, o.corpo);
    var rodape = h('div', { class: 'adm-modal-rodape' }, o.rodape);
    var m = { ocupado: false, corpo: corpo, rodape: rodape, opener: document.activeElement, inertes: [] };
    m.caixa = h('div', { class: 'adm-modal' + (o.largo ? ' adm-modal-largo' : ''), role: 'dialog', 'aria-modal': 'true', 'aria-labelledby': idTitulo, tabindex: '-1' }, [
        h('div', { class: 'adm-modal-topo' }, [
            h('h2', { id: idTitulo, text: o.titulo }),
            h('button', { type: 'button', class: 'adm-modal-x', 'aria-label': 'Fechar', text: '×', onClick: function() { if (!m.ocupado) m.fechar(); } })
        ]),
        corpo,
        rodape
    ]);
    m.overlay = h('div', { class: 'adm-overlay' }, m.caixa);
    m.overlay.addEventListener('mousedown', function(e) { if (e.target === m.overlay && !m.ocupado) m.fechar(); });
    m.fechar = function() {
        var i = _pilha.indexOf(m);
        if (i < 0) return;
        _pilha.splice(i, 1);
        m.inertes.forEach(function(n) { n.inert = false; });
        m.overlay.remove();
        if (!_pilha.length) {
            document.removeEventListener('keydown', _onTecla, true);
            document.body.classList.remove('adm-sem-rolagem');
        }
        if (m.opener && document.body.contains(m.opener) && m.opener.focus) { try { m.opener.focus(); } catch (e) {} }
        if (o.aoFechar) o.aoFechar();
    };
    m.definirCorpo = function(n) { Adm.limpar(corpo); if (n) corpo.appendChild(Array.isArray(n) ? h('div', null, n) : n); };
    m.definirRodape = function(n) { Adm.limpar(rodape); if (n) rodape.appendChild(Array.isArray(n) ? h('div', { class: 'adm-acoes' }, n) : n); };

    Array.prototype.forEach.call(document.body.children, function(n) {
        if (n.id === 'gsToast' || n.tagName === 'SCRIPT' || n.inert) return;
        n.inert = true;
        m.inertes.push(n);
    });
    document.body.appendChild(m.overlay);
    document.body.classList.add('adm-sem-rolagem');
    _pilha.push(m);
    if (_pilha.length === 1) document.addEventListener('keydown', _onTecla, true);
    setTimeout(function() {
        var alvo = m.caixa.querySelector('[autofocus]') || corpo.querySelector(_FOCAVEIS) || m.caixa;
        try { alvo.focus(); } catch (e) {}
    }, 0);
    return m;
};

// Lista de definições [[rótulo, valor(Node|string)], ...]
Adm.dl = function(linhas) {
    return h('dl', { class: 'adm-dl' }, linhas.filter(Boolean).map(function(l) {
        return h('div', null, [h('dt', { text: l[0] }), h('dd', null, (l[1] && l[1].nodeType) ? l[1] : Adm.texto(l[1]))]);
    }));
};

// Ação com motivo obrigatório (3..500). o: {titulo, mensagem, linhas, campos:[{nome, rotulo, tipo, opcoes, valor,
// obrigatorio, autocomplete}], textoConfirmar, perigo, executar(dados) -> Promise, sucesso (texto|fn(res)), aoConcluir(res)}
Adm.acao = function(o) {
    return new Promise(function(resolve) {
        var concluido = false;
        var erro = h('p', { class: 'adm-erro', role: 'alert', hidden: true });
        var contador = h('span', { class: 'adm-contador', 'aria-live': 'polite', text: '0/500' });
        var idMotivo = 'admMotivo' + (++_seq);
        var motivo = h('textarea', { id: idMotivo, class: 'gs-input adm-motivo', rows: '3', maxlength: '500', required: true, autofocus: true,
            placeholder: 'Descreva o motivo (fica registrado na auditoria)' });
        motivo.addEventListener('input', function() { contador.textContent = motivo.value.length + '/500'; });
        var inputs = {}, wrapCampos = {};
        var campos = (o.campos || []).map(function(c) {
            var id = 'admCampo' + (++_seq);
            var el;
            if (c.tipo === 'select') {
                el = h('select', { id: id, class: 'gs-input' }, c.opcoes.map(function(op) {
                    return h('option', { value: op[0], text: op[1] });
                }));
                if (c.valor !== undefined) el.value = c.valor;
            } else if (c.tipo === 'textarea') {
                el = h('textarea', { id: id, class: 'gs-input adm-motivo', rows: String(c.linhas || 4), maxlength: c.max ? String(c.max) : null, placeholder: c.placeholder || null });
                if (c.valor) el.value = c.valor;
            } else {
                el = h('input', { id: id, class: 'gs-input', type: c.tipo || 'text', autocomplete: c.autocomplete || 'off',
                    maxlength: c.max ? String(c.max) : null, value: c.valor || '' });
            }
            inputs[c.nome] = el;
            var dica = c.min || c.ajuda ? h('small', { class: 'adm-sub adm-bloco', text: (c.ajuda || '') + (c.min ? (c.ajuda ? ' ' : '') + 'Mínimo ' + c.min + ' caracteres.' : '') }) : null;
            var wrap = h('div', { class: 'adm-campo' }, [h('label', { for: id }, [c.rotulo, c.obrigatorio === false ? h('span', { class: 'adm-obrig', text: ' (opcional)' }) : null]), el, dica]);
            if (c.aoMudar) el.addEventListener('change', function() { c.aoMudar(el.value, inputs, wrapCampos); });
            wrapCampos[c.nome] = wrap;
            return wrap;
        });
        var form = h('form', { class: 'adm-form-acao', novalidate: true }, [
            o.mensagem ? h('p', { class: 'adm-modal-msg', text: o.mensagem }) : null,
            (o.linhas && o.linhas.length) ? Adm.dl(o.linhas) : null,
            campos,
            o.extra || null,
            h('div', { class: 'adm-campo' }, [
                h('label', { for: idMotivo }, ['Motivo ', h('span', { class: 'adm-obrig', text: '(obrigatório, 3 a 500 caracteres)' })]),
                motivo,
                contador
            ]),
            erro
        ]);
        var btnCancelar = h('button', { type: 'button', class: 'gs-btn gs-btn-secondary', text: 'Cancelar' });
        var btnOk = h('button', { type: 'button', class: 'gs-btn ' + (o.perigo ? 'gs-btn-danger' : 'gs-btn-primary'), text: o.textoConfirmar || 'Confirmar' });
        var m = Adm.modal({ titulo: o.titulo, corpo: form, largo: !!o.largo, rodape: h('div', { class: 'adm-acoes' }, [btnCancelar, btnOk]),
            aoFechar: function() { if (!concluido) resolve(null); } });

        if (o.aoAbrir) o.aoAbrir(inputs, wrapCampos);
        function mostrarErro(msg) { erro.textContent = msg; erro.hidden = !msg; }
        btnCancelar.addEventListener('click', function() { if (!m.ocupado) m.fechar(); });
        async function enviar(e) {
            if (e) e.preventDefault();
            if (m.ocupado) return;
            var dados = { motivo: motivo.value.trim() };
            Object.keys(inputs).forEach(function(k) { dados[k] = inputs[k].value.trim(); });
            var faltando = (o.campos || []).filter(function(c) {
                return c.obrigatorio !== false && !dados[c.nome] && !(wrapCampos[c.nome] && wrapCampos[c.nome].hidden);
            });
            if (faltando.length) { mostrarErro('Preencha: ' + faltando.map(function(c) { return c.rotulo; }).join(', ') + '.'); inputs[faltando[0].nome].focus(); return; }
            var curto = (o.campos || []).filter(function(c) { return c.min && dados[c.nome] && dados[c.nome].length < c.min; })[0];
            if (curto) { mostrarErro(curto.rotulo + ': mínimo de ' + curto.min + ' caracteres.'); inputs[curto.nome].focus(); return; }
            var msgVal = o.validar ? o.validar(dados) : '';
            if (msgVal) { mostrarErro(msgVal); return; }
            if (dados.motivo.length < 3 || dados.motivo.length > 500) {
                mostrarErro('O motivo precisa ter entre 3 e 500 caracteres.');
                motivo.setAttribute('aria-invalid', 'true');
                motivo.focus();
                return;
            }
            motivo.removeAttribute('aria-invalid');
            mostrarErro('');
            m.ocupado = true;
            btnOk.disabled = btnCancelar.disabled = true;
            btnOk.classList.add('loading');
            try {
                var res = await o.executar(dados);
                concluido = true;
                m.ocupado = false;
                m.fechar();
                var txt = typeof o.sucesso === 'function' ? o.sucesso(res) : (o.sucesso || 'Ação concluída.');
                if (txt) Adm.toast(txt, 'success');
                if (o.aoConcluir) o.aoConcluir(res);
                resolve(res);
            } catch (err) {
                m.ocupado = false;
                btnOk.disabled = btnCancelar.disabled = false;
                btnOk.classList.remove('loading');
                if (err.status === 401) return;
                mostrarErro(err.message + (err.status ? ' (HTTP ' + err.status + ')' : ''));
            }
        }
        btnOk.addEventListener('click', enviar);
        form.addEventListener('submit', enviar);
    });
};

// Modal de detalhe que carrega dados: carregar() -> Promise<data>; render(data, m, recarregar) -> Node
Adm.detalhe = function(titulo, carregar, render) {
    var m = Adm.modal({ titulo: titulo, largo: true, corpo: Adm.carregando(),
        rodape: h('div', { class: 'adm-acoes' }, h('button', { type: 'button', class: 'gs-btn gs-btn-secondary', text: 'Fechar', onClick: function() { m.fechar(); } })) });
    async function recarregar() {
        m.definirCorpo(Adm.carregando());
        try {
            var data = await carregar();
            if (_pilha.indexOf(m) < 0) return;
            m.definirCorpo(render(data, m, recarregar));
        } catch (err) {
            if (err.status === 401) return;
            m.definirCorpo(Adm.estadoErro(err, recarregar));
        }
    }
    recarregar();
    return m;
};

// ─── Estados ─────────────────────────────────────────────────────────────────
Adm.carregando = function() {
    return h('div', { class: 'adm-estado', role: 'status' }, [h('span', { class: 'adm-spinner', 'aria-hidden': 'true' }), 'Carregando…']);
};
Adm.estadoVazio = function(texto) {
    return h('div', { class: 'adm-estado adm-vazio', text: texto || 'Nenhum registro encontrado.' });
};
Adm.estadoErro = function(err, tentar) {
    return h('div', { class: 'adm-estado adm-estado-erro', role: 'alert' }, [
        h('p', { text: (err && err.message) || 'Erro ao carregar.' }),
        tentar ? h('button', { type: 'button', class: 'gs-btn gs-btn-secondary', text: 'Tentar de novo', onClick: tentar }) : null
    ]);
};

// ─── Tabela ──────────────────────────────────────────────────────────────────
// colunas: [{titulo, valor(item) -> Node|string, classe}] ; opts: {aoClicar(item), rotulo}
Adm.tabela = function(colunas, itens, opts) {
    opts = opts || {};
    var tabela = h('table', { class: 'adm-tabela' }, [
        opts.rotulo ? h('caption', { class: 'adm-sr', text: opts.rotulo }) : null,
        h('thead', null, h('tr', null, colunas.map(function(c) { return h('th', { scope: 'col', class: c.classe || null, text: c.titulo }); }))),
        h('tbody', null, itens.map(function(it) {
            var tr = h('tr', null, colunas.map(function(c) {
                var v = c.valor(it);
                return h('td', { class: c.classe || null }, (v && v.nodeType) || Array.isArray(v) ? v : Adm.texto(v));
            }));
            if (opts.aoClicar) {
                tr.classList.add('adm-clicavel');
                tr.tabIndex = 0;
                tr.addEventListener('click', function(e) {
                    if (e.target.closest('button,a,summary')) return;
                    opts.aoClicar(it);
                });
                tr.addEventListener('keydown', function(e) {
                    if ((e.key === 'Enter' || e.key === ' ') && e.target === tr) { e.preventDefault(); opts.aoClicar(it); }
                });
            }
            return tr;
        }))
    ]);
    return h('div', { class: 'adm-tabela-wrap', tabindex: '0', role: 'region', 'aria-label': opts.rotulo || 'Tabela' }, tabela);
};

// Botão pequeno de ação em linha de tabela
Adm.botao = function(texto, aoClicar, classe) {
    return h('button', { type: 'button', class: 'gs-btn adm-btn-sm ' + (classe || 'gs-btn-secondary'), text: texto, onClick: aoClicar });
};

// ─── Navegação (estado na hash: #tela?filtro=x&pagina=2) ─────────────────────
// Na hash, '' é mantido (ex.: status= "Todos" sobrepõe o filtro padrão da tela); undefined/null saem.
Adm.navegar = function(tela, params) {
    var partes = [];
    Object.keys(params || {}).forEach(function(k) {
        var v = params[k];
        if (v === undefined || v === null) return;
        partes.push(encodeURIComponent(k) + '=' + encodeURIComponent(v));
    });
    var alvo = '#' + tela + (partes.length ? '?' + partes.join('&') : '');
    if (window.location.hash === alvo) { if (Adm.renderizar) Adm.renderizar(); }
    else window.location.hash = alvo;
};

// ─── Filtros ─────────────────────────────────────────────────────────────────
// defs: [{nome, rotulo, tipo: select|texto|numero|data|checkbox, opcoes, placeholder}]
Adm.filtros = function(tela, defs, params, padroes) {
    var campos = {};
    var form = h('form', { class: 'adm-filtros', role: 'search', 'aria-label': 'Filtros' }, defs.map(function(d) {
        var id = 'admF_' + tela + '_' + d.nome;
        var el;
        if (d.tipo === 'select') {
            el = h('select', { id: id, class: 'gs-input', name: d.nome }, [h('option', { value: '', text: d.todos || 'Todos' })].concat(
                d.opcoes.map(function(op) { return h('option', { value: op[0], text: op[1] }); })));
            el.value = params[d.nome] || '';
        } else if (d.tipo === 'checkbox') {
            el = h('input', { id: id, type: 'checkbox', name: d.nome });
            el.checked = params[d.nome] === 'true';
            campos[d.nome] = el;
            return h('label', { class: 'adm-filtro adm-filtro-check', for: id }, [el, d.rotulo]);
        } else {
            el = h('input', { id: id, class: 'gs-input', name: d.nome,
                type: d.tipo === 'data' ? 'date' : d.tipo === 'numero' ? 'number' : 'search',
                min: d.tipo === 'numero' ? '1' : null, placeholder: d.placeholder || null, value: params[d.nome] || '' });
        }
        campos[d.nome] = el;
        return h('div', { class: 'adm-filtro' + (d.largo ? ' adm-filtro-largo' : '') }, [h('label', { for: id, text: d.rotulo }), el]);
    }).concat([
        h('div', { class: 'adm-filtro-botoes' }, [
            h('button', { type: 'submit', class: 'gs-btn gs-btn-primary adm-btn-sm', text: 'Filtrar' }),
            h('button', { type: 'button', class: 'gs-btn gs-btn-secondary adm-btn-sm', text: 'Limpar', onClick: function() { Adm.navegar(tela, padroes || {}); } })
        ])
    ]));
    form.addEventListener('submit', function(e) {
        e.preventDefault();
        var pad = padroes || {};
        var novo = { limite: params.limite || undefined };
        // Parâmetros fixos da tela (padrões que não são campo de filtro) seguem junto
        Object.keys(pad).forEach(function(k) { if (!campos[k]) novo[k] = pad[k]; });
        Object.keys(campos).forEach(function(k) {
            var el = campos[k];
            var v = el.type === 'checkbox' ? (el.checked ? 'true' : '') : el.value.trim();
            // vazio só fica na hash quando a tela tem padrão para o campo (para "Todos" valer)
            novo[k] = v !== '' ? v : (k in pad ? '' : undefined);
        });
        Adm.navegar(tela, novo);
    });
    return form;
};

// ─── Paginação ───────────────────────────────────────────────────────────────
Adm.paginacao = function(tela, params, pagina) {
    var total = Number(pagina.total) || 0;
    var limite = Number(pagina.limite) || 20;
    var atual = Number(pagina.pagina) || 1;
    var paginas = Math.max(1, Math.ceil(total / limite));
    function ir(p, lim) {
        var novo = Object.assign({}, params, { pagina: p > 1 ? String(p) : undefined, limite: lim ? String(lim) : params.limite });
        Adm.navegar(tela, novo);
    }
    var sel = h('select', { class: 'gs-input adm-limite', 'aria-label': 'Itens por página' }, [20, 50, 100].map(function(n) {
        return h('option', { value: String(n), text: n + ' por página' });
    }));
    sel.value = String(limite);
    sel.addEventListener('change', function() { ir(1, sel.value); });
    return h('nav', { class: 'adm-paginacao', 'aria-label': 'Paginação' }, [
        h('span', { class: 'adm-pag-info', text: Adm.num(total) + (total === 1 ? ' registro' : ' registros') + ' · página ' + atual + ' de ' + paginas }),
        h('div', { class: 'adm-acoes' }, [
            sel,
            h('button', { type: 'button', class: 'gs-btn gs-btn-secondary adm-btn-sm', text: '‹ Anterior', disabled: atual <= 1, onClick: function() { ir(atual - 1); } }),
            h('button', { type: 'button', class: 'gs-btn gs-btn-secondary adm-btn-sm', text: 'Próxima ›', disabled: atual >= paginas, onClick: function() { ir(atual + 1); } })
        ])
    ]);
};

// ─── Tela de lista genérica ──────────────────────────────────────────────────
// cfg: {tela, titulo, descricao, filtros, padroes, caminho, params(p) -> params da API, colunas,
//       aoClicar(item), topo(data) -> Node, rotulo, antes: Node}
Adm.lista = function(raiz, params, cfg) {
    var p = Object.assign({}, cfg.padroes || {}, params);
    var conteudo = h('div', { class: 'adm-lista-conteudo' }, Adm.carregando());
    raiz.appendChild(h('section', { class: 'adm-tela' }, [
        h('header', { class: 'adm-tela-topo' }, [
            h('div', null, [h('h1', { class: 'adm-titulo', text: cfg.titulo }), cfg.descricao ? h('p', { class: 'adm-sub', text: cfg.descricao }) : null]),
            cfg.botoes || null
        ]),
        cfg.antes || null,
        cfg.filtros ? Adm.filtros(cfg.tela, cfg.filtros, p, cfg.padroes) : null,
        conteudo
    ]));
    async function carregar() {
        Adm.limpar(conteudo).appendChild(Adm.carregando());
        try {
            var apiParams = Object.assign({}, cfg.params ? cfg.params(p) : p);
            apiParams.pagina = p.pagina || 1;
            apiParams.limite = p.limite || 20;
            var data = await Adm.get(cfg.caminho, apiParams);
            Adm.limpar(conteudo);
            if (cfg.topo) { var t = cfg.topo(data); if (t) conteudo.appendChild(t); }
            var itens = (data && data.itens) || [];
            if (!itens.length) conteudo.appendChild(Adm.estadoVazio());
            else conteudo.appendChild(Adm.tabela(cfg.colunas, itens, { aoClicar: cfg.aoClicar, rotulo: cfg.rotulo || cfg.titulo }));
            if (data && data.total > 0) conteudo.appendChild(Adm.paginacao(cfg.tela, p, data));
        } catch (err) {
            if (err.status === 401) return;
            Adm.limpar(conteudo).appendChild(Adm.estadoErro(err, carregar));
        }
    }
    carregar();
    return { recarregar: carregar };
};

// Cartão de métrica
Adm.cartao = function(rotulo, valor, nota, classe) {
    return h('div', { class: 'adm-cartao' + (classe ? ' ' + classe : '') }, [
        h('span', { class: 'adm-cartao-rotulo', text: rotulo }),
        h('strong', { class: 'adm-cartao-valor', text: valor }),
        nota ? h('span', { class: 'adm-cartao-nota', text: nota }) : null
    ]);
};

// Link interno para outra tela (ex.: usuário de uma transação)
Adm.link = function(texto, tela, params) {
    return h('a', { href: '#' + tela + Adm.qs(params), class: 'gs-link', text: texto });
};

// ─── Imagens de evidência (rota autenticada) ─────────────────────────────────
// Baixa com o cookie admin_jwt e devolve um object URL (blob:, aceito pela CSP do painel).
// Só aceita JPEG/PNG/WebP; nunca usa caminho público. As URLs são liberadas ao sair da página.
Adm.MIMES_IMAGEM = ['image/jpeg', 'image/png', 'image/webp'];
Adm._urlsImagem = [];
Adm.imagemAutenticada = async function(caminho) {
    var res = await fetch('/api/admin' + caminho, { credentials: 'same-origin', cache: 'no-store' });
    if (res.status === 401) { window.location.replace('/admin/login?sessao=expirada'); throw new Error('Sessão encerrada.'); }
    if (!res.ok) throw new Error(res.status === 404 ? 'Imagem indisponível.' : 'Erro ao carregar (' + res.status + ').');
    var blob = await res.blob();
    if (Adm.MIMES_IMAGEM.indexOf(blob.type) < 0) throw new Error('Arquivo não é imagem válida.');
    var url = URL.createObjectURL(blob);
    Adm._urlsImagem.push(url);
    return url;
};
window.addEventListener('pagehide', function() {
    Adm._urlsImagem.forEach(function(u) { try { URL.revokeObjectURL(u); } catch (e) {} });
    Adm._urlsImagem = [];
});
