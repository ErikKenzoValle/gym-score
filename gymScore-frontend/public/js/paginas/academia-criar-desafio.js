// Script da página academia-criar-desafio.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
(async function () {
var user = await Gestao.exigirPerfil('academia');
if (!user) return; // redirecionado para o login/home

var idEdicao = Gestao.param('id');
var modoEdicao = !!idEdicao;
var CHAVE_RASCUNHO = 'gx_rascunho_desafio';

var $ = function(id) { return document.getElementById(id); };
var taxaPct = 10;          // % da plataforma (GET /plataforma/taxas; na edição vale o snapshot do desafio)
var saldo = null;           // centavos; null = não foi possível ler
var original = null;        // DesafioResponse (só na edição)
var categoria = 'geral';
var instrutores = [];
var bloqueios = { entrada: false, pagamento: false, selecao: false, premioMin: 0 };

function radio(nome) { var r = document.querySelector('input[name="' + nome + '"]:checked'); return r ? r.value : ''; }
function marcarRadio(nome, valor) {
    var r = document.querySelector('input[name="' + nome + '"][value="' + valor + '"]');
    if (r) r.checked = true;
}
function inteiro(el) { var n = parseInt(el.value, 10); return isNaN(n) ? null : n; }
function temFixo(modo) { return modo === 'fixo' || modo === 'fixo_percentual'; }
function temPct(modo) { return modo === 'percentual' || modo === 'fixo_percentual'; }

// ── Estado do formulário ──────────────────────────────────────────────────
function ler() {
    return {
        titulo: $('titulo').value.trim(), descricao: $('descricao').value.trim(), categoria: categoria,
        data: $('data').value, hora: $('hora').value, local: $('local').value.trim(),
        vagas: inteiro($('vagas')), posicoes: parseInt($('posicoes').value, 10),
        entrada: Mask.centavos($('entrada')), premioFixo: Mask.centavos($('premioFixo')),
        dividir: $('dividir').checked, pctPote: inteiro($('pctPote')),
        selecao: radio('selecao'), idInstrutor: parseInt($('idInstrutor').value, 10) || 0,
        modo: radio('modo'), fixo: Mask.centavos($('fixo')), teto: Mask.centavos($('teto')), pct: inteiro($('pctInstrutor')) || 0,
        simN: inteiro($('simN'))
    };
}

// Mesma conta do backend (services/financeiro.go) para a prévia; o servidor sempre revalida.
function calcular(v) {
    var pctInstr = temPct(v.modo) ? v.pct : 0;
    var maxPote = Math.max(0, 100 - taxaPct - pctInstr);
    var pctPote = v.dividir ? (v.pctPote === null ? maxPote : v.pctPote) : 0;
    var r = { maxPote: maxPote, pctPote: pctPote, pctInstr: pctInstr };
    if (pctPote + taxaPct + pctInstr > 100) {
        r.erro = 'A soma do pote (' + pctPote + '%), da taxa da plataforma (' + taxaPct + '%) e do instrutor (' + pctInstr + '%) passa de 100%. Com a taxa de ' +
            taxaPct + '% e o instrutor em ' + pctInstr + '%, o máximo para o pote é ' + maxPote + '%.';
        return r;
    }
    var vaga = v.selecao === 'vaga_aberta';
    r.instrFixo = temFixo(v.modo) ? (vaga ? v.teto : v.fixo) : 0;
    var vagas = v.vagas || 0;
    r.n = v.simN === null ? vagas : Math.max(0, Math.min(v.simN, vagas));
    r.arrecadacao = v.entrada * r.n;
    r.pote = Math.floor(r.arrecadacao * pctPote / 100);
    r.premioTotal = v.premioFixo + r.pote;
    r.taxa = Math.floor(r.arrecadacao * taxaPct / 100);
    r.instrPctV = Math.floor(r.arrecadacao * pctInstr / 100);
    r.instrTotal = r.instrFixo + r.instrPctV;
    r.reserva = v.premioFixo + r.instrFixo;
    r.liquido = r.arrecadacao + r.reserva - r.premioTotal - r.instrTotal - r.taxa;
    r.resultado = r.liquido - r.reserva;
    r.premios = Gestao.dividirPremio(r.premioTotal, v.posicoes, Math.min(r.n, v.posicoes));
    return r;
}

// Reserva já debitada (edição) para mostrar só a diferença
function reservaAtual() { return original && original.financeiro ? original.financeiro.reserva_centavos : 0; }

// ── Atualização da tela ───────────────────────────────────────────────────
function atualizar() {
    var v = ler();
    var vaga = v.selecao === 'vaga_aberta';

    // Visibilidade dos campos conforme seleção e modo
    $('blocoDesignado').hidden = vaga;
    $('campoFixo').hidden = vaga || !temFixo(v.modo);
    $('campoTeto').hidden = !(vaga && temFixo(v.modo));
    $('campoPct').hidden = !temPct(v.modo);
    $('blocoPote').hidden = !v.dividir;
    $('rotuloFixo').textContent = 'Valor fixo (R$)';
    $('ajudaPagamento').textContent = vaga
        ? (temFixo(v.modo)
            ? 'O instrutor escolhido recebe o lance dele como parte fixa. O teto fica reservado e o que sobrar volta ao seu saldo na escolha.'
            : 'No modo percentual não há lance: os candidatos concorrem só pela vaga e você escolhe um.')
        : 'O valor fixo é reservado do seu saldo agora; o percentual é pago só no encerramento.';

    var calc = calcular(v);
    $('ajudaPote').textContent = 'Em branco = máximo permitido (' + calc.maxPote + '%: 100% menos a taxa de ' + taxaPct + '% e o percentual do instrutor). ' +
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

    // A quantidade de posições não pode passar das vagas
    Array.prototype.forEach.call($('posicoes').options, function(o) { o.disabled = v.vagas !== null && parseInt(o.value, 10) > v.vagas; });
    if ($('posicoes').options[$('posicoes').selectedIndex].disabled) {
        $('posicoes').value = String(Math.max(1, Math.min(5, v.vagas || 1)));
        v.posicoes = parseInt($('posicoes').value, 10);
        calc = calcular(v);
    }
    if ($('simN').value === '' ) $('simN').placeholder = String(v.vagas || 0);
    $('simN').max = String(v.vagas || 0);

    desenharSimulador(v, calc);
    if (!modoEdicao) guardarRascunho(v);
}

function desenharSimulador(v, c) {
    var box = limpar($('simResultado'));
    if (c.erro) {
        box.appendChild(h('p', { class: 'gs-msg error show', role: 'alert', style: { marginTop: '12px' }, text: c.erro }));
        return;
    }
    var vaga = v.selecao === 'vaga_aberta';
    var modos = { fixo: 'fixo', percentual: 'percentual', fixo_percentual: 'fixo + percentual' };
    box.appendChild(Gestao.lista([
        ['Arrecadação com ' + c.n + (c.n === 1 ? ' inscrito' : ' inscritos'), fmtBRL(c.arrecadacao)],
        v.premioFixo ? ['Prêmio fixo', fmtBRL(v.premioFixo)] : null,
        v.dividir ? ['Pote dos atletas (' + c.pctPote + '%)', fmtBRL(c.pote)] : null,
        ['Prêmio total dos atletas', fmtBRL(c.premioTotal), 'total'],
        c.premios.length > 1 || c.premios.length === 1 ? ['Por posição', c.premios.map(function(p, i) { return (i + 1) + 'º ' + fmtBRL(p); }).join(' · ')] : null,
        ['Taxa da plataforma (' + taxaPct + '%)', '−' + fmtBRL(c.taxa)],
        ['Instrutor (' + modos[v.modo] + (vaga && temFixo(v.modo) ? ', até o teto' : '') + ')', '−' + fmtBRL(c.instrTotal)],
        ['Você recebe de volta', fmtBRL(c.liquido), 'total'],
        ['Resultado para a academia', fmtBRL(c.resultado), c.resultado < 0 ? 'neg' : 'pos']
    ], ''));

    var diff = c.reserva - reservaAtual();
    var falta = saldo !== null && diff > saldo ? diff - saldo : 0;
    var texto = modoEdicao
        ? 'Reserva: ' + fmtBRL(reservaAtual()) + ' → ' + fmtBRL(c.reserva) + (diff > 0 ? ' (debita ' + fmtBRL(diff) + ' do saldo)' : diff < 0 ? ' (devolve ' + fmtBRL(-diff) + ' ao saldo)' : ' (sem mudança)')
        : 'Será reservado do seu saldo agora: ' + fmtBRL(c.reserva);
    box.appendChild(h('div', { class: 'gx-reserva' + (falta ? ' falta' : '') }, [
        h('div', null, [texto + '. ']),
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
        h('a', { class: 'gx-btn', href: '/depositar?voltar=' + encodeURIComponent(volta), style: { marginTop: '10px' } }, 'Depositar agora')
    ]));
}

// ── Rascunho (criação): sobrevive à ida ao depósito ───────────────────────
function guardarRascunho(v) {
    try { sessionStorage.setItem(CHAVE_RASCUNHO, JSON.stringify(v)); } catch (e) { /* storage cheio ou bloqueado */ }
}
function restaurarRascunho() {
    var v;
    try { v = JSON.parse(sessionStorage.getItem(CHAVE_RASCUNHO)); } catch (e) { v = null; }
    if (!v) return;
    preencher(v);
}
function preencher(v) {
    $('titulo').value = v.titulo || ''; $('descricao').value = v.descricao || '';
    $('data').value = v.data || ''; $('hora').value = v.hora || ''; $('local').value = v.local || '';
    if (v.vagas) $('vagas').value = v.vagas;
    if (v.posicoes) $('posicoes').value = String(v.posicoes);
    Mask.definirCentavos($('entrada'), v.entrada || 0);
    Mask.definirCentavos($('premioFixo'), v.premioFixo || 0);
    $('dividir').checked = v.dividir !== false;
    $('pctPote').value = v.pctPote === null || v.pctPote === undefined ? '' : v.pctPote;
    marcarRadio('selecao', v.selecao || 'designado');
    if (v.idInstrutor) $('idInstrutor').value = String(v.idInstrutor);
    marcarRadio('modo', v.modo || 'fixo');
    Mask.definirCentavos($('fixo'), v.fixo || 0);
    Mask.definirCentavos($('teto'), v.teto || 0);
    $('pctInstrutor').value = v.pct || '';
    $('simN').value = v.simN === null || v.simN === undefined ? '' : v.simN;
    selecionarCategoria(v.categoria || 'geral');
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

// ── Instrutores vinculados ────────────────────────────────────────────────
function desenharInstrutores(selecionado) {
    var sel = limpar($('idInstrutor'));
    sel.appendChild(h('option', { value: '', text: instrutores.length ? '— Escolha o instrutor —' : '— Nenhum instrutor vinculado —' }));
    instrutores.forEach(function(i) {
        sel.appendChild(h('option', { value: String(i.id), selected: i.id === selecionado,
            text: nomeCompleto(i) + (i.cref ? ' · CREF ' + i.cref : '') }));
    });
    var ajuda = $('ajudaInstrutor');
    limpar(ajuda);
    if (!instrutores.length) {
        ajuda.appendChild(document.createTextNode('Você ainda não tem instrutor vinculado. '));
        ajuda.appendChild(h('a', { class: 'gs-link', href: '/academia/instrutores' }, 'Vincular um instrutor'));
        ajuda.appendChild(document.createTextNode(' ou abra a vaga de instrutor acima.'));
    }
}

// ── Datas ─────────────────────────────────────────────────────────────────
function doisDigitos(n) { return (n < 10 ? '0' : '') + n; }
function paraCampos(raw) {
    var d = new Date(raw);
    if (isNaN(d.getTime())) return { data: '', hora: '' };
    return { data: d.getFullYear() + '-' + doisDigitos(d.getMonth() + 1) + '-' + doisDigitos(d.getDate()), hora: doisDigitos(d.getHours()) + ':' + doisDigitos(d.getMinutes()) };
}
function dataEventoISO(v) {
    var d = new Date(v.data + 'T' + v.hora + ':00');
    return isNaN(d.getTime()) ? null : d;
}

// ── Validação ─────────────────────────────────────────────────────────────
function validar(v, calc) {
    var primeiro = null;
    function falha(el, msg) { erroCampo(el, msg); if (msg && !primeiro) primeiro = el; }
    falha($('titulo'), v.titulo.length < 3 ? 'Informe um título com pelo menos 3 caracteres.' : '');
    falha($('local'), !v.local ? 'Informe o local do evento.' : '');
    var quando = v.data && v.hora ? dataEventoISO(v) : null;
    falha($('data'), !v.data ? 'Informe a data.' : (quando && quando.getTime() <= Date.now() ? 'A data do evento precisa estar no futuro.' : ''));
    falha($('hora'), !v.hora ? 'Informe a hora.' : '');
    falha($('vagas'), v.vagas === null || v.vagas < 2 || v.vagas > 500 ? 'Use de 2 a 500 vagas.' :
        (original && v.vagas < original.participantes_count ? 'Já há ' + original.participantes_count + ' inscritos: não dá para ter menos vagas.' : ''));
    falha($('posicoes'), v.vagas !== null && v.posicoes > v.vagas ? 'O pódio não pode ter mais posições que vagas.' : '');
    falha($('pctPote'), v.dividir && v.pctPote !== null && (v.pctPote < 0 || v.pctPote > 100) ? 'Use um percentual de 0 a 100.' : '');
    falha($('pctInstrutor'), temPct(v.modo) && (v.pct < 0 || v.pct > 100) ? 'Use um percentual de 0 a 100.' : '');
    if (v.selecao === 'designado' && !v.idInstrutor) falha($('idInstrutor'), 'Escolha o instrutor ou abra a vaga de instrutor.');
    else falha($('idInstrutor'), '');
    if (bloqueios.premioMin) {
        var fo = original.financeiro || {};
        falha($('premioFixo'), v.premioFixo < (fo.premio_fixo_centavos || 0) ? 'Já há atletas inscritos: o prêmio só pode aumentar.' : '');
        if (fo.dividir_pote && (!v.dividir || calc.pctPote < fo.percentual_pote)) {
            falha($('pctPote'), 'Já há atletas inscritos: o percentual do pote só pode aumentar (hoje ' + fo.percentual_pote + '%).');
        }
    }
    if (calc.erro) falha($('pctPote'), calc.erro);
    if (primeiro) { primeiro.focus(); return false; }
    return true;
}

// ── Payload ───────────────────────────────────────────────────────────────
function montarCriacao(v, calc) {
    var vaga = v.selecao === 'vaga_aberta';
    var p = {
        titulo: v.titulo, descricao: v.descricao, categoria: v.categoria, local: v.local,
        data_evento: dataEventoISO(v).toISOString(), vagas: v.vagas, valor_entrada_centavos: v.entrada, posicoes_podio: v.posicoes,
        premio_fixo_centavos: v.premioFixo, dividir_pote: v.dividir,
        instrutor_selecao: v.selecao, instrutor_modo_pagamento: v.modo
    };
    if (v.dividir && v.pctPote !== null) p.percentual_pote = v.pctPote;
    if (vaga) p.instrutor_teto_centavos = temFixo(v.modo) ? v.teto : 0;
    else {
        p.id_instrutor = v.idInstrutor;
        if (temFixo(v.modo)) p.instrutor_fixo_centavos = v.fixo;
    }
    if (temPct(v.modo)) p.instrutor_pct = v.pct;
    return p;
}

// Edição: manda só o que mudou (o servidor recusa alguns campos conforme o estado do desafio)
function montarEdicao(v, calc) {
    var o = original, f = o.financeiro || {}, p = {};
    function se(chave, novo, antigo) { if (novo !== antigo) p[chave] = novo; }
    se('titulo', v.titulo, o.titulo);
    se('descricao', v.descricao, o.descricao || '');
    se('categoria', v.categoria, o.categoria);
    se('local', v.local, o.local);
    var quando = dataEventoISO(v);
    var antes = o.data_evento ? new Date(o.data_evento) : null;
    if (!antes || Math.abs(quando.getTime() - antes.getTime()) >= 60000) p.data_evento = quando.toISOString();
    se('vagas', v.vagas, o.vagas);
    se('posicoes_podio', v.posicoes, o.posicoes_podio);
    if (!bloqueios.entrada) se('valor_entrada_centavos', v.entrada, o.valor_entrada_centavos);
    se('premio_fixo_centavos', v.premioFixo, f.premio_fixo_centavos || 0);
    se('dividir_pote', v.dividir, !!f.dividir_pote);
    if (v.dividir) {
        // Em branco = máximo permitido (mesma regra do servidor)
        se('percentual_pote', v.pctPote === null ? calc.maxPote : v.pctPote, f.percentual_pote);
    }
    if (!bloqueios.pagamento) {
        se('instrutor_modo_pagamento', v.modo, f.instrutor_modo_pagamento);
        if (v.selecao === 'designado') {
            se('instrutor_fixo_centavos', temFixo(v.modo) ? v.fixo : 0, f.instrutor_fixo_centavos || 0);
            if (v.idInstrutor && (!o.instrutor || o.instrutor.id !== v.idInstrutor)) p.id_instrutor = v.idInstrutor;
        } else {
            se('instrutor_teto_centavos', temFixo(v.modo) ? v.teto : 0, f.instrutor_teto_centavos || 0);
        }
        se('instrutor_pct', temPct(v.modo) ? v.pct : 0, f.instrutor_pct || 0);
    } else if (v.selecao === 'designado' && v.idInstrutor && (!o.instrutor || o.instrutor.id !== v.idInstrutor)) {
        p.id_instrutor = v.idInstrutor;
    }
    return p;
}

// ── Envio ─────────────────────────────────────────────────────────────────
async function enviar(ev) {
    ev.preventDefault();
    var erroForm = $('erroForm');
    erroForm.classList.remove('show');
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
        linhas.push(['Reservado do seu saldo', fmtBRL(calc.reserva)]);
        if (saldo !== null) linhas.push(['Saldo depois', fmtBRL(saldo - calc.reserva)]);
    }
    linhas.push(['Inscrição', v.entrada ? fmtBRL(v.entrada) : 'Gratuita']);
    var ok = await Modal.confirmar({
        titulo: modoEdicao ? 'Salvar alterações?' : 'Publicar o desafio?',
        mensagem: modoEdicao ? 'As mudanças aparecem para os atletas na hora.'
            : 'O prêmio fixo' + (v.selecao === 'vaga_aberta' ? ' e o teto do instrutor ficam' : ' e o valor do instrutor ficam') + ' reservados até o encerramento. Se cancelar o desafio, a reserva volta inteira.',
        linhas: linhas, confirmar: modoEdicao ? 'Salvar' : 'Publicar'
    });
    if (!ok) return;

    var btn = $('btnSalvar');
    btnLoading(btn, true);
    try {
        if (modoEdicao) await API.editarDesafio(idEdicao, payload);
        else await API.criarDesafio(payload);
        try { sessionStorage.removeItem(CHAVE_RASCUNHO); } catch (e) {}
        showToast(modoEdicao ? 'Desafio atualizado!' : 'Desafio publicado!', 'success');
        setTimeout(function() { window.location.href = '/academia/desafios'; }, 900);
    } catch (e) {
        btnLoading(btn, false);
        erroForm.textContent = mensagemErro(e);
        erroForm.classList.add('show');
        erroForm.scrollIntoView({ block: 'center' });
        // Saldo insuficiente (o saldo local pode estar defasado): oferece o depósito
        if (e.status === 422) mostrarSaldo(Math.max(1, diff - (saldo || 0)));
    }
}

// ── Edição: carrega e trava o que o servidor recusaria ────────────────────
function aplicarEdicao(d) {
    original = d;
    var f = d.financeiro || {};
    taxaPct = f.taxa_plataforma_pct !== undefined ? f.taxa_plataforma_pct : taxaPct;
    var quando = paraCampos(d.data_evento);
    preencher({
        titulo: d.titulo, descricao: d.descricao, categoria: d.categoria || 'geral', data: quando.data, hora: quando.hora, local: d.local,
        vagas: d.vagas, posicoes: d.posicoes_podio, entrada: d.valor_entrada_centavos, premioFixo: f.premio_fixo_centavos || 0,
        dividir: !!f.dividir_pote, pctPote: f.dividir_pote ? f.percentual_pote : null,
        selecao: d.instrutor_selecao || 'designado', idInstrutor: d.instrutor ? d.instrutor.id : 0,
        modo: f.instrutor_modo_pagamento || 'fixo',
        fixo: d.instrutor_selecao === 'vaga_aberta' ? 0 : (f.instrutor_fixo_centavos || 0),
        teto: f.instrutor_teto_centavos || 0, pct: f.instrutor_pct || 0, simN: null
    });
    $('tituloPagina').textContent = 'EDITAR DESAFIO';
    $('subPagina').textContent = 'Altere o que precisar. A reserva do seu saldo é ajustada pela diferença.';
    $('btnSalvar').textContent = 'Salvar alterações';

    // Trocar o tipo de seleção depois de criado o desafio não é permitido
    Array.prototype.forEach.call(document.querySelectorAll('input[name="selecao"]'), function(r) { r.disabled = true; });
    bloqueios.selecao = true;
    if (d.participantes_count > 0) {
        bloqueios.entrada = true;
        $('entrada').disabled = true;
        bloqueios.premioMin = (f.premio_fixo_centavos || 0) + (f.pote_premio_centavos || 0);
    }
    if (d.instrutor_selecao === 'vaga_aberta' && d.instrutor) {
        bloqueios.pagamento = true;
        ['fixo', 'teto', 'pctInstrutor'].forEach(function(id) { $(id).disabled = true; });
        Array.prototype.forEach.call(document.querySelectorAll('input[name="modo"]'), function(r) { r.disabled = true; });
        var nota = $('notaSelecao');
        nota.hidden = false;
        nota.textContent = 'Instrutor já escolhido na vaga aberta: o pagamento dele não pode mais ser alterado.';
    } else if (d.instrutor_selecao === 'vaga_aberta') {
        var n = $('notaSelecao');
        n.hidden = false;
        n.textContent = 'Vaga de instrutor aberta: o instrutor sai da escolha entre os candidatos.';
    }
    if (d.instrutor_selecao === 'vaga_aberta') $('idInstrutor').disabled = true;
}

// ── Início ────────────────────────────────────────────────────────────────
async function iniciar() {
    var carregando = $('carregando');
    Skeleton.cards(carregando, 3, 110);
    try {
        var reqs = [API.taxasPlataforma().catch(function() { return null; }),
            API.carteira().catch(function() { return null; }),
            API.categoriasDesafio().catch(function() { return null; }),
            API.listarInstrutoresVinculados(1, 100),
            modoEdicao ? API.buscarDesafio(idEdicao) : Promise.resolve(null)];
        var res = await Promise.all(reqs);
        if (res[0] && res[0].data) taxaPct = res[0].data.taxa_inscricao_pct;
        if (res[1] && res[1].data) saldo = res[1].data.saldo_centavos;
        var cats = res[2] && res[2].data && res[2].data.length ? res[2].data : Object.keys(ROTULOS_CAT);
        instrutores = (res[3].data && res[3].data.itens) || [];
        var d = res[4] && res[4].data;

        if (modoEdicao && !d.pode_editar) {
            Skeleton.fim(carregando);
            limpar(carregando).appendChild(estadoVazio({
                icone: '🔒', titulo: 'Este desafio não pode mais ser editado',
                texto: 'Só desafios abertos podem ser editados.', acao: { texto: 'Voltar aos meus desafios', href: '/academia/desafios' }
            }));
            return;
        }

        desenharCategorias(cats);
        desenharInstrutores(d && d.instrutor ? d.instrutor.id : 0);
        var amanha = new Date();
        $('data').min = amanha.getFullYear() + '-' + doisDigitos(amanha.getMonth() + 1) + '-' + doisDigitos(amanha.getDate());
        // Instrutor atual pode não estar na lista de vinculados (ex.: veio da vaga): mantém como opção
        if (d && d.instrutor && !instrutores.some(function(i) { return i.id === d.instrutor.id; })) {
            $('idInstrutor').appendChild(h('option', { value: String(d.instrutor.id), selected: true, text: d.instrutor.nome }));
        }
        Mask.auto($('form'));
        if (modoEdicao) aplicarEdicao(d); else restaurarRascunho();

        Skeleton.fim(carregando);
        limpar(carregando);
        $('form').hidden = false;
        atualizar();
    } catch (e) {
        Gestao.erro(carregando, e, iniciar);
    }
}

$('form').addEventListener('input', atualizar);
$('form').addEventListener('change', atualizar);
$('form').addEventListener('submit', enviar);
iniciar();
renderBottomNav('/academia/criar-desafio');
})();
