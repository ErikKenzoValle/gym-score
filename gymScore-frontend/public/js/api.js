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

// Contagem regressiva legível até uma data ("Faltam 3 dias", "Começa em 2h 10min")
function fmtContagem(raw) {
    var d = new Date(raw);
    if (!raw || isNaN(d.getTime())) return '';
    var min = Math.floor((d.getTime() - Date.now()) / 60000);
    if (min <= 0) return 'Já começou';
    if (min < 60) return 'Começa em ' + min + ' min';
    var horas = Math.floor(min / 60);
    if (horas < 24) return 'Começa em ' + horas + 'h' + (min % 60 ? ' ' + (min % 60) + 'min' : '');
    var dias = Math.floor(horas / 24);
    return 'Faltam ' + dias + (dias === 1 ? ' dia' : ' dias');
}

// Mensagem amigável para erros de rede (fetch lança TypeError sem status)
function mensagemErro(e) {
    if (!e) return 'Erro inesperado. Tente de novo.';
    if (e.status === undefined && (e.name === 'TypeError' || /fetch|network/i.test(e.message || ''))) {
        return 'Sem conexão. Tente de novo.';
    }
    return e.message || 'Erro inesperado. Tente de novo.';
}

// Compartilha uma URL (Web Share API) com fallback de copiar para a área de transferência
async function compartilharLink(url, titulo, texto) {
    var abs = new URL(url, window.location.origin).href;
    try {
        if (navigator.share) {
            await navigator.share({ title: titulo || 'GymScore', text: texto || '', url: abs });
            return;
        }
    } catch(e) {
        if (e && e.name === 'AbortError') return; // usuário cancelou
    }
    try {
        await navigator.clipboard.writeText(abs);
        showToast('Link copiado!', 'success');
    } catch(e) {
        window.prompt('Copie o link:', abs);
    }
}

// ─── Máscaras e validadores de formulário ────────────────────────────────────
// Uso declarativo: <input data-mask="moeda|cpf|cnpj|telefone|cref" ...>. Aplicado sozinho no
// DOMContentLoaded (Mask.auto) e em qualquer input via Mask.aplicar(input, tipo).
//  - moeda: o input mostra "1.234,56"; o valor em centavos fica em Mask.centavos(input).
//    Limite opcional em centavos: data-max-centavos="1000000".
var Mask = {
    digitos: function(s) { return String(s == null ? '' : s).replace(/\D/g, ''); },

    cpf: function(v) {
        var d = Mask.digitos(v).slice(0, 11);
        return d.replace(/^(\d{3})(\d)/, '$1.$2').replace(/^(\d{3})\.(\d{3})(\d)/, '$1.$2.$3').replace(/\.(\d{3})(\d)/, '.$1-$2');
    },
    cnpj: function(v) {
        var d = Mask.digitos(v).slice(0, 14);
        return d.replace(/^(\d{2})(\d)/, '$1.$2').replace(/^(\d{2})\.(\d{3})(\d)/, '$1.$2.$3')
            .replace(/\.(\d{3})(\d)/, '.$1/$2').replace(/(\d{4})(\d)/, '$1-$2');
    },
    telefone: function(v) {
        var d = Mask.digitos(v).slice(0, 11);
        if (d.length <= 2) return d.length ? '(' + d : '';
        if (d.length <= 6) return '(' + d.slice(0, 2) + ') ' + d.slice(2);
        if (d.length <= 10) return '(' + d.slice(0, 2) + ') ' + d.slice(2, 6) + '-' + d.slice(6);
        return '(' + d.slice(0, 2) + ') ' + d.slice(2, 7) + '-' + d.slice(7);
    },
    // CREF: 6 dígitos + "-" + letra + "/" + UF (ex.: 123456-G/SP)
    cref: function(v) {
        var s = String(v == null ? '' : v).toUpperCase().replace(/[^0-9A-Z]/g, '');
        var m = /^(\d{0,6})([A-Z]?)([A-Z]{0,2})/.exec(s);
        var out = m[1];
        if (m[2]) out += '-' + m[2];
        if (m[3]) out += '/' + m[3];
        return out;
    },

    // Centavos inteiros -> texto do input ("1.234,56"; 0 vira vazio para mostrar o placeholder)
    moedaTexto: function(centavos) {
        return centavos > 0 ? fmtBRL(centavos).slice(3) : '';
    },
    // Lê o valor de um input de moeda em centavos (0 se vazio)
    centavos: function(input) {
        return parseInt(input.dataset.centavos || '0', 10) || 0;
    },
    // Define o valor de um input de moeda (centavos)
    definirCentavos: function(input, centavos) {
        var max = parseInt(input.dataset.maxCentavos || '0', 10);
        if (max > 0 && centavos > max) centavos = max;
        input.dataset.centavos = String(centavos);
        input.value = Mask.moedaTexto(centavos);
    },

    aplicar: function(input, tipo) {
        if (!input || input._gsMask) return;
        input._gsMask = tipo;
        if (tipo === 'moeda') {
            input.setAttribute('inputmode', 'numeric');
            if (!input.placeholder) input.placeholder = '0,00';
            input.autocomplete = 'off';
            input.addEventListener('input', function() {
                var d = Mask.digitos(input.value).replace(/^0+/, '').slice(0, 10);
                Mask.definirCentavos(input, d ? parseInt(d, 10) : 0);
            });
            if (input.value) Mask.definirCentavos(input, reaisParaCentavos(input.value) || 0);
            return;
        }
        var fmt = Mask[tipo];
        if (typeof fmt !== 'function') return;
        input.setAttribute('inputmode', tipo === 'cref' ? 'text' : (tipo === 'telefone' ? 'tel' : 'numeric'));
        if (tipo === 'telefone') input.setAttribute('autocomplete', 'tel');
        else input.setAttribute('autocomplete', 'off');
        input.setAttribute('maxlength', { cpf: 14, cnpj: 18, telefone: 15, cref: 12 }[tipo]);
        input.addEventListener('input', function() { input.value = fmt(input.value); });
        if (input.value) input.value = fmt(input.value);
    },
    auto: function(raiz) {
        (raiz || document).querySelectorAll('[data-mask]').forEach(function(el) { Mask.aplicar(el, el.dataset.mask); });
    }
};

// Validadores (retornam boolean). CPF/CNPJ com dígito verificador.
var Valid = {
    cpf: function(v) {
        var d = Mask.digitos(v);
        if (d.length !== 11 || /^(\d)\1{10}$/.test(d)) return false;
        for (var t = 9; t < 11; t++) {
            var soma = 0;
            for (var i = 0; i < t; i++) soma += parseInt(d.charAt(i), 10) * (t + 1 - i);
            var dv = ((soma * 10) % 11) % 10;
            if (dv !== parseInt(d.charAt(t), 10)) return false;
        }
        return true;
    },
    cnpj: function(v) {
        var d = Mask.digitos(v);
        if (d.length !== 14 || /^(\d)\1{13}$/.test(d)) return false;
        for (var t = 12; t < 14; t++) {
            var soma = 0, peso = t - 7;
            for (var i = 0; i < t; i++) {
                soma += parseInt(d.charAt(i), 10) * peso;
                peso = peso === 2 ? 9 : peso - 1;
            }
            var dv = soma % 11 < 2 ? 0 : 11 - (soma % 11);
            if (dv !== parseInt(d.charAt(t), 10)) return false;
        }
        return true;
    },
    telefone: function(v) { var n = Mask.digitos(v).length; return n === 10 || n === 11; },
    cref: function(v) { return /^\d{4,6}-[A-Z]\/[A-Z]{2}$/.test(String(v || '').toUpperCase()); },
    email: function(v) { return /^[^\s@]+@[^\s@]+\.[^\s@]{2,}$/.test(String(v || '').trim()); }
};

// Mostra/limpa erro inline abaixo de um campo (aria-invalid + aria-describedby). msg vazia limpa.
function erroCampo(input, msg) {
    var id = (input.id || (input.id = 'campo' + Math.random().toString(36).slice(2, 8))) + '-erro';
    var el = document.getElementById(id);
    if (!msg) {
        input.removeAttribute('aria-invalid');
        input.removeAttribute('aria-describedby');
        if (el) el.remove();
        return;
    }
    if (!el) {
        el = h('p', { id: id, class: 'gs-field-erro', role: 'alert' });
        input.insertAdjacentElement('afterend', el);
    }
    el.textContent = msg;
    input.setAttribute('aria-invalid', 'true');
    input.setAttribute('aria-describedby', id);
}

// ─── Skeleton e estados vazios ───────────────────────────────────────────────
// Skeleton.cards(el, 3): preenche el com 3 cartões "carregando" (limpa o conteúdo anterior)
var Skeleton = {
    cards: function(el, n, altura) {
        limpar(el);
        el.setAttribute('aria-busy', 'true');
        for (var i = 0; i < (n || 3); i++) {
            el.appendChild(h('div', { class: 'gs-skeleton', style: { height: (altura || 96) + 'px' }, 'aria-hidden': 'true' }));
        }
    },
    // Chame ao terminar de renderizar o conteúdo real
    fim: function(el) { el.removeAttribute('aria-busy'); }
};

// estadoVazio({ icone: '🏆', titulo, texto, acao: { texto, href | onClick }, secundaria: {...} }) -> elemento
// Sempre diga o próximo passo em "acao". Para erro de carregamento use { erro: true, acao: {texto:'Tentar de novo', onClick} }.
function estadoVazio(o) {
    function botao(a, primario) {
        if (!a) return null;
        var attrs = { class: 'gs-btn ' + (primario ? 'gs-btn-primary' : 'gs-btn-secondary') };
        if (a.href) { attrs.href = a.href; return h('a', attrs, a.texto); }
        attrs.type = 'button';
        attrs.onClick = a.onClick;
        return h('button', attrs, a.texto);
    }
    return h('div', { class: 'gs-vazio' + (o.erro ? ' erro' : ''), role: o.erro ? 'alert' : null }, [
        o.icone ? h('div', { class: 'gs-vazio-icone', 'aria-hidden': 'true', text: o.icone }) : null,
        h('h3', { text: o.titulo }),
        o.texto ? h('p', { text: o.texto }) : null,
        (o.acao || o.secundaria) ? h('div', { class: 'gs-vazio-acoes' }, [botao(o.acao, true), botao(o.secundaria, false)]) : null
    ]);
}

// ─── Modal acessível ─────────────────────────────────────────────────────────
// Modal.abrir(overlay, {rotulo, aoFechar, remover, semEsc, semFundo}) em markup existente
//   (overlay = elemento de fundo; o diálogo é o 1º [role=dialog] dentro dele, ou o próprio overlay).
// Modal.fechar(overlay) · Modal.criar({titulo, corpo, acoes, aoFechar}) -> {el, corpo, fechar}
// Modal.confirmar({titulo, mensagem, linhas:[[rotulo,valor]], confirmar, cancelar, perigo}) -> Promise<boolean>
// Faz: foco inicial, trap de Tab, Esc, restauração de foco, inert no resto da página, trava de rolagem.
var Modal = {
    _pilha: [],
    _seq: 0,
    _FOCAVEIS: 'a[href],button:not([disabled]),input:not([disabled]):not([type="hidden"]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])',

    abrir: function(overlay, opts) {
        opts = opts || {};
        var dialog = overlay.querySelector('[role="dialog"],[role="alertdialog"]') || overlay;
        if (!dialog.getAttribute('role')) dialog.setAttribute('role', 'dialog');
        dialog.setAttribute('aria-modal', 'true');
        if (opts.rotulo) dialog.setAttribute('aria-label', opts.rotulo);
        if (!dialog.hasAttribute('tabindex')) dialog.setAttribute('tabindex', '-1');

        var topo = overlay;
        while (topo.parentNode && topo.parentNode !== document.body) topo = topo.parentNode;
        var estado = { overlay: overlay, dialog: dialog, topo: topo, opener: document.activeElement,
            aoFechar: opts.aoFechar, remover: !!opts.remover, semEsc: !!opts.semEsc, inertes: [] };

        Array.prototype.forEach.call(document.body.children, function(n) {
            if (n === topo || n.id === 'gsToast' || n.tagName === 'SCRIPT' || n.inert) return;
            n.inert = true;
            estado.inertes.push(n);
        });
        overlay.hidden = false;
        overlay.classList.add('aberto');
        document.body.classList.add('gs-modal-lock');
        if (!opts.semFundo) {
            estado.onFundo = function(e) { if (e.target === overlay) Modal.fechar(overlay); };
            overlay.addEventListener('mousedown', estado.onFundo);
        }
        Modal._pilha.push(estado);
        if (Modal._pilha.length === 1) document.addEventListener('keydown', Modal._onKey, true);

        var alvo = dialog.querySelector('[autofocus]') || dialog.querySelector(Modal._FOCAVEIS) || dialog;
        setTimeout(function() { try { alvo.focus(); } catch(e) {} }, 0);
        return estado;
    },

    fechar: function(overlay) {
        var idx = -1;
        Modal._pilha.forEach(function(s, i) { if (s.overlay === overlay) idx = i; });
        if (idx < 0) return;
        var s = Modal._pilha.splice(idx, 1)[0];
        s.inertes.forEach(function(n) { n.inert = false; });
        if (s.onFundo) overlay.removeEventListener('mousedown', s.onFundo);
        overlay.classList.remove('aberto');
        if (s.remover) overlay.remove(); else overlay.hidden = true;
        if (!Modal._pilha.length) {
            document.removeEventListener('keydown', Modal._onKey, true);
            document.body.classList.remove('gs-modal-lock');
        }
        if (s.opener && document.body.contains(s.opener) && s.opener.focus) { try { s.opener.focus(); } catch(e) {} }
        if (s.aoFechar) s.aoFechar();
    },

    _onKey: function(e) {
        var s = Modal._pilha[Modal._pilha.length - 1];
        if (!s) return;
        if (e.key === 'Escape' && !s.semEsc) { e.stopPropagation(); Modal.fechar(s.overlay); return; }
        if (e.key !== 'Tab') return;
        var itens = Array.prototype.filter.call(s.dialog.querySelectorAll(Modal._FOCAVEIS), function(el) { return el.offsetParent !== null || el === document.activeElement; });
        if (!itens.length) { e.preventDefault(); s.dialog.focus(); return; }
        var primeiro = itens[0], ultimo = itens[itens.length - 1];
        if (e.shiftKey && (document.activeElement === primeiro || document.activeElement === s.dialog)) { e.preventDefault(); ultimo.focus(); }
        else if (!e.shiftKey && document.activeElement === ultimo) { e.preventDefault(); primeiro.focus(); }
    },

    // corpo: Node | string | array (h()). acoes: [{texto, classe?, onClick?(fechar), fecha?:true}]
    criar: function(o) {
        var id = 'gsModalTitulo' + (++Modal._seq);
        var overlay, corpo = h('div', { class: 'gs-modal-corpo' }, o.corpo);
        function fechar() { Modal.fechar(overlay); }
        var rodape = (o.acoes && o.acoes.length) ? h('div', { class: 'gs-modal-acoes' }, o.acoes.map(function(a) {
            return h('button', { type: 'button', class: 'gs-btn ' + (a.classe || 'gs-btn-secondary'), onClick: function() {
                if (a.onClick) a.onClick(fechar);
                if (a.fecha !== false && !a.onClick) fechar();
            }, text: a.texto });
        })) : null;
        overlay = h('div', { class: 'gs-modal-overlay', hidden: true }, h('div', { class: 'gs-modal', role: o.alerta ? 'alertdialog' : 'dialog', 'aria-labelledby': id }, [
            h('div', { class: 'gs-modal-topo' }, [
                h('h2', { id: id, text: o.titulo }),
                h('button', { type: 'button', class: 'gs-modal-x', 'aria-label': 'Fechar', onClick: fechar, text: '×' })
            ]),
            corpo,
            rodape
        ]));
        document.body.appendChild(overlay);
        Modal.abrir(overlay, { remover: true, aoFechar: o.aoFechar });
        return { el: overlay, corpo: corpo, fechar: fechar };
    },

    confirmar: function(o) {
        return new Promise(function(resolve) {
            var decidido = false;
            function decidir(v, fechar) { decidido = true; resolve(v); if (fechar) fechar(); }
            var m = Modal.criar({
                titulo: o.titulo, alerta: true,
                corpo: [
                    o.mensagem ? h('p', { class: 'gs-modal-msg', text: o.mensagem }) : null,
                    (o.linhas && o.linhas.length) ? h('dl', { class: 'gs-modal-linhas' }, o.linhas.map(function(l) {
                        return h('div', null, [h('dt', { text: l[0] }), h('dd', { text: l[1] })]);
                    })) : null
                ],
                acoes: [
                    { texto: o.cancelar || 'Cancelar', onClick: function(f) { decidir(false, f); } },
                    { texto: o.confirmar || 'Confirmar', classe: o.perigo ? 'gs-btn-danger' : 'gs-btn-primary', onClick: function(f) { decidir(true, f); } }
                ],
                aoFechar: function() { if (!decidido) resolve(false); }
            });
            var ok = m.el.querySelector('.gs-modal-acoes button:last-child');
            if (ok) ok.setAttribute('autofocus', '');
        });
    }
};

// ─── Rótulos do ledger da carteira ───────────────────────────────────────────
var ROTULOS_TRANSACAO = {
    deposito: 'Depósito PIX',
    saque: 'Saque PIX',
    tarifa_saque: 'Tarifa de saque',
    entrada_desafio: 'Inscrição em desafio',
    estorno_desafio: 'Estorno de inscrição',
    premio_desafio: 'Prêmio de desafio',
    reserva_desafio: 'Reserva para desafio',
    estorno_reserva: 'Reserva devolvida',
    pagamento_instrutor: 'Pagamento de instrutor',
    receita_academia: 'Receita do desafio',
    ajuste: 'Saldo inicial migrado',
    estorno_anulacao: 'Estorno por anulação de resultado',
    abatimento_debito: 'Abatimento de débito'
};
function rotuloTransacao(tipo) { return ROTULOS_TRANSACAO[tipo] || 'Movimentação'; }

// ─── Desafios: rótulos e cartão compartilhado ────────────────────────────────
var ROTULOS_CATEGORIA = {
    geral: 'Geral', corrida: 'Corrida', musculacao: 'Musculação', crossfit: 'CrossFit', funcional: 'Funcional',
    ciclismo: 'Ciclismo', natacao: 'Natação', calistenia: 'Calistenia', luta: 'Luta', outros: 'Outros'
};
function rotuloCategoria(c) {
    return ROTULOS_CATEGORIA[c] || (c ? c.charAt(0).toUpperCase() + c.slice(1) : '');
}
var ROTULOS_STATUS_DESAFIO = { aberto: 'Aberto', em_andamento: 'Em andamento', encerrado: 'Encerrado', cancelado: 'Cancelado' };
function badgeStatus(status) {
    var k = ROTULOS_STATUS_DESAFIO[status] ? status : 'aberto';
    return h('span', { class: 'status-badge status-' + k, text: ROTULOS_STATUS_DESAFIO[status] || status });
}
// Prêmio total do pódio em centavos (premiacao.* do Sprint 6; cai para a soma da prevista)
function premioTotalCentavos(d) {
    if (d.premiacao && d.premiacao.premio_total_centavos != null) return d.premiacao.premio_total_centavos;
    return (d.premiacao_prevista_centavos || []).reduce(function(s, v) { return s + v; }, 0);
}
function premiosPorPosicao(d) {
    return (d.premiacao && d.premiacao.por_posicao_centavos) || d.premiacao_prevista_centavos || [];
}
// Cartão de desafio (feed e lista). Link real: abre em nova aba, mostra a URL, funciona com teclado.
function cartaoDesafio(d) {
    var vagas = d.vagas || 0, inscritos = d.participantes_count || 0;
    var pct = vagas > 0 ? Math.min(100, Math.round(inscritos / vagas * 100)) : 0;
    var lotado = vagas > 0 && inscritos >= vagas;
    var quando = d.data_evento ? fmtDataHora(d.data_evento) : 'Data a definir';
    var contagem = d.status === 'aberto' && d.data_evento ? fmtContagem(d.data_evento) : '';
    return h('a', { class: 'desafio-card', href: '/desafios?id=' + encodeURIComponent(d.id) }, [
        h('div', { class: 'desafio-card-topo' }, [
            d.categoria ? h('span', { class: 'chip-tag', text: rotuloCategoria(d.categoria) }) : null,
            d.status !== 'aberto' ? badgeStatus(d.status) : null,
            d.voce_participa ? h('span', { class: 'tag-participa', text: 'Inscrito' }) : null
        ]),
        h('h3', { class: 'desafio-card-titulo', text: d.titulo }),
        d.academia ? h('p', { class: 'desafio-card-sub', text: d.academia.nome_fantasia }) : null,
        h('p', { class: 'desafio-card-meta', text: quando + (contagem ? ' · ' + contagem : '') + (d.local ? ' · ' + d.local : '') }),
        h('div', { class: 'desafio-card-fin' }, [
            h('span', null, ['Entrada ', h('strong', { text: d.valor_entrada_centavos > 0 ? fmtBRL(d.valor_entrada_centavos) : 'Grátis' })]),
            h('span', { class: 'premio' }, ['Prêmio ', h('strong', { text: premioTotalCentavos(d) > 0 || d.status !== 'aberto' ? fmtBRL(premioTotalCentavos(d)) : 'cresce com as inscrições' })])
        ]),
        vagas > 0 ? h('div', { class: 'desafio-card-vagas' }, [
            h('div', { class: 'vagas-linha' }, [h('span', { text: lotado ? 'Lotado' : 'Vagas ' + inscritos + '/' + vagas }), h('span', { text: pct + '%' })]),
            h('div', { class: 'gs-progresso', role: 'progressbar', 'aria-valuemin': '0', 'aria-valuemax': String(vagas), 'aria-valuenow': String(inscritos), 'aria-label': 'Vagas preenchidas' },
                h('span', { style: { width: pct + '%' } }))
        ]) : null
    ]);
}

// ─── Auth ────────────────────────────────────────────────────────────────────
// Sessão só pelo cookie HttpOnly `jwt` emitido pelo backend (o JS nunca vê nem guarda o token).
// No localStorage fica apenas o mínimo para exibição/roteamento (id, nome, perfil); CPF, saldo,
// e-mail e demais dados ficam só em memória, carregados de /api/usuarios/me (Auth.refresh()).
var _CAMPOS_USUARIO_LOCAL = ['id', 'nome', 'sobrenome', 'perfil', 'username'];
var Auth = {
    _mem: null, // perfil completo da página atual (vem de /usuarios/me)
    getUser: function() {
        if (Auth._mem) return Auth._mem;
        try { return JSON.parse(localStorage.getItem('gym_user')); } catch(e) { return null; }
    },
    // Guarda o usuário: completo em memória, mínimo no localStorage
    // Se o id mudou (outra conta logou por cookie), o cache anterior é descartado e reescrito.
    saveUser: function(user) {
        if (!user) return;
        var anterior = Auth._mem;
        if (!anterior) { try { anterior = JSON.parse(localStorage.getItem('gym_user')); } catch(e) { anterior = null; } }
        if (anterior && anterior.id !== user.id) { Auth.clear(); Auth._verificado = null; }
        Auth._mem = user;
        var min = {};
        _CAMPOS_USUARIO_LOCAL.forEach(function(k) { if (user[k] !== undefined) min[k] = user[k]; });
        try { localStorage.setItem('gym_user', JSON.stringify(min)); } catch(e) {}
    },
    logado: function() { return !!Auth.getUser(); },
    clear: function() {
        Auth._mem = null;
        try {
            localStorage.removeItem('gym_user');
            localStorage.removeItem('gym_token'); // resto de versões antigas
        } catch(e) {}
    },
    // Revoga a sessão no backend (o cookie HttpOnly é limpo pela resposta) e depois limpa o cache local
    logout: async function() {
        try { await fetch('/api/logout', { method: 'POST', credentials: 'same-origin' }); } catch(e) {}
        Auth.clear();
        window.location.href = '/login';
    },
    // Sem sessão conhecida: vai ao login guardando o destino (?next=). Se o cookie tiver expirado,
    // a primeira chamada à API devolve 401 e _req manda para o login.
    require: function() {
        if (!Auth.logado()) {
            var destino = window.location.pathname + window.location.search;
            window.location.href = '/login' + (destino !== '/' ? '?next=' + encodeURIComponent(destino) : '');
            return false;
        }
        return true;
    },
    // Destino pós-login: ?next= (só caminho interno) ou a home do perfil
    destinoPosLogin: function(perfil) {
        var next = new URLSearchParams(window.location.search).get('next');
        if (next && Notificacoes._linkSeguro(next) && next.indexOf('/login') !== 0) return next;
        return perfil === 'academia' ? '/academia' : perfil === 'instrutor' ? '/instrutor' : '/menu';
    },
    // Identidade/perfil confirmados pelo servidor (/usuarios/me), uma vez por página. O cache
    // gym_user só serve para exibição: guardas de perfil decidem por aqui. Resolve com o usuário,
    // ou null quando não há sessão (401: _req já limpa o cache e manda para o login, se a página
    // não for pública). Outros erros (rede/5xx) rejeitam a promise.
    _verificado: null,
    verificar: function() {
        if (!Auth._verificado) {
            Auth._verificado = API.me().then(function(resp) {
                if (!resp.data || !resp.data.id) throw new Error('Resposta inválida de /usuarios/me');
                Auth.saveUser(resp.data);
                return resp.data;
            }, function(e) {
                Auth._verificado = null;
                if (e && e.status === 401) { Auth.clear(); return null; }
                throw e;
            });
        }
        return Auth._verificado;
    },
    // Rebusca o perfil completo (saldo, cpf, elo, pontos...) em /usuarios/me
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
        { href: '/desafios',  label: 'Desafios',  icon: 'podio'     },
        { href: '/ingressos', label: 'Ingressos', icon: 'ingressos' },
        { href: '/amigos',    label: 'Amigos',    icon: 'amigos'    },
        { href: '/perfil',    label: 'Perfil',    icon: 'perfil'    }
    ],
    academia: [
        { href: '/academia',              label: 'Dashboard',  icon: 'dashboard'   },
        { href: '/academia/desafios',     label: 'Desafios',   icon: 'agenda'      },
        { href: '/academia/criar-desafio',label: 'Criar',      icon: 'criar'       },
        { href: '/academia/instrutores',  label: 'Instrutores',icon: 'instrutores' },
        { href: '/perfil',                label: 'Perfil',     icon: 'perfil'      }
    ],
    instrutor: [
        { href: '/instrutor',         label: 'Agenda',  icon: 'agenda'    },
        { href: '/instrutor/vagas',   label: 'Vagas',   icon: 'ingressos' },
        { href: '/instrutor/ganhos',  label: 'Ganhos',  icon: 'carteira'  },
        { href: '/perfil',            label: 'Perfil',  icon: 'perfil'    }
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
        if (isActive) { a.setAttribute('aria-current', 'page'); a.classList.add('active'); }
        a.appendChild(_navIcon(l.icon));
        a.appendChild(h('span', { text: l.label }));
        nav.appendChild(a);
    });

    // Sino de notificações (antes de "Perfil")
    var bell = h('button', { type: 'button', id: 'bellBtn', class: 'nav-bell', 'aria-label': 'Notificações', 'aria-haspopup': 'dialog', 'aria-expanded': 'false' }, [
        _navIcon('sino'),
        h('span', { id: 'bellBadge', class: 'bell-badge', hidden: true, 'aria-hidden': 'true' }),
        h('span', { text: 'Avisos' })
    ]);
    bell.addEventListener('click', function() { Notificacoes.abrir(); });
    nav.insertBefore(bell, nav.lastChild);

    document.body.appendChild(nav);
    document.body.classList.add('has-bottom-nav');
    Notificacoes.iniciar();
    Moderacao.verificarSuspensao();
}

// Notificações in-app: contador por polling (30 s) + painel com lista
var Notificacoes = {
    _timer: null,
    _painel: null,
    _naoLidas: 0,

    iniciar: function() {
        if (Notificacoes._timer || !Auth.logado()) return;
        Notificacoes.atualizarContador();
        Notificacoes._timer = setInterval(function() {
            if (document.hidden) return;
            Notificacoes.atualizarContador();
            if (Notificacoes._painel) Notificacoes.carregarLista();
        }, 30000);
        document.addEventListener('visibilitychange', function() {
            if (!document.hidden) Notificacoes.atualizarContador();
        });
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
        var base = document.title.replace(/^\(\d+\+?\) /, '');
        document.title = (n > 0 ? '(' + (n > 99 ? '99+' : n) + ') ' : '') + base;
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
        var overlay = h('div', { class: 'notif-overlay' }, sheet);
        document.body.appendChild(overlay);
        Notificacoes._painel = overlay;
        var bellBtn = document.getElementById('bellBtn');
        if (bellBtn) bellBtn.setAttribute('aria-expanded', 'true');
        Modal.abrir(overlay, { remover: true, aoFechar: function() {
            Notificacoes._painel = null;
            var b = document.getElementById('bellBtn');
            if (b) b.setAttribute('aria-expanded', 'false');
        } });
        Notificacoes.carregarLista();
    },

    fechar: function() {
        if (!Notificacoes._painel) return;
        Modal.fechar(Notificacoes._painel);
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

// ─── Moderação: rótulos, aviso de suspensão, imagens autenticadas e modal de texto ──
var Moderacao = {
    CATEGORIAS: [
        ['suborno_instrutor', 'Suborno de instrutor'],
        ['manipulacao_podio', 'Manipulação de pódio'],
        ['fraude_presenca', 'Fraude de presença'],
        ['conduta_abusiva', 'Conduta abusiva'],
        ['perfil_falso', 'Perfil falso'],
        ['outro', 'Outro']
    ],
    // Estas três exigem o desafio (o backend recusa sem id_desafio)
    CATEGORIAS_DESAFIO: ['suborno_instrutor', 'manipulacao_podio', 'fraude_presenca'],
    STATUS_DENUNCIA: { aberta: 'Aberta', em_analise: 'Em análise', procedente: 'Procedente', improcedente: 'Improcedente', arquivada: 'Arquivada' },
    TIPOS_PUNICAO: { advertencia: 'Advertência', suspensao: 'Suspensão', banimento: 'Banimento', bloqueio_academia: 'Bloqueio na academia', anulacao_resultado: 'Anulação de resultado' },
    STATUS_RECURSO: { pendente: 'Em análise', deferido: 'Deferido', indeferido: 'Indeferido' },
    STATUS_PENDENTE: { a_liberar: 'A liberar', retido: 'Retido por contestação', liberado: 'Liberado', cancelado: 'Cancelado' },
    TIPOS_PENDENTE: { premio_desafio: 'Prêmio de desafio', pagamento_instrutor: 'Pagamento de instrutor', receita_academia: 'Receita do desafio', taxa_inscricao: 'Taxa da plataforma' },
    MIMES_IMAGEM: ['image/jpeg', 'image/png', 'image/webp'],
    EVIDENCIA_MAX_BYTES: 2 * 1024 * 1024, // DENUNCIA_EVIDENCIA_MAX_BYTES padrão do backend

    rotuloCategoria: function(c) {
        for (var i = 0; i < Moderacao.CATEGORIAS.length; i++) if (Moderacao.CATEGORIAS[i][0] === c) return Moderacao.CATEGORIAS[i][1];
        return c || '—';
    },
    // Selo de status (classes .md-selo.<status> em base.css)
    selo: function(texto, chave) { return h('span', { class: 'md-selo md-' + chave, text: texto }); },
    seloDenuncia: function(st) { return Moderacao.selo(Moderacao.STATUS_DENUNCIA[st] || st, st); },

    // Fim da suspensão vigente do usuário da página (de /usuarios/me) ou null
    suspensoAte: function() {
        var u = Auth._mem;
        var raw = u && u.moderacao && u.moderacao.suspenso_ate;
        if (!raw) return null;
        var d = new Date(raw);
        return isNaN(d.getTime()) || d.getTime() <= Date.now() ? null : d;
    },

    // Faixa fixa no topo: "Conta suspensa até ... Motivo: ..." (uma por página)
    avisarSuspensao: function(dados) {
        dados = dados || {};
        var u = Auth._mem;
        if (u) {
            u.moderacao = u.moderacao || {};
            if (dados.suspenso_ate) u.moderacao.suspenso_ate = dados.suspenso_ate;
        }
        var ate = dados.suspenso_ate ? new Date(dados.suspenso_ate) : Moderacao.suspensoAte();
        if (!ate || isNaN(ate.getTime())) return;
        var motivo = dados.motivo || (u && u.moderacao && u.moderacao.motivo) || '';
        var el = document.getElementById('mdAvisoSuspensao');
        if (!el) {
            el = h('div', { id: 'mdAvisoSuspensao', class: 'md-aviso-suspensao', role: 'status' });
            document.body.insertBefore(el, document.body.firstChild);
            document.body.classList.add('md-com-aviso');
        }
        limpar(el).appendChild(h('div', { class: 'md-aviso-texto' }, [
            h('strong', { text: 'Conta suspensa até ' + fmtDataHora(ate) + '.' }),
            ' Você pode ver seus dados, usar a carteira e recorrer, mas não pode entrar em desafios nem fazer ações de gestão.',
            motivo ? h('span', { class: 'md-aviso-motivo', text: 'Motivo: ' + motivo }) : null
        ]));
        el.appendChild(h('a', { class: 'md-aviso-link', href: '/minhas-punicoes', text: 'Ver punição e recorrer' }));
    },
    // Chamado pelo renderBottomNav: confere /usuarios/me (uma vez por página) e mostra o aviso
    verificarSuspensao: function() {
        if (!Auth.logado()) return;
        Auth.verificar().then(function(u) {
            if (u && Moderacao.suspensoAte()) Moderacao.avisarSuspensao(u.moderacao);
        }, function() {});
    },

    // Evidência: baixa pela rota autenticada (cookie) e devolve um object URL (nunca caminho público).
    // Recusa o que não for imagem da whitelist. Os URLs são revogados ao sair da página.
    _urls: [],
    imagemAutenticada: async function(url) {
        var res = await fetch(url, { credentials: 'same-origin', cache: 'no-store' });
        if (!res.ok) { var e = new Error(res.status === 404 ? 'Imagem indisponível.' : 'Erro ao carregar a imagem.'); e.status = res.status; throw e; }
        var blob = await res.blob();
        if (Moderacao.MIMES_IMAGEM.indexOf(blob.type) < 0) throw new Error('Arquivo não é uma imagem válida.');
        var obj = URL.createObjectURL(blob);
        Moderacao._urls.push(obj);
        return obj;
    },
    // Miniaturas das evidências: lista [{ordem, mime, tamanho_bytes}] + função ordem -> URL da rota autenticada
    galeria: function(evidencias, urlDe) {
        var box = h('div', { class: 'md-galeria' });
        (evidencias || []).forEach(function(ev) {
            var fig = h('figure', { class: 'md-thumb' }, h('span', { class: 'md-thumb-carregando', text: 'Carregando…' }));
            box.appendChild(fig);
            Moderacao.imagemAutenticada(urlDe(ev.ordem)).then(function(src) {
                var img = h('img', { src: src, alt: 'Evidência ' + ev.ordem });
                var link = h('button', { type: 'button', class: 'md-thumb-btn', 'aria-label': 'Ampliar evidência ' + ev.ordem }, img);
                link.addEventListener('click', function() {
                    Modal.criar({ titulo: 'Evidência ' + ev.ordem, corpo: h('img', { class: 'md-img-grande', src: src, alt: 'Evidência ' + ev.ordem }) });
                });
                limpar(fig).appendChild(link);
                fig.appendChild(h('figcaption', { text: '#' + ev.ordem + ' · ' + Math.max(1, Math.round((ev.tamanho_bytes || 0) / 1024)) + ' KB' }));
            }, function(err) {
                limpar(fig).appendChild(h('span', { class: 'md-thumb-erro', text: err.message }));
            });
        });
        return box;
    },

    // Modal com campo de texto obrigatório (contador, min..max) e confirmação.
    // o: {titulo, mensagem, linhas, rotulo, min, max, placeholder, extra: Node, validar() -> erro|'' ,
    //     confirmar, perigo, executar(texto) -> Promise}. Resolve com o resultado ou null se cancelado.
    textoModal: function(o) {
        return new Promise(function(resolve) {
            var concluido = false, ocupado = false;
            var min = o.min || 3, max = o.max || 2000;
            var id = 'mdTexto' + (++Modal._seq);
            var contador = h('span', { class: 'md-contador', 'aria-live': 'polite', text: '0/' + max });
            var ta = h('textarea', { id: id, class: 'gs-input md-textarea', rows: '5', maxlength: String(max), required: true,
                placeholder: o.placeholder || '', 'aria-describedby': id + '-ajuda' });
            ta.addEventListener('input', function() { contador.textContent = ta.value.trim().length + '/' + max; });
            var erro = h('p', { class: 'gs-msg error', role: 'alert' });
            var btnOk;
            var m = Modal.criar({
                titulo: o.titulo, alerta: !!o.perigo,
                corpo: [
                    o.mensagem ? h('p', { class: 'gs-modal-msg', text: o.mensagem }) : null,
                    (o.linhas && o.linhas.length) ? h('dl', { class: 'gs-modal-linhas' }, o.linhas.map(function(l) {
                        return h('div', null, [h('dt', { text: l[0] }), h('dd', { text: l[1] })]);
                    })) : null,
                    o.extra || null,
                    h('div', { class: 'gs-field' }, [
                        h('label', { class: 'gs-label', for: id, text: o.rotulo || 'Justificativa' }),
                        ta,
                        h('div', { class: 'md-ajuda', id: id + '-ajuda' }, [h('span', { text: 'Obrigatório, de ' + min + ' a ' + max + ' caracteres.' }), contador])
                    ]),
                    erro
                ],
                acoes: [
                    { texto: 'Cancelar', onClick: function(f) { if (!ocupado) f(); } },
                    { texto: o.confirmar || 'Confirmar', classe: o.perigo ? 'gs-btn-danger' : 'gs-btn-primary', onClick: function() { enviar(); } }
                ],
                aoFechar: function() { if (!concluido) resolve(null); }
            });
            btnOk = m.el.querySelector('.gs-modal-acoes button:last-child');
            function mostrar(msg) { erro.textContent = msg || ''; erro.classList.toggle('show', !!msg); }
            async function enviar() {
                if (ocupado) return;
                var texto = ta.value.trim();
                if (texto.length < min || texto.length > max) {
                    ta.setAttribute('aria-invalid', 'true');
                    mostrar('O texto precisa ter entre ' + min + ' e ' + max + ' caracteres.');
                    ta.focus();
                    return;
                }
                ta.removeAttribute('aria-invalid');
                var msgVal = o.validar ? o.validar() : '';
                if (msgVal) { mostrar(msgVal); return; }
                mostrar('');
                ocupado = true;
                btnLoading(btnOk, true);
                try {
                    var r = await o.executar(texto);
                    concluido = true;
                    ocupado = false;
                    m.fechar();
                    resolve(r);
                } catch (e) {
                    ocupado = false;
                    btnLoading(btnOk, false);
                    mostrar(mensagemErro(e));
                }
            }
        });
    }
};
window.addEventListener('pagehide', function() {
    Moderacao._urls.forEach(function(u) { try { URL.revokeObjectURL(u); } catch (e) {} });
    Moderacao._urls = [];
});

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

    var banner = h('div', { id: 'lgpdBanner', role: 'region', 'aria-label': 'Aviso de privacidade' }, [
        h('p', null, [
            h('strong', { text: 'Seus dados, seu controle.' }),
            ' O GymScore coleta nome, e-mail, CPF e data de nascimento para operação da plataforma, conforme a ',
            h('a', { href: '/privacidade', text: 'Política de Privacidade' }),
            ' (LGPD). Dados sensíveis ficam ',
            h('strong', { text: 'ocultos por padrão' }),
            '.'
        ]),
        h('button', { type: 'button', class: 'gs-btn gs-btn-primary', onClick: acceptLGPD, text: 'Entendi e aceito' })
    ]);
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
    var headers = Object.assign({}, options.headers || {});
    // FormData define o próprio Content-Type (com boundary)
    if (!(options.body instanceof FormData) && !headers['Content-Type']) headers['Content-Type'] = 'application/json';

    // Sessão pelo cookie HttpOnly (same-origin); o fetch manda Origin nos POST/PATCH/DELETE (CSRF)
    var res = await fetch(path, Object.assign({}, options, { headers: headers, credentials: 'same-origin' }));
    var json = null;
    try { json = await res.json(); } catch(e) { json = null; }

    if (!res.ok) {
        // Sessão expirada em rota protegida: volta para o login
        // (não dispara em rotas públicas de auth nem em páginas públicas, evitando loop de redirecionamento)
        if (res.status === 401 && !_rotaPublica(path) && !_paginaPublica()) {
            Auth.clear();
            var destino = window.location.pathname + window.location.search;
            window.location.href = '/login' + (destino !== '/' ? '?next=' + encodeURIComponent(destino) : '');
        }
        var msg = (json && (json.error || json.message)) ||
            (res.status === 429 ? 'Muitas tentativas. Aguarde um instante.' :
             res.status === 413 ? 'Arquivo grande demais.' : 'Erro na requisição');
        var err = new Error(msg);
        err.status = res.status;
        // Erros com código estável (moderação): conta_banida, conta_suspensa, debito_aberto...
        if (json && json.codigo) { err.codigo = json.codigo; err.data = json.data || null; }
        if (err.codigo === 'conta_banida' && path !== '/api/login' && path !== '/api/recursos/banimento') {
            // Banido com sessão aberta: a sessão não vale mais; a tela de login mostra o motivo e o recurso
            Auth.clear();
            window.location.href = '/login?banido=1';
        }
        if (err.codigo === 'conta_suspensa') Moderacao.avisarSuspensao(err.data);
        // 403 de academia/instrutor ainda não aprovado (B2): {status_cadastro: pendente|rejeitada}
        if (json && json.status_cadastro) {
            err.statusCadastro = json.status_cadastro;
            if (res.status === 403) CadastroStatus.mostrar(json.status_cadastro);
        }
        throw err;
    }
    return json || {};
}

// ─── Cadastro de academia/instrutor aguardando aprovação do admin ────────────
// Chamado por _req no 403 com status_cadastro: esconde o conteúdo da página e mostra o aviso
// (para o instrutor, também o convite de vínculo, que funciona mesmo pendente).
var CadastroStatus = {
    _mostrado: false,
    mostrar: function(status) {
        if (CadastroStatus._mostrado) return;
        CadastroStatus._mostrado = true;
        var perfil = (Auth.getUser() || {}).perfil;
        var tipo = perfil === 'academia' ? 'academia' : 'instrutor';
        var rejeitado = status === 'rejeitada';
        var raiz = document.querySelector('main') || document.body;
        Array.prototype.forEach.call(raiz.children, function(c) { c.hidden = true; });
        var convite = perfil === 'instrutor' ? h('div', { class: 'gs-cadastro-convite' }) : null;
        var card = h('section', { class: 'gs-card gs-cadastro-status' + (rejeitado ? ' rejeitado' : ''), role: 'status', 'aria-live': 'polite' }, [
            h('span', { class: 'gs-cadastro-icone', 'aria-hidden': 'true', text: rejeitado ? '✕' : '⏳' }),
            h('h1', { text: rejeitado ? 'Cadastro não aprovado' : 'Cadastro em análise' }),
            h('p', { text: rejeitado
                ? 'A equipe GymScore não aprovou seu cadastro de ' + tipo + '. O motivo está nas suas notificações. Se quiser revisar, fale com o suporte.'
                : 'Seu cadastro de ' + tipo + ' está aguardando a aprovação da equipe GymScore. Você recebe uma notificação assim que ele for analisado.' }),
            convite,
            h('div', { class: 'gs-cadastro-acoes' }, [
                h('a', { class: 'gs-btn gs-btn-secondary', href: '/perfil', text: 'Meu perfil' }),
                h('button', { type: 'button', class: 'gs-btn gs-btn-secondary', text: 'Sair', onClick: function() { Auth.logout(); } })
            ])
        ]);
        raiz.insertBefore(card, raiz.firstChild);
        if (convite && typeof Gestao !== 'undefined' && Gestao.vinculo) Gestao.vinculo(convite, { soConvite: true });
    }
};

function _rotaPublica(path) {
    return path === '/api/login' || path === '/api/logout' || path.indexOf('/api/auth/') === 0 || path === '/api/recursos/banimento';
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
    // O backend revoga as sessões antigas e já devolve o cookie novo na resposta
    alterarSenha: function(senhaAtual, novaSenha) {
        return _req('/api/usuarios/senha', {
            method: 'PATCH',
            body: JSON.stringify({ senha_atual: senhaAtual, nova_senha: novaSenha })
        });
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

    // Desafios (contrato Sprint 6: envelope {itens,total,pagina,limite}; dinheiro em centavos)
    // filtros: objeto {status, categoria, q, id_academia, pagina, limite}. Aceita também a forma
    // antiga (status, pagina, limite) para não quebrar chamadas existentes.
    listarDesafios: function(filtros, pagina, limite) {
        if (filtros === null || typeof filtros !== 'object') filtros = { status: filtros, pagina: pagina, limite: limite };
        return _req('/api/desafios' + _qs(filtros));
    },
    listarMeusDesafios: function(filtros, pagina, limite) {
        if (filtros === null || typeof filtros !== 'object') filtros = { status: filtros, pagina: pagina, limite: limite };
        return _req('/api/desafios/meus' + _qs(filtros));
    },
    categoriasDesafio: function() { return _req('/api/desafios/categorias'); },
    buscarDesafio: function(id) { return _req('/api/desafios/' + encodeURIComponent(id)); },
    criarDesafio: function(dados) { return _post('/api/desafios', dados); },
    editarDesafio: function(id, dados) {
        return _req('/api/desafios/' + encodeURIComponent(id), { method: 'PATCH', body: JSON.stringify(dados) });
    },
    entrarDesafio: function(id) { return _post('/api/desafios/' + encodeURIComponent(id) + '/entrar'); },
    sairDesafio: function(id) { return _post('/api/desafios/' + encodeURIComponent(id) + '/sair'); },
    cancelarDesafio: function(id) { return _post('/api/desafios/' + encodeURIComponent(id) + '/cancelar'); },
    iniciarDesafio: function(id) { return _post('/api/desafios/' + encodeURIComponent(id) + '/iniciar'); },
    convidarAmigo: function(id, idAmigo) {
        return _post('/api/desafios/' + encodeURIComponent(id) + '/convidar', { id_amigo: idAmigo });
    },
    // Taxas da plataforma: {modo, taxa_inscricao_pct, tarifa_saque_centavos}
    taxasPlataforma: function() { return _req('/api/plataforma/taxas'); },

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


    // ═══ INÍCIO: métodos de ACADEMIA e INSTRUTOR (Sprint 6) ═══════════════════
    // Bloco reservado a front-s6-gestao: acrescente os métodos aqui (releia o arquivo antes de editar).
    // Contrato em DEV-BACKEND-SPRINT6.md (Partes A, B e Financeiro/leilão). Dinheiro sempre em centavos.
    // Academia (sempre escopado pela academia do JWT)
    listarDesafiosAcademia: function(status, pagina, limite) {
        return _req('/api/academia/desafios' + _qs({ status: status, pagina: pagina, limite: limite }));
    },
    listarInstrutoresVinculados: function(pagina, limite) {
        return _req('/api/academia/instrutores' + _qs({ pagina: pagina, limite: limite }));
    },
    vincularInstrutor: function(email) { return _post('/api/academia/instrutores', { email: email }); },
    desvincularInstrutor: function(id) {
        return _req('/api/academia/instrutores/' + encodeURIComponent(id), { method: 'DELETE' });
    },
    listarCandidaturas: function(idDesafio) {
        return _req('/api/desafios/' + encodeURIComponent(idDesafio) + '/candidaturas');
    },
    escolherCandidatura: function(idDesafio, idCandidatura) {
        return _post('/api/desafios/' + encodeURIComponent(idDesafio) + '/candidaturas/' + encodeURIComponent(idCandidatura) + '/escolher');
    },

    // Instrutor
    agendaInstrutor: function(status, pagina, limite) {
        return _req('/api/instrutor/agenda' + _qs({ status: status, pagina: pagina, limite: limite }));
    },
    listarVagas: function(pagina, limite, categoria, q) {
        return _req('/api/instrutor/vagas' + _qs({ pagina: pagina, limite: limite, categoria: categoria, q: q }));
    },
    candidatar: function(idDesafio, lanceCentavos, mensagem) {
        return _post('/api/desafios/' + encodeURIComponent(idDesafio) + '/candidaturas',
            { lance_centavos: lanceCentavos, mensagem: mensagem || '' });
    },
    retirarCandidatura: function(idDesafio) {
        return _req('/api/desafios/' + encodeURIComponent(idDesafio) + '/candidaturas', { method: 'DELETE' });
    },
    // Presença: exatamente um entre codigo (conteúdo do QR, 32 hex) e idUsuario (marcação na lista)
    marcarPresencaPor: function(idDesafio, porCodigo, valor) {
        var corpo = porCodigo ? { codigo_ingresso: valor } : { id_usuario: valor };
        return _post('/api/desafios/' + encodeURIComponent(idDesafio) + '/presenca', corpo);
    },
    // Convite de vínculo academia → instrutor (o vínculo só existe depois do aceite)
    listarConvitesAcademia: function() { return _req('/api/academia/instrutores/convites'); },
    cancelarConviteAcademia: function(idInstrutor) {
        return _req('/api/academia/instrutores/convites/' + encodeURIComponent(idInstrutor), { method: 'DELETE' });
    },
    // {convite: {id_academia, nome_fantasia, convite_em} | null, id_academia}
    meuConvite: function() { return _req('/api/instrutor/convite'); },
    aceitarConvite: function() { return _post('/api/instrutor/convite/aceitar'); },
    recusarConvite: function() { return _post('/api/instrutor/convite/recusar'); },
    // O instrutor deixa a academia (409 se designado em desafio ativo)
    sairDaAcademia: function() { return _req('/api/instrutor/vinculo', { method: 'DELETE' }); },
    // ═══ FIM: métodos de ACADEMIA e INSTRUTOR ════════════════════════════════

    // ─── Moderação (rodada 3) ───────────────────────────────────────────────
    // Denúncia: {id_denunciado, id_desafio?, categoria, descricao} -> {id, status, jurisdicao, ...}
    criarDenuncia: function(dados) { return _post('/api/denuncias', dados); },
    // Uma imagem por chamada (multipart, campo "arquivo"); até 3 por denúncia, só com status "aberta"
    enviarEvidencia: function(idDenuncia, arquivo) {
        var fd = new FormData();
        fd.append('arquivo', arquivo);
        return _req('/api/denuncias/' + encodeURIComponent(idDenuncia) + '/evidencias', { method: 'POST', body: fd });
    },
    minhasDenuncias: function(pagina, limite) { return _req('/api/denuncias/minhas' + _qs({ pagina: pagina, limite: limite })); },
    minhaDenuncia: function(id) { return _req('/api/denuncias/' + encodeURIComponent(id)); },
    urlEvidenciaMinha: function(id, ordem) { return '/api/denuncias/' + encodeURIComponent(id) + '/evidencias/' + encodeURIComponent(ordem); },
    minhasPunicoes: function(pagina, limite) { return _req('/api/punicoes/minhas' + _qs({ pagina: pagina, limite: limite })); },
    recorrer: function(idPunicao, texto) { return _post('/api/punicoes/' + encodeURIComponent(idPunicao) + '/recurso', { texto: texto }); },
    // Q1: banido recorre sem sessão (reconfere a senha). Sem texto = só consulta o andamento.
    recursoBanimento: function(email, senha, texto) {
        var corpo = { email: email, senha: senha };
        if (texto) corpo.texto = texto;
        return _post('/api/recursos/banimento', corpo);
    },
    carteiraPendentes: function(pagina, limite) { return _req('/api/carteira/pendentes' + _qs({ pagina: pagina, limite: limite })); },
    carteiraDebitos: function() { return _req('/api/carteira/debitos'); },

    // Academia julgadora (só suborno_instrutor de desafio dela, enquanto a jurisdição for dela)
    academiaDenuncias: function(status, pagina, limite) {
        return _req('/api/academia/denuncias' + _qs({ status: status, pagina: pagina, limite: limite }));
    },
    academiaDenuncia: function(id) { return _req('/api/academia/denuncias/' + encodeURIComponent(id)); },
    urlEvidenciaAcademia: function(id, ordem) { return '/api/academia/denuncias/' + encodeURIComponent(id) + '/evidencias/' + encodeURIComponent(ordem); },
    academiaAssumir: function(id) { return _post('/api/academia/denuncias/' + encodeURIComponent(id) + '/assumir'); },
    academiaEncaminhar: function(id, justificativa) {
        return _post('/api/academia/denuncias/' + encodeURIComponent(id) + '/encaminhar', { justificativa: justificativa });
    },
    // corpo: {status: procedente|improcedente|arquivada, decisao_texto, punicao?: {tipo: advertencia|bloqueio_academia, fim?}}
    academiaDecidir: function(id, corpo) { return _post('/api/academia/denuncias/' + encodeURIComponent(id) + '/decidir', corpo); },
    academiaPunicoes: function(tipo, vigente, pagina, limite) {
        return _req('/api/academia/punicoes' + _qs({ tipo: tipo, vigente: vigente, pagina: pagina, limite: limite }));
    }
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

// Aplica as máscaras declarativas (data-mask) assim que o DOM estiver pronto
if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', function() { Mask.auto(); });
else Mask.auto();
