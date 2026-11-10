// Script da página academia-criar-desafio.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
// Rodadas 5+6: endereço estruturado (js/endereco.js) pré-preenchido com o da academia, início e fim,
// equipe de instrutores (nº de vagas, indicados vinculados, responsável) com pagamento POR instrutor,
// resumo/regras obrigatórios, nº de premiados + divisão por posição (pontos-base) e filtros de nível/idade.
(async function () {
var user = await Gestao.exigirPerfil('academia');
if (!user) return; // redirecionado para o login/home

var idEdicao = Gestao.param('id');
var modoEdicao = !!idEdicao;
var CHAVE_RASCUNHO = 'gx_rascunho_desafio_v2';

var $ = function(id) { return document.getElementById(id); };
var taxaPct = 10;          // % da plataforma (GET /plataforma/taxas; na edição vale o snapshot do desafio)
var saldo = null;           // centavos; null = não foi possível ler
var original = null;        // DesafioResponse (só na edição)
var categoria = 'geral';
var instrutores = [];       // vinculados (GET /academia/instrutores)
var enderecoAcademia = null;
var endereco = null;        // Endereco.montar(...)
var divisao = [];           // pontos-base por posição (10000 = 100%)
var bloqueios = { entrada: false, pagamento: false, premiacao: false, premioMin: 0 };

var LIMITES = { resumo: [20, 280], regras: [50, 10000] };

function radio(nome) { var r = document.querySelector('input[name="' + nome + '"]:checked'); return r ? r.value : ''; }
function marcarRadio(nome, valor) {
    var r = document.querySelector('input[name="' + nome + '"][value="' + valor + '"]');
    if (r) r.checked = true;
}
function inteiro(el) { var n = parseInt(el.value, 10); return isNaN(n) ? null : n; }
function temFixo(modo) { return modo === 'fixo' || modo === 'fixo_percentual'; }
function temPct(modo) { return modo === 'percentual' || modo === 'fixo_percentual'; }
// Tamanho em caracteres (runas), como o backend conta: emoji = 1
function tamanho(s) { return Array.from(s || '').length; }
function indicadosMarcados() {
    return Array.prototype.map.call(document.querySelectorAll('#indicados input:checked'), function(c) { return parseInt(c.value, 10); });
}

// ── Estado do formulário ──────────────────────────────────────────────────
function ler() {
    return {
        titulo: $('titulo').value.trim(), resumo: $('resumo').value.trim(), regras: $('regras').value.trim(), categoria: categoria,
        data: $('data').value, hora: $('hora').value, dataFim: $('dataFim').value, horaFim: $('horaFim').value,
        endereco: endereco ? endereco.valores() : null,
        vagas: inteiro($('vagas')), posicoes: inteiro($('posicoes')), divisao: divisao.slice(),
        nivelMin: parseInt($('nivelMin').value, 10) || 0, nivelMax: parseInt($('nivelMax').value, 10) || 0,
        idadeMin: inteiro($('idadeMin')), idadeMax: inteiro($('idadeMax')),
        entrada: Mask.centavos($('entrada')), premioFixo: Mask.centavos($('premioFixo')),
        dividir: $('dividir').checked, pctPote: inteiro($('pctPote')),
        instrVagas: inteiro($('instrutoresVagas')), indicados: modoEdicao ? [] : indicadosMarcados(),
        responsavel: parseInt($('responsavel').value, 10) || 0,
        modo: radio('modo'), fixo: Mask.centavos($('fixo')), pct: inteiro($('pctInstrutor')) || 0,
        simN: inteiro($('simN'))
    };
}

// Mesma conta do backend (services/financeiro.go, rodada 5: valores POR instrutor) para a prévia; o
// servidor sempre revalida.
function calcular(v) {
    var nInstr = Math.max(1, v.instrVagas || 1);
    var pctInstr = temPct(v.modo) ? v.pct : 0;
    var pctEquipe = pctInstr * nInstr;
    var maxPote = Math.max(0, 100 - taxaPct - pctEquipe);
    var pctPote = v.dividir ? (v.pctPote === null ? maxPote : v.pctPote) : 0;
    var r = { maxPote: maxPote, pctPote: pctPote, pctInstr: pctInstr, nInstr: nInstr };
    if (pctPote + taxaPct + pctEquipe > 100) {
        r.erro = 'A soma passa de 100%: pote ' + pctPote + '% + taxa da plataforma ' + taxaPct + '% + instrutores ' + nInstr + ' × ' + pctInstr + '% = ' +
            (pctPote + taxaPct + pctEquipe) + '%. ' + (maxPote > 0 ? 'Com esses instrutores, o máximo para o pote é ' + maxPote + '%.' : 'Diminua o percentual dos instrutores.');
        return r;
    }
    r.fixoInstr = temFixo(v.modo) ? v.fixo : 0;
    var vagas = v.vagas || 0;
    r.n = v.simN === null ? vagas : Math.max(0, Math.min(v.simN, vagas));
    r.arrecadacao = v.entrada * r.n;
    r.pote = Math.floor(r.arrecadacao * pctPote / 100);
    r.premioTotal = v.premioFixo + r.pote;
    r.taxa = Math.floor(r.arrecadacao * taxaPct / 100);
    r.pctPorInstr = Math.floor(r.arrecadacao * pctInstr / 100);
    r.porInstr = r.fixoInstr + r.pctPorInstr;
    r.instrTotal = nInstr * r.porInstr;
    r.reserva = v.premioFixo + nInstr * r.fixoInstr;
    r.liquido = r.arrecadacao + r.reserva - r.premioTotal - r.instrTotal - r.taxa;
    r.resultado = r.liquido - r.reserva;
    r.premios = Gestao.dividirPremio(r.premioTotal, v.divisao.length ? v.divisao : (v.posicoes || 1), v.posicoes || 1);
    return r;
}

// Reserva já debitada (edição) para mostrar só a diferença
function reservaAtual() { return original && original.financeiro ? original.financeiro.reserva_centavos : 0; }

// ── Divisão do prêmio por posição ─────────────────────────────────────────
// "33,33" → 3333 (pontos-base); null se inválido
function paraBP(texto) {
    var s = String(texto || '').trim().replace(',', '.');
    if (!/^\d{1,3}(\.\d{1,2})?$/.test(s)) return null;
    return Math.round(parseFloat(s) * 100);
}
function deBP(bp) { return (bp / 100).toFixed(2).replace('.', ',').replace(/,00$/, ''); }

function desenharDivisao() {
    var box = limpar($('divisao'));
    var travada = bloqueios.premiacao;
    divisao.forEach(function(bp, i) {
        var id = 'div' + i;
        var campo = h('input', { type: 'text', id: id, class: 'gs-input pct', inputmode: 'decimal', autocomplete: 'off', maxlength: '6',
            value: deBP(bp), disabled: travada, 'data-pos': String(i) });
        campo.addEventListener('input', function() {
            var n = paraBP(campo.value);
            divisao[i] = n === null ? 0 : n;
            erroCampo(campo, '');
        });
        box.appendChild(h('div', { class: 'gx-divisao-linha' }, [
            h('label', { class: 'gx-divisao-pos', for: id }, [h('span', { 'aria-hidden': 'true', text: Gestao.medalha(i + 1) + ' ' }), (i + 1) + 'º lugar']),
            h('div', { class: 'gx-prefixo gx-divisao-campo' }, [campo, h('span', { class: 'fim', 'aria-hidden': 'true', text: '%' })]),
            h('span', { class: 'gx-divisao-valor', id: 'divValor' + i, 'aria-live': 'off' })
        ]));
    });
}

// Problemas da divisão (para a soma e para a validação). Devolve { msg, pos } ou null.
function problemaDivisao(lista, n) {
    if (lista.length !== n) return { msg: 'A divisão precisa ter uma linha por premiado.' };
    var soma = 0;
    for (var i = 0; i < lista.length; i++) {
        if (!(lista[i] > 0)) return { msg: 'A ' + (i + 1) + 'ª posição precisa receber mais que 0%.', pos: i };
        if (i > 0 && lista[i] > lista[i - 1]) return { msg: 'A ' + (i + 1) + 'ª posição não pode receber mais que a ' + i + 'ª.', pos: i };
        soma += lista[i];
    }
    if (soma !== 10000) return { msg: 'A soma está em ' + Gestao.pctBP(soma) + ': precisa dar exatamente 100%.' + (soma < 10000 ? ' Faltam ' : ' Sobram ') + Gestao.pctBP(Math.abs(10000 - soma)) + '.' };
    return null;
}

function atualizarDivisao(calc) {
    var n = divisao.length;
    var p = problemaDivisao(divisao, n);
    var soma = divisao.reduce(function(a, b) { return a + (b || 0); }, 0);
    var el = $('somaDivisao');
    el.textContent = p ? p.msg : 'Soma: 100% ✓';
    el.className = 'gx-divisao-soma ' + (p ? 'erro' : 'ok');
    // Prévia em R$ sobre o prêmio total do simulador (todas as posições ocupadas)
    var premios = !calc.erro && !p ? Gestao.dividirPremio(calc.premioTotal, divisao, n) : [];
    for (var i = 0; i < n; i++) {
        var alvo = $('divValor' + i);
        if (alvo) alvo.textContent = premios.length ? fmtBRL(premios[i]) : '—';
    }
    return soma;
}

// Sugestão do servidor (GET /desafios/divisao-padrao); sem rede, a mesma conta local
var cacheSugestao = {};
async function sugestao(n) {
    if (cacheSugestao[n]) return cacheSugestao[n].slice();
    try {
        var d = (await API.divisaoPadrao(n)).data || {};
        if (Array.isArray(d.divisao_premio_bp) && d.divisao_premio_bp.length === n) {
            cacheSugestao[n] = d.divisao_premio_bp;
            return d.divisao_premio_bp.slice();
        }
    } catch (e) { /* cai na conta local */ }
    return Gestao.divisaoPadraoBP(n);
}
var seqDivisao = 0;
async function aplicarSugestao(n) {
    var minha = ++seqDivisao;
    var lista = await sugestao(n);
    if (minha !== seqDivisao) return; // o número de premiados mudou de novo
    divisao = lista;
    desenharDivisao();
    atualizar();
}

// Número de premiados mudou: nova divisão sugerida (a personalizada não serve para outro tamanho)
function aoMudarPosicoes() {
    var n = inteiro($('posicoes'));
    var vagas = inteiro($('vagas')) || 0;
    if (n === null || n < 1 || n > 100 || (vagas && n > vagas)) return; // a validação mostra o erro
    if (n === divisao.length) return;
    aplicarSugestao(n);
}

// ── Atualização da tela ───────────────────────────────────────────────────
function contador(el, saida, lim) {
    var n = tamanho(el.value.trim());
    saida.textContent = n + '/' + lim[1] + (n < lim[0] ? ' · mínimo ' + lim[0] + ' caracteres' : '');
    saida.classList.toggle('erro', n > 0 && n < lim[0]);
}

function atualizar() {
    var v = ler();
    contador($('resumo'), $('contResumo'), LIMITES.resumo);
    contador($('regras'), $('contRegras'), LIMITES.regras);

    $('campoFixo').hidden = !temFixo(v.modo);
    $('campoPct').hidden = !temPct(v.modo);
    $('blocoPote').hidden = !v.dividir;
    $('ajudaPagamento').textContent = bloqueios.pagamento
        ? 'Há instrutores que já aceitaram estas condições (candidatura pendente ou aceita): o pagamento não pode mais mudar.'
        : 'Os valores são de CADA instrutor. O fixo é reservado do seu saldo agora (× número de instrutores); o percentual é pago no encerramento.';

    var calc = calcular(v);
    $('ajudaPote').textContent = 'Em branco = máximo permitido (' + calc.maxPote + '%: 100% menos a taxa de ' + taxaPct + '% e o percentual dos instrutores). ' +
        'A taxa da plataforma sai da sua parte, nunca do pote dos atletas.';
    $('pctPote').max = String(calc.maxPote);
    $('pctPote').placeholder = String(calc.maxPote);
    $('ajudaEntrada').textContent = bloqueios.entrada
        ? 'Já há atletas inscritos: o valor da inscrição não pode mais ser alterado.'
        : 'Em branco ou 0,00 = desafio gratuito. Máximo R$ 10.000,00.';
    $('ajudaPremioFixo').textContent = bloqueios.premioMin
        ? 'Já há atletas inscritos: o prêmio só pode aumentar (hoje ' + fmtBRL(bloqueios.premioMin) + ').'
        : 'Sai do seu saldo na criação (reserva) e é pago ao pódio no encerramento.';
    $('vagas').min = modoEdicao && original ? String(Math.max(2, original.participantes_count)) : '2';
    $('posicoes').max = String(Math.min(100, Math.max(1, v.vagas || 1)));
    $('ajudaPosicoes').textContent = 'De 1 até ' + Math.min(100, Math.max(1, v.vagas || 1)) + ' (o número de vagas, no máximo 100).';
    if ($('simN').value === '') $('simN').placeholder = String(v.vagas || 0);
    $('simN').max = String(v.vagas || 0);

    if (!modoEdicao) atualizarIndicados(v);
    atualizarDivisao(calc);
    desenharSimulador(v, calc);
    if (!modoEdicao) guardarRascunho(v);
}

// Indicados: no máximo o nº de instrutores; responsável entre os marcados
function atualizarIndicados(v) {
    var marcados = v.indicados;
    var livres = Math.max(0, (v.instrVagas || 1) - marcados.length);
    var ajuda = $('ajudaIndicados');
    limpar(ajuda);
    if (!instrutores.length) {
        ajuda.appendChild(document.createTextNode('Você ainda não tem instrutor vinculado. A vaga fica aberta para candidaturas, ou '));
        ajuda.appendChild(h('a', { class: 'gs-link', href: '/academia/instrutores' }, 'vincule um instrutor'));
        ajuda.appendChild(document.createTextNode('.'));
    } else if (marcados.length > (v.instrVagas || 1)) {
        ajuda.textContent = 'Você marcou ' + marcados.length + ' instrutores para ' + (v.instrVagas || 1) + (v.instrVagas === 1 ? ' vaga' : ' vagas') + '. Aumente o número de instrutores ou desmarque alguém.';
    } else {
        ajuda.textContent = marcados.length + (marcados.length === 1 ? ' indicado entra' : ' indicados entram') + ' direto na equipe. ' +
            (livres ? livres + (livres === 1 ? ' vaga fica aberta' : ' vagas ficam abertas') + ' para candidatura de instrutores aprovados.' : 'A equipe já fica completa.');
    }
    // Responsável: só entre os indicados (sem indicados, o primeiro aceito vira o responsável)
    var sel = $('responsavel');
    var atual = parseInt(sel.value, 10) || v.responsavel || 0;
    $('campoResponsavel').hidden = !marcados.length;
    var ids = Array.prototype.map.call(sel.options, function(o) { return parseInt(o.value, 10); });
    if (ids.join() !== marcados.join()) {
        limpar(sel);
        marcados.forEach(function(id) {
            var i = instrutores.filter(function(x) { return x.id === id; })[0];
            sel.appendChild(h('option', { value: String(id), text: i ? nomeCompleto(i) : 'Instrutor #' + id }));
        });
        if (marcados.indexOf(atual) >= 0) sel.value = String(atual);
    }
}

function desenharSimulador(v, c) {
    var box = limpar($('simResultado'));
    if (c.erro) {
        box.appendChild(h('p', { class: 'gs-msg error show gx-recuo', role: 'alert', text: c.erro }));
        return;
    }
    var nI = c.nInstr;
    box.appendChild(Gestao.lista([
        ['Arrecadação com ' + c.n + (c.n === 1 ? ' inscrito' : ' inscritos'), fmtBRL(c.arrecadacao)],
        v.premioFixo ? ['Prêmio fixo', fmtBRL(v.premioFixo)] : null,
        v.dividir ? ['Pote dos atletas (' + c.pctPote + '%)', fmtBRL(c.pote)] : null,
        ['Prêmio total dos atletas', fmtBRL(c.premioTotal), 'total'],
        c.premios.length ? ['Por posição', c.premios.map(function(p, i) { return (i + 1) + 'º ' + fmtBRL(p); }).join(' · ')] : null,
        ['Taxa da plataforma (' + taxaPct + '%)', '−' + fmtBRL(c.taxa)],
        ['Cada instrutor recebe', fmtBRL(c.porInstr)],
        ['Instrutores (' + nI + ' × ' + fmtBRL(c.porInstr) + ')', '−' + fmtBRL(c.instrTotal)],
        ['Você recebe de volta', fmtBRL(c.liquido), 'total'],
        ['Resultado para a academia', fmtBRL(c.resultado), c.resultado < 0 ? 'neg' : 'pos']
    ], ''));

    var diff = c.reserva - reservaAtual();
    var falta = saldo !== null && diff > saldo ? diff - saldo : 0;
    var composicao = 'prêmio fixo ' + fmtBRL(v.premioFixo) + ' + ' + nI + ' × ' + fmtBRL(c.fixoInstr) + ' dos instrutores';
    var texto = modoEdicao
        ? 'Reserva: ' + fmtBRL(reservaAtual()) + ' → ' + fmtBRL(c.reserva) + (diff > 0 ? ' (debita ' + fmtBRL(diff) + ' do saldo)' : diff < 0 ? ' (devolve ' + fmtBRL(-diff) + ' ao saldo)' : ' (sem mudança)')
        : 'Será reservado do seu saldo agora: ' + fmtBRL(c.reserva);
    box.appendChild(h('div', { class: 'gx-reserva' + (falta ? ' falta' : '') }, [
        h('div', { text: texto + '.' }),
        h('div', { class: 'gx-texto', text: 'Reserva = ' + composicao + '. A parte fixa das vagas de instrutor não preenchidas volta para você no encerramento.' }),
        saldo !== null ? h('div', { class: 'gx-texto', text: 'Seu saldo: ' + fmtBRL(saldo) + (falta ? ' · faltam ' + fmtBRL(falta) : '') }) : null
    ]));
    mostrarSaldo(falta);
}

function mostrarSaldo(falta) {
    var el = $('saldoAviso');
    limpar(el);
    el.hidden = !falta;
    if (!falta) return;
    var volta = '/academia/criar-desafio' + (modoEdicao ? '?id=' + encodeURIComponent(idEdicao) : '');
    el.appendChild(h('div', { class: 'gx-card destaque', role: 'alert' }, [
        h('p', { class: 'gx-texto', text: 'Saldo insuficiente para a reserva: faltam ' + fmtBRL(falta) + '. Deposite e volte; o que você preencheu fica salvo.' }),
        h('div', { class: 'gx-acoes' }, h('a', { class: 'gx-btn', href: '/depositar?voltar=' + encodeURIComponent(volta) }, 'Depositar agora'))
    ]));
}

// ── Rascunho (criação): sobrevive à ida ao depósito ───────────────────────
function guardarRascunho(v) {
    try { sessionStorage.setItem(CHAVE_RASCUNHO, JSON.stringify(v)); } catch (e) { /* storage cheio ou bloqueado */ }
}
function lerRascunho() {
    try { return JSON.parse(sessionStorage.getItem(CHAVE_RASCUNHO)); } catch (e) { return null; }
}
function valorOuVazio(n) { return n === null || n === undefined ? '' : String(n); }
function preencher(v) {
    $('titulo').value = v.titulo || ''; $('resumo').value = v.resumo || ''; $('regras').value = v.regras || '';
    $('data').value = v.data || ''; $('hora').value = v.hora || '';
    $('dataFim').value = v.dataFim || ''; $('horaFim').value = v.horaFim || '';
    if (v.endereco && endereco) endereco.preencher(v.endereco);
    if (v.vagas) $('vagas').value = v.vagas;
    if (v.posicoes) $('posicoes').value = String(v.posicoes);
    divisao = Array.isArray(v.divisao) && v.divisao.length === (v.posicoes || 0) ? v.divisao.slice() : Gestao.divisaoPadraoBP(v.posicoes || 3);
    $('nivelMin').value = String(v.nivelMin || 0); $('nivelMax').value = String(v.nivelMax || 0);
    $('idadeMin').value = valorOuVazio(v.idadeMin); $('idadeMax').value = valorOuVazio(v.idadeMax);
    Mask.definirCentavos($('entrada'), v.entrada || 0);
    Mask.definirCentavos($('premioFixo'), v.premioFixo || 0);
    $('dividir').checked = v.dividir !== false;
    $('pctPote').value = valorOuVazio(v.pctPote);
    $('instrutoresVagas').value = String(v.instrVagas || 1);
    (v.indicados || []).forEach(function(id) {
        var c = document.querySelector('#indicados input[value="' + id + '"]');
        if (c) c.checked = true;
    });
    if (v.responsavel) $('responsavel').dataset.inicial = String(v.responsavel);
    marcarRadio('modo', v.modo || 'fixo');
    Mask.definirCentavos($('fixo'), v.fixo || 0);
    $('pctInstrutor').value = v.pct || '';
    $('simN').value = valorOuVazio(v.simN);
    selecionarCategoria(v.categoria || 'geral');
    desenharDivisao();
}

// ── Categorias ────────────────────────────────────────────────────────────
var ROTULOS_CAT = { geral: 'Geral', corrida: 'Corrida', musculacao: 'Musculação', crossfit: 'CrossFit', funcional: 'Funcional',
    ciclismo: 'Ciclismo', natacao: 'Natação', calistenia: 'Calistenia', luta: 'Luta', outros: 'Outros' };
function selecionarCategoria(c) {
    categoria = c;
    Array.prototype.forEach.call($('categorias').children, function(b) {
        var ativo = b.dataset.cat === c;
        b.classList.toggle('ativo', ativo);
        b.setAttribute('aria-pressed', ativo ? 'true' : 'false');
    });
}
function desenharCategorias(lista) {
    var box = limpar($('categorias'));
    lista.forEach(function(c) {
        box.appendChild(h('button', { type: 'button', class: 'chip-categoria', 'data-cat': c, 'aria-pressed': 'false',
            onClick: function() { selecionarCategoria(c); atualizar(); }, text: ROTULOS_CAT[c] || c }));
    });
    selecionarCategoria(categoria);
}

// ── Filtros de nível ──────────────────────────────────────────────────────
function desenharNiveis() {
    ['nivelMin', 'nivelMax'].forEach(function(id) {
        var sel = limpar($(id));
        sel.appendChild(h('option', { value: '0', text: 'Sem limite' }));
        for (var n = 1; n <= 10; n++) sel.appendChild(h('option', { value: String(n), text: 'Nível ' + n }));
    });
}

// ── Instrutores vinculados (indicados na criação) ─────────────────────────
function desenharIndicados() {
    var box = limpar($('indicados'));
    instrutores.forEach(function(i) {
        box.appendChild(h('label', { class: 'gx-opcao' }, [
            h('input', { type: 'checkbox', value: String(i.id), name: 'indicado' }),
            h('div', null, [h('strong', { text: nomeCompleto(i) }),
                h('span', { text: [i.cref ? 'CREF ' + i.cref : '', i.especialidade || '', i.desafios_ativos ? i.desafios_ativos + (i.desafios_ativos === 1 ? ' desafio ativo' : ' desafios ativos') : '']
                    .filter(Boolean).join(' · ') })])
        ]));
    });
}

// Edição: a equipe atual (indicar/remover/responsável ficam na tela de equipe)
function desenharEquipeAtual(d) {
    var box = limpar($('blocoEquipeAtual'));
    box.hidden = false;
    box.appendChild(h('span', { class: 'gs-label', text: 'Equipe atual' }));
    box.appendChild(Gestao.equipe(d));
    var pend = d.financeiro ? d.financeiro.candidaturas_pendentes : 0;
    box.appendChild(h('div', { class: 'gx-acoes' }, h('a', { class: 'gx-btn pequeno contorno', href: '/academia/candidatos?id=' + d.id },
        'Gerenciar equipe' + (pend ? ' (' + pend + (pend === 1 ? ' candidato' : ' candidatos') + ')' : ''))));
}

// ── Datas ─────────────────────────────────────────────────────────────────
function doisDigitos(n) { return (n < 10 ? '0' : '') + n; }
function paraCampos(raw) {
    var d = raw ? new Date(raw) : null;
    if (!d || isNaN(d.getTime())) return { data: '', hora: '' };
    return { data: d.getFullYear() + '-' + doisDigitos(d.getMonth() + 1) + '-' + doisDigitos(d.getDate()), hora: doisDigitos(d.getHours()) + ':' + doisDigitos(d.getMinutes()) };
}
function quando(data, hora) {
    if (!data || !hora) return null;
    var d = new Date(data + 'T' + hora + ':00');
    return isNaN(d.getTime()) ? null : d;
}
function hojeISO() {
    var d = new Date();
    return d.getFullYear() + '-' + doisDigitos(d.getMonth() + 1) + '-' + doisDigitos(d.getDate());
}
// Início mudou: o término acompanha a data (se vazio ou se estava no mesmo dia do início antigo)
var dataInicioAnterior = '';
function aoMudarInicio() {
    var d = $('data').value;
    if (d && (!$('dataFim').value || $('dataFim').value === dataInicioAnterior)) $('dataFim').value = d;
    if ($('hora').value && !$('horaFim').value) {
        var p = $('hora').value.split(':');
        var h2 = (parseInt(p[0], 10) + 2);
        if (h2 < 24) $('horaFim').value = doisDigitos(h2) + ':' + p[1]; // sugestão: 2 h de evento
    }
    $('dataFim').min = d || hojeISO();
    dataInicioAnterior = d;
}

// ── Validação ─────────────────────────────────────────────────────────────
function validar(v, calc) {
    var primeiro = null;
    function falha(el, msg) { erroCampo(el, msg); if (msg && !primeiro) primeiro = el; }
    falha($('titulo'), v.titulo.length < 3 ? 'Informe um título com pelo menos 3 caracteres.' : '');
    var nr = tamanho(v.resumo), ng = tamanho(v.regras);
    falha($('resumo'), nr < 20 ? 'Escreva um resumo com pelo menos 20 caracteres (agora: ' + nr + ').' : nr > 280 ? 'O resumo tem no máximo 280 caracteres.' : '');
    falha($('regras'), ng < 50 ? 'Descreva as regras com pelo menos 50 caracteres (agora: ' + ng + ').' : ng > 10000 ? 'As regras têm no máximo 10.000 caracteres.' : '');

    var ini = quando(v.data, v.hora), fim = quando(v.dataFim, v.horaFim);
    var inicioMudou = !original || !original.data_evento || !ini || Math.abs(ini.getTime() - new Date(original.data_evento).getTime()) >= 60000;
    falha($('data'), !v.data ? 'Informe a data de início.' : (ini && inicioMudou && ini.getTime() <= Date.now() ? 'O início precisa estar no futuro.' : ''));
    falha($('hora'), !v.hora ? 'Informe a hora de início.' : '');
    falha($('dataFim'), !v.dataFim ? 'Informe a data de término.' : '');
    falha($('horaFim'), !v.horaFim ? 'Informe a hora de término.' : (ini && fim && fim.getTime() <= ini.getTime() ? 'O término precisa ser depois do início.' : ''));

    // validar() marca e foca o 1º campo inválido do endereço
    if (endereco && !endereco.validar() && !primeiro) primeiro = document.activeElement;

    falha($('vagas'), v.vagas === null || v.vagas < 2 || v.vagas > 500 ? 'Use de 2 a 500 vagas.' :
        (original && v.vagas < original.participantes_count ? 'Já há ' + original.participantes_count + ' inscritos: não dá para ter menos vagas.' : ''));

    falha($('nivelMax'), v.nivelMin && v.nivelMax && v.nivelMin > v.nivelMax ? 'O nível máximo não pode ser menor que o mínimo.' : '');
    falha($('idadeMin'), v.idadeMin !== null && (v.idadeMin < 0 || v.idadeMin > 120) ? 'Use uma idade de 0 a 120.' : '');
    falha($('idadeMax'), v.idadeMax !== null && (v.idadeMax < 0 || v.idadeMax > 120) ? 'Use uma idade de 0 a 120.' :
        (v.idadeMin && v.idadeMax && v.idadeMin > v.idadeMax ? 'A idade máxima não pode ser menor que a mínima.' : ''));

    var maxPos = Math.min(100, v.vagas || 100);
    falha($('posicoes'), v.posicoes === null || v.posicoes < 1 || v.posicoes > 100 ? 'Use de 1 a 100 premiados.' :
        (v.vagas !== null && v.posicoes > v.vagas ? 'Não dá para premiar mais atletas (' + v.posicoes + ') que as vagas (' + v.vagas + ').' : ''));
    if (!bloqueios.premiacao && v.posicoes >= 1 && v.posicoes <= maxPos) {
        var p = problemaDivisao(v.divisao, v.posicoes);
        if (p) {
            var campo = $('div' + (p.pos === undefined ? 0 : p.pos)) || $('posicoes');
            falha(campo, p.msg);
        }
    }

    falha($('pctPote'), v.dividir && v.pctPote !== null && (v.pctPote < 0 || v.pctPote > 100) ? 'Use um percentual de 0 a 100.' : '');
    var minEquipe = original ? (original.equipe || []).length : 0;
    falha($('instrutoresVagas'), v.instrVagas === null || v.instrVagas < 1 || v.instrVagas > 20 ? 'Use de 1 a 20 instrutores.' :
        (minEquipe && v.instrVagas < minEquipe ? 'A equipe já tem ' + minEquipe + ' instrutores: não dá para ter menos vagas. Remova alguém na tela de equipe.' :
        (!modoEdicao && v.indicados.length > v.instrVagas ? 'Há mais indicados (' + v.indicados.length + ') que instrutores no evento.' : '')));
    falha($('pctInstrutor'), temPct(v.modo) && (v.pct < 0 || v.pct > 100) ? 'Use um percentual de 0 a 100.' : '');
    if (bloqueios.premioMin) {
        var fo = original.financeiro || {};
        falha($('premioFixo'), v.premioFixo < (fo.premio_fixo_centavos || 0) ? 'Já há atletas inscritos: o prêmio só pode aumentar.' : '');
        if (fo.dividir_pote && (!v.dividir || calc.pctPote < fo.percentual_pote)) {
            falha($('pctPote'), 'Já há atletas inscritos: o percentual do pote só pode aumentar (hoje ' + fo.percentual_pote + '%).');
        }
    }
    if (calc.erro) falha(temPct(v.modo) ? $('pctInstrutor') : $('pctPote'), calc.erro);
    if (primeiro) { primeiro.focus(); return false; }
    return true;
}

// ── Payload ───────────────────────────────────────────────────────────────
function filtro(n) { return n && n > 0 ? n : 0; }
function montarCriacao(v, calc) {
    var p = Object.assign({
        titulo: v.titulo, resumo: v.resumo, regras: v.regras, categoria: v.categoria,
        data_evento: quando(v.data, v.hora).toISOString(), data_fim_evento: quando(v.dataFim, v.horaFim).toISOString(),
        vagas: v.vagas, valor_entrada_centavos: v.entrada,
        posicoes_podio: v.posicoes, divisao_premio_bp: v.divisao,
        premio_fixo_centavos: v.premioFixo, dividir_pote: v.dividir,
        instrutores_vagas: v.instrVagas, indicados: v.indicados, instrutor_modo_pagamento: v.modo
    }, v.endereco);
    if (filtro(v.nivelMin)) p.nivel_min = v.nivelMin;
    if (filtro(v.nivelMax)) p.nivel_max = v.nivelMax;
    if (filtro(v.idadeMin)) p.idade_min = v.idadeMin;
    if (filtro(v.idadeMax)) p.idade_max = v.idadeMax;
    if (v.indicados.length && v.indicados.indexOf(v.responsavel) >= 0) p.id_responsavel = v.responsavel;
    if (v.dividir && v.pctPote !== null) p.percentual_pote = v.pctPote;
    if (temFixo(v.modo)) p.instrutor_fixo_centavos = v.fixo;
    if (temPct(v.modo)) p.instrutor_pct = v.pct;
    return p;
}

// Edição: manda só o que mudou (o servidor recusa alguns campos conforme o estado do desafio)
var CAMPOS_END = ['cep', 'logradouro', 'numero', 'complemento', 'bairro', 'cidade', 'uf'];
function montarEdicao(v, calc) {
    var o = original, f = o.financeiro || {}, p = {};
    function se(chave, novo, antigo) { if (novo !== antigo) p[chave] = novo; }
    se('titulo', v.titulo, o.titulo);
    se('resumo', v.resumo, (o.resumo || '').trim());
    se('regras', v.regras, (o.regras || '').trim());
    se('categoria', v.categoria, o.categoria);
    // Endereço: só inteiro (o servidor exige todos os campos se algum vier)
    var oe = o.endereco || {};
    if (!o.endereco || CAMPOS_END.some(function(k) { return (v.endereco[k] || '') !== (oe[k] || ''); })) Object.assign(p, v.endereco);
    var ini = quando(v.data, v.hora), fim = quando(v.dataFim, v.horaFim);
    if (!o.data_evento || Math.abs(ini.getTime() - new Date(o.data_evento).getTime()) >= 60000) p.data_evento = ini.toISOString();
    if (!o.data_fim_evento || Math.abs(fim.getTime() - new Date(o.data_fim_evento).getTime()) >= 60000) p.data_fim_evento = fim.toISOString();
    se('vagas', v.vagas, o.vagas);
    if (!bloqueios.premiacao) {
        se('posicoes_podio', v.posicoes, o.posicoes_podio);
        if (p.posicoes_podio !== undefined || v.divisao.join() !== (o.divisao_premio_bp || []).join()) p.divisao_premio_bp = v.divisao;
        se('nivel_min', filtro(v.nivelMin), o.nivel_min || 0);
        se('nivel_max', filtro(v.nivelMax), o.nivel_max || 0);
        se('idade_min', filtro(v.idadeMin), o.idade_min || 0);
        se('idade_max', filtro(v.idadeMax), o.idade_max || 0);
    }
    if (!bloqueios.entrada) se('valor_entrada_centavos', v.entrada, o.valor_entrada_centavos);
    se('premio_fixo_centavos', v.premioFixo, f.premio_fixo_centavos || 0);
    se('dividir_pote', v.dividir, !!f.dividir_pote);
    if (v.dividir) {
        // Em branco = máximo permitido (mesma regra do servidor)
        se('percentual_pote', v.pctPote === null ? calc.maxPote : v.pctPote, f.percentual_pote);
    }
    se('instrutores_vagas', v.instrVagas, o.instrutores_vagas || f.instrutores_vagas || 1);
    if (!bloqueios.pagamento) {
        se('instrutor_modo_pagamento', v.modo, f.instrutor_modo_pagamento);
        se('instrutor_fixo_centavos', temFixo(v.modo) ? v.fixo : 0, f.instrutor_fixo_centavos || 0);
        se('instrutor_pct', temPct(v.modo) ? v.pct : 0, f.instrutor_pct || 0);
    }
    return p;
}

// ── Envio ─────────────────────────────────────────────────────────────────
async function enviar(ev) {
    ev.preventDefault();
    var erroForm = $('erroForm');
    erroForm.classList.remove('show');
    // Número de premiados digitado e enviado sem sair do campo: sincroniza a divisão antes de validar
    var nPos = inteiro($('posicoes'));
    if (!bloqueios.premiacao && nPos >= 1 && nPos <= 100 && nPos <= (inteiro($('vagas')) || 0) && nPos !== divisao.length) {
        await aplicarSugestao(nPos);
        showToast('A divisão do prêmio foi ajustada para ' + nPos + (nPos === 1 ? ' premiado' : ' premiados') + '. Confira e envie de novo.', '');
        return;
    }
    var v = ler(), calc = calcular(v);
    if (!validar(v, calc)) { showToast('Corrija os campos destacados.', 'error'); return; }

    var diff = calc.reserva - reservaAtual();
    if (saldo !== null && diff > saldo) { mostrarSaldo(diff - saldo); $('saldoAviso').scrollIntoView({ block: 'center' }); return; }

    var payload = modoEdicao ? montarEdicao(v, calc) : montarCriacao(v, calc);
    if (modoEdicao && !Object.keys(payload).length) { showToast('Nada foi alterado.', ''); return; }

    var linhas = [];
    if (modoEdicao) {
        linhas.push(['Reserva', fmtBRL(reservaAtual()) + ' → ' + fmtBRL(calc.reserva)]);
        if (diff) linhas.push([diff > 0 ? 'Debitado agora' : 'Devolvido agora', fmtBRL(Math.abs(diff))]);
    } else {
        linhas.push(['Quando', Gestao.horario({ data_evento: quando(v.data, v.hora), data_fim_evento: quando(v.dataFim, v.horaFim) })]);
        linhas.push(['Instrutores', v.instrVagas + (v.indicados.length ? ' (' + v.indicados.length + (v.indicados.length === 1 ? ' indicado)' : ' indicados)') : ' (vagas abertas)')]);
        linhas.push(['Reservado do seu saldo', fmtBRL(calc.reserva)]);
        if (saldo !== null) linhas.push(['Saldo depois', fmtBRL(saldo - calc.reserva)]);
    }
    linhas.push(['Inscrição', v.entrada ? fmtBRL(v.entrada) : 'Gratuita']);
    var ok = await Modal.confirmar({
        titulo: modoEdicao ? 'Salvar alterações?' : 'Publicar o desafio?',
        mensagem: modoEdicao ? 'As mudanças aparecem para os atletas e instrutores na hora.'
            : 'O prêmio fixo e a parte fixa de cada instrutor ficam reservados até o encerramento. Se cancelar o desafio, a reserva volta inteira.',
        linhas: linhas, confirmar: modoEdicao ? 'Salvar' : 'Publicar'
    });
    if (!ok) return;

    var btn = $('btnSalvar');
    btnLoading(btn, true);
    try {
        var resp = modoEdicao ? await API.editarDesafio(idEdicao, payload) : await API.criarDesafio(payload);
        try { sessionStorage.removeItem(CHAVE_RASCUNHO); } catch (e) {}
        showToast(modoEdicao ? 'Desafio atualizado!' : 'Desafio publicado!', 'success');
        var id = modoEdicao ? idEdicao : (resp && resp.data && resp.data.id);
        setTimeout(function() { window.location.href = id ? '/academia/podio?id=' + encodeURIComponent(id) : '/academia/desafios'; }, 900);
    } catch (e) {
        btnLoading(btn, false);
        erroForm.textContent = mensagemErro(e);
        erroForm.classList.add('show');
        erroForm.scrollIntoView({ block: 'center' });
        // Saldo insuficiente (o saldo local pode estar defasado): oferece o depósito
        if (e.status === 422 && /saldo/i.test(e.message || '')) mostrarSaldo(Math.max(1, diff - (saldo || 0)));
    }
}

// ── Edição: carrega e trava o que o servidor recusaria ────────────────────
function aplicarEdicao(d) {
    original = d;
    var f = d.financeiro || {};
    taxaPct = f.taxa_plataforma_pct !== undefined ? f.taxa_plataforma_pct : taxaPct;
    var ini = paraCampos(d.data_evento), fim = paraCampos(d.data_fim_evento);
    preencher({
        titulo: d.titulo, resumo: d.resumo || d.descricao || '', regras: d.regras || '', categoria: d.categoria || 'geral',
        data: ini.data, hora: ini.hora, dataFim: fim.data, horaFim: fim.hora,
        endereco: d.endereco || enderecoAcademia,
        vagas: d.vagas, posicoes: d.posicoes_podio, divisao: d.divisao_premio_bp || [],
        nivelMin: d.nivel_min || 0, nivelMax: d.nivel_max || 0, idadeMin: d.idade_min, idadeMax: d.idade_max,
        entrada: d.valor_entrada_centavos, premioFixo: f.premio_fixo_centavos || 0,
        dividir: !!f.dividir_pote, pctPote: f.dividir_pote ? f.percentual_pote : null,
        instrVagas: d.instrutores_vagas || f.instrutores_vagas || 1,
        modo: f.instrutor_modo_pagamento || 'fixo', fixo: f.instrutor_fixo_centavos || 0, pct: f.instrutor_pct || 0, simN: null
    });
    dataInicioAnterior = ini.data;
    $('tituloPagina').textContent = 'EDITAR DESAFIO';
    document.title = 'Editar desafio — GymScore';
    $('subPagina').textContent = 'Altere o que precisar. A reserva do seu saldo é ajustada pela diferença.';
    $('btnSalvar').textContent = 'Salvar alterações';
    if (!d.endereco) $('ajudaLocal').textContent = 'Este desafio é anterior ao endereço estruturado: confira o endereço abaixo (veio o da academia) e salve.';
    else $('ajudaLocal').textContent = 'Para mudar o local, digite o novo CEP: o endereço é buscado sozinho.';

    // Equipe: na edição, indicar/remover/responsável ficam na tela de equipe
    $('blocoIndicados').hidden = true;
    desenharEquipeAtual(d);
    $('instrutoresVagas').min = String(Math.max(1, (d.equipe || []).length));

    if (d.participantes_count > 0) {
        bloqueios.entrada = true;
        $('entrada').disabled = true;
        bloqueios.premioMin = (f.premio_fixo_centavos || 0) + (f.pote_premio_centavos || 0);
    }
    // Divisão, nº de premiados e filtros só mudam antes da 1ª inscrição (U1/U4)
    if (!d.pode_editar_premiacao) {
        bloqueios.premiacao = true;
        ['posicoes', 'nivelMin', 'nivelMax', 'idadeMin', 'idadeMax'].forEach(function(id) { $(id).disabled = true; });
        $('btnSugestao').hidden = true;
        var nota = $('notaPremiacao');
        nota.hidden = false;
        nota.textContent = 'Já há atletas inscritos: o número de premiados, a divisão do prêmio e os requisitos de nível/idade não podem mais mudar.';
        $('ajudaRequisitos').textContent = 'Já há atletas inscritos: os requisitos não podem mais mudar.';
        desenharDivisao();
    }
    // Pagamento travado: alguém já aceitou estas condições (candidatura pendente ou membro por candidatura)
    var porCandidatura = (d.equipe || []).some(function(m) { return m.origem === 'candidatura'; });
    if ((f.candidaturas_pendentes || 0) > 0 || porCandidatura) {
        bloqueios.pagamento = true;
        ['fixo', 'pctInstrutor'].forEach(function(id) { $(id).disabled = true; });
        Array.prototype.forEach.call(document.querySelectorAll('input[name="modo"]'), function(r) { r.disabled = true; });
    }
}

// ── Início ────────────────────────────────────────────────────────────────
async function iniciar() {
    var carregando = $('carregando');
    Skeleton.cards(carregando, 3, 110);
    try {
        var reqs = [API.taxasPlataforma().catch(function() { return null; }),
            API.carteira().catch(function() { return null; }),
            API.categoriasDesafio().catch(function() { return null; }),
            modoEdicao ? Promise.resolve(null) : API.listarInstrutoresVinculados(1, 100),
            API.academiaPerfil().catch(function() { return null; }),
            modoEdicao ? API.buscarDesafio(idEdicao) : Promise.resolve(null)];
        var res = await Promise.all(reqs);
        if (res[0] && res[0].data) taxaPct = res[0].data.taxa_inscricao_pct;
        if (res[1] && res[1].data) saldo = res[1].data.saldo_centavos;
        var cats = res[2] && res[2].data && res[2].data.length ? res[2].data : Object.keys(ROTULOS_CAT);
        instrutores = ((res[3] && res[3].data && res[3].data.itens) || []).filter(function(i) { return !i.convite_pendente; });
        var a = res[4] && res[4].data;
        if (a && a.cep) enderecoAcademia = { cep: a.cep, logradouro: a.logradouro, numero: a.numero, complemento: a.complemento, bairro: a.bairro, cidade: a.cidade, uf: a.uf };
        var d = res[5] && res[5].data;

        if (modoEdicao && !d.pode_editar) {
            Skeleton.fim(carregando);
            limpar(carregando).appendChild(estadoVazio({
                icone: '🔒', titulo: 'Este desafio não pode mais ser editado',
                texto: 'Só desafios abertos podem ser editados.', acao: { texto: 'Voltar aos meus desafios', href: '/academia/desafios' }
            }));
            return;
        }

        desenharCategorias(cats);
        desenharNiveis();
        desenharIndicados();
        endereco = Endereco.montar($('enderecoCampos'), { prefixo: 'local' });
        if (enderecoAcademia) $('btnEndAcademia').hidden = false;
        $('data').min = hojeISO();
        $('dataFim').min = hojeISO();
        Mask.auto($('form'));

        if (modoEdicao) aplicarEdicao(d);
        else {
            var r = lerRascunho();
            preencher(r || { posicoes: 3, instrVagas: 1, endereco: enderecoAcademia });
            if (r && !r.endereco && enderecoAcademia) endereco.preencher(enderecoAcademia);
            if (r && r.responsavel) $('responsavel').dataset.inicial = String(r.responsavel);
            dataInicioAnterior = $('data').value;
            if (!r) aplicarSugestao(3); // confirma a sugestão com o servidor
        }

        Skeleton.fim(carregando);
        limpar(carregando);
        $('form').hidden = false;
        atualizar();
        var inicial = parseInt($('responsavel').dataset.inicial, 10);
        if (inicial && document.querySelector('#responsavel option[value="' + inicial + '"]')) { $('responsavel').value = String(inicial); atualizar(); }
    } catch (e) {
        Gestao.erro(carregando, e, iniciar);
    }
}

// Ao corrigir um campo, o erro dele some (a validação completa roda de novo no envio)
$('form').addEventListener('input', function(ev) {
    if (ev.target && ev.target.getAttribute && ev.target.getAttribute('aria-invalid') === 'true') erroCampo(ev.target, '');
    atualizar();
});
$('form').addEventListener('change', function(ev) {
    if (ev.target === $('posicoes') || ev.target === $('vagas')) aoMudarPosicoes();
    if (ev.target === $('data') || ev.target === $('hora')) aoMudarInicio();
    atualizar();
});
$('form').addEventListener('submit', enviar);
$('btnSugestao').addEventListener('click', function() {
    var n = inteiro($('posicoes'));
    if (n && n >= 1 && n <= 100) aplicarSugestao(n);
});
$('btnEndAcademia').addEventListener('click', function() {
    if (!enderecoAcademia || !endereco) return;
    endereco.preencher(enderecoAcademia);
    CAMPOS_END.forEach(function(k) { if (endereco.campos[k]) erroCampo(endereco.campos[k], ''); });
    showToast('Endereço da academia aplicado.', 'success');
    atualizar();
});
iniciar();
renderBottomNav('/academia/criar-desafio');
})();
