// Script da página carteira.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
if (!Auth.require()) throw new Error('unauthenticated');
checkLGPDConsent();

var LIMITE = 15;
var pagina = 1;
var totalItens = 0;
var saldoCentavos = 0;
var acompanhando = {}; // id do saque -> true (polling ativo)
var tarifaCentavos = 0; // tarifa de saque (0 = sem tarifa), vem de GET /carteira
var saqueBloqueado = false; // débito aberto (anulação de resultado): o backend recusa o saque com 409
var debitoAberto = 0, aLiberar = 0, retido = 0;

var STATUS = {
    pending: 'Pendente',
    received: 'Confirmado',
    concluido: 'Concluído',
    refunded: 'Estornado',
    failed: 'Devolvido'
};

// ── Saldo ────────────────────────────────────────────────────────────────
function renderSaldo() {
    document.getElementById('saldoValor').textContent = Privacy.isHidden() ? 'R$ •••' : fmtBRL(saldoCentavos);
    document.getElementById('eyeIcon').textContent = Privacy.isHidden() ? '🙈' : '👁';
    document.getElementById('saldoToggle').setAttribute('aria-pressed', Privacy.isHidden() ? 'false' : 'true');
    document.querySelectorAll('[data-valor-privado]').forEach(function(el) {
        el.textContent = Privacy.isHidden() ? 'R$ •••' : fmtBRL(parseInt(el.dataset.valorPrivado, 10) || 0);
    });
    atualizarSaque();
}

// Sacar fica desabilitado sem saldo para o mínimo (R$ 5,00 + tarifa)
function atualizarSaque() {
    var minimo = 500 + tarifaCentavos;
    var btn = document.getElementById('btnAbrirSaque');
    var dica = document.getElementById('saqueDica');
    var sem = saldoCentavos < minimo || saqueBloqueado;
    btn.disabled = sem;
    dica.hidden = !sem;
    dica.textContent = saqueBloqueado
        ? 'Saque bloqueado: você tem um débito de ' + fmtBRL(debitoAberto) + ' em aberto. Os próximos créditos abatem o débito automaticamente.'
        : 'Saldo insuficiente para sacar: o mínimo é ' + fmtBRL(minimo) + (tarifaCentavos ? ' (R$ 5,00 + tarifa)' : '') + '.';
    if (sem) document.getElementById('saqueBox').hidden = true;
    atualizarTarifaInfo();
}
function atualizarTarifaInfo() {
    var el = document.getElementById('saqueTarifaInfo');
    if (!tarifaCentavos) { el.textContent = ''; return; }
    var v = Mask.centavos(document.getElementById('saqueValor'));
    el.textContent = 'Tarifa de saque: ' + fmtBRL(tarifaCentavos) + '. ' +
        (v > 0 ? 'Será debitado ' + fmtBRL(v + tarifaCentavos) + ' do seu saldo (você recebe ' + fmtBRL(v) + ').' : 'O débito é o valor sacado mais a tarifa.');
}

async function carregarSaldo() {
    try {
        var resp = await API.carteira();
        saldoCentavos = resp.data.saldo_centavos;
        tarifaCentavos = resp.data.tarifa_saque_centavos || 0;
        saqueBloqueado = !!resp.data.saque_bloqueado;
        debitoAberto = resp.data.debito_aberto_centavos || 0;
        aLiberar = resp.data.a_liberar_centavos || 0;
        retido = resp.data.retido_centavos || 0;
        var u = Auth.getUser() || {};
        u.saldo_centavos = saldoCentavos;
        Auth.saveUser(u);
    } catch (e) {
        showToast('Não foi possível atualizar o saldo.', 'error');
    }
    renderSaldo();
}

// ── Extrato paginado ─────────────────────────────────────────────────────
async function carregarExtrato() {
    var msg = document.getElementById('extratoMsg');
    var lista = limpar(document.getElementById('extratoList'));
    msg.className = 'gs-msg show';
    msg.textContent = 'Carregando extrato...';
    document.getElementById('extratoVazio').hidden = true;
    try {
        var resp = await API.extrato(pagina, LIMITE);
        var dados = resp.data || {};
        var itens = dados.itens || [];
        totalItens = dados.total || 0;
        msg.classList.remove('show');

        document.getElementById('extratoVazio').hidden = totalItens !== 0;
        itens.forEach(function(it) { lista.appendChild(itemExtrato(it)); });
        atualizarPaginacao();

        // Retoma o acompanhamento de saques pendentes que aparecem nesta página
        itens.forEach(function(it) {
            if (it.tipo === 'saque' && it.status === 'pending') acompanharSaque(it.id);
        });
    } catch (e) {
        msg.className = 'gs-msg error show';
        msg.textContent = 'Erro ao carregar o extrato. ' + mensagemErro(e);
    }
}

function itemExtrato(it) {
    var positivo = it.valor_centavos >= 0;
    var detalhe = it.descricao || '';
    return h('li', { class: 'extrato-item' }, [
        h('div', { class: 'extrato-info' }, [
            h('span', { class: 'extrato-tipo', text: rotuloTransacao(it.tipo) }),
            detalhe ? h('span', { class: 'extrato-desc', text: detalhe }) : null,
            h('span', { class: 'extrato-data', text: fmtDataHora(it.criado_em) })
        ]),
        h('div', { class: 'extrato-valor-box' }, [
            h('span', { class: 'extrato-valor ' + (positivo ? 'pos' : 'neg'),
                text: (positivo ? '+ ' : '− ') + fmtBRL(Math.abs(it.valor_centavos)) }),
            h('span', { class: 'extrato-status st-' + it.status, text: STATUS[it.status] || '—' })
        ])
    ]);
}

function atualizarPaginacao() {
    var paginas = Math.max(1, Math.ceil(totalItens / LIMITE));
    document.getElementById('extratoPag').hidden = totalItens <= LIMITE;
    document.getElementById('extratoAnt').disabled = pagina <= 1;
    document.getElementById('extratoProx').disabled = pagina >= paginas;
    document.getElementById('extratoInfo').textContent = 'Página ' + pagina + ' de ' + paginas;
}

// ── Saque PIX ────────────────────────────────────────────────────────────
function abrirSaque(abrir) {
    document.getElementById('saqueBox').hidden = !abrir;
    if (abrir) { ajustarChave(); atualizarTarifaInfo(); document.getElementById('saqueValor').focus(); }
}

function msgSaque(texto, tipo) {
    var el = document.getElementById('saqueMsg');
    el.className = 'gs-msg ' + (tipo || '') + (texto ? ' show' : '');
    el.textContent = texto || '';
}

async function enviarSaque(e) {
    e.preventDefault();
    msgSaque('');
    var valor = Mask.centavos(document.getElementById('saqueValor'));
    var tipo = document.getElementById('saqueTipo').value;
    var chave = document.getElementById('saqueChave').value.trim();

    if (valor < 500) { msgSaque('O valor mínimo do saque é R$ 5,00.', 'error'); return; }
    if (valor > 1000000) { msgSaque('O valor máximo por saque é R$ 10.000,00.', 'error'); return; }
    if (valor + tarifaCentavos > saldoCentavos) {
        msgSaque(tarifaCentavos ? 'Saldo insuficiente: o saque de ' + fmtBRL(valor) + ' mais a tarifa de ' + fmtBRL(tarifaCentavos) + ' exige ' + fmtBRL(valor + tarifaCentavos) + '.' : 'Saldo insuficiente para este saque.', 'error');
        return;
    }
    if (!chave) { msgSaque('Informe a chave PIX.', 'error'); return; }
    var erroChave = validarChave(tipo, chave);
    if (erroChave) { msgSaque(erroChave, 'error'); return; }

    var linhas = [['Valor do saque', fmtBRL(valor)]];
    if (tarifaCentavos) linhas.push(['Tarifa de saque', fmtBRL(tarifaCentavos)], ['Total debitado', fmtBRL(valor + tarifaCentavos)]);
    linhas.push(['Saldo após o saque', fmtBRL(saldoCentavos - valor - tarifaCentavos)], ['Chave PIX (' + tipo + ')', chave]);
    var ok = await Modal.confirmar({ titulo: 'Confirmar saque?', mensagem: 'O valor é debitado agora e enviado por PIX. Se o PIX falhar, devolvemos o valor e a tarifa.',
        linhas: linhas, confirmar: 'Sacar ' + fmtBRL(valor), cancelar: 'Voltar' });
    if (!ok) return;

    var btn = document.getElementById('saqueEnviar');
    btnLoading(btn, true);
    try {
        var resp = await API.sacar(valor, chave, tipo);
        showToast('Saque solicitado!', 'success');
        document.getElementById('saqueForm').reset();
        Mask.definirCentavos(document.getElementById('saqueValor'), 0);
        abrirSaque(false);
        await recarregarTudo();
        acompanharSaque(resp.data.id);
    } catch (err) {
        if (err.codigo === 'debito_aberto') {
            msgSaque('Saque bloqueado: há um débito de ' + fmtBRL((err.data || {}).debito_aberto_centavos || 0) + ' em aberto.', 'error');
            recarregarTudo();
            btnLoading(btn, false);
            return;
        }
        msgSaque(mensagemErro(err), 'error');
        if (err.status === 502) recarregarTudo(); // valor já devolvido pelo backend
    }
    btnLoading(btn, false);
}

// Faixa de status de um saque em andamento
function faixaSaque(id) {
    var el = document.getElementById('saque-' + id);
    if (!el) {
        el = h('div', { class: 'saque-status', id: 'saque-' + id, role: 'status' });
        document.getElementById('saquesAndamento').appendChild(el);
    }
    return el;
}

function acompanharSaque(id) {
    if (acompanhando[id]) return;
    acompanhando[id] = true;
    var faixa = faixaSaque(id);
    var tentativas = 0;

    async function checar() {
        tentativas++;
        try {
            var resp = await API.consultarSaque(id);
            var st = resp.data.status;
            var valor = fmtBRL(resp.data.valor_centavos);
            if (st === 'pending') {
                faixa.className = 'saque-status pendente';
                limpar(faixa).appendChild(h('span', { text: 'Saque de ' + valor + ' em processamento…' }));
                if (tentativas < 120) { setTimeout(checar, 5000); return; }
                delete acompanhando[id];
                return;
            }
            delete acompanhando[id];
            if (st === 'concluido') {
                faixa.className = 'saque-status ok';
                limpar(faixa).appendChild(h('span', { text: 'Saque de ' + valor + ' concluído.' }));
            } else {
                faixa.className = 'saque-status erro';
                limpar(faixa).appendChild(h('span', { text: 'Saque de ' + valor + ' falhou. O valor e a tarifa foram devolvidos ao seu saldo.' }));
            }
            faixa.appendChild(h('button', { type: 'button', class: 'saque-fechar', 'aria-label': 'Fechar aviso',
                text: '×', onClick: function() { faixa.remove(); } }));
            recarregarTudo();
        } catch (e) {
            // Erro de rede no polling: tenta de novo
            if (tentativas < 120) setTimeout(checar, 5000);
            else delete acompanhando[id];
        }
    }
    checar();
}

// Validação e máscara da chave conforme o tipo
function validarChave(tipo, chave) {
    if (tipo === 'CPF' && !Valid.cpf(chave)) return 'CPF inválido.';
    if (tipo === 'CNPJ' && !Valid.cnpj(chave)) return 'CNPJ inválido.';
    if (tipo === 'EMAIL' && !Valid.email(chave)) return 'E-mail inválido.';
    if (tipo === 'PHONE' && !Valid.telefone(chave)) return 'Telefone inválido (DDD + número).';
    return '';
}
function ajustarChave() {
    var tipo = document.getElementById('saqueTipo').value;
    var el = document.getElementById('saqueChave');
    var u = Auth.getUser() || {};
    el.value = '';
    el.setAttribute('inputmode', { CPF: 'numeric', CNPJ: 'numeric', PHONE: 'tel', EMAIL: 'email', EVP: 'text' }[tipo]);
    el.placeholder = { CPF: '000.000.000-00', CNPJ: '00.000.000/0000-00', PHONE: '(00) 00000-0000', EMAIL: 'voce@email.com', EVP: 'Cole a chave aleatória' }[tipo];
    el.maxLength = { CPF: 14, CNPJ: 18, PHONE: 15, EMAIL: 140, EVP: 140 }[tipo];
    if (tipo === 'CPF' && u.cpf) el.value = Mask.cpf(u.cpf);
    if (tipo === 'EMAIL' && u.email) el.value = u.email;
}
document.getElementById('saqueTipo').addEventListener('change', ajustarChave);
document.getElementById('saqueChave').addEventListener('input', function() {
    var tipo = document.getElementById('saqueTipo').value;
    var fn = { CPF: Mask.cpf, CNPJ: Mask.cnpj, PHONE: Mask.telefone }[tipo];
    if (fn) this.value = fn(this.value);
});
// setTimeout: a máscara de moeda (api.js) atualiza o valor no mesmo evento, depois deste listener
document.getElementById('saqueValor').addEventListener('input', function() { setTimeout(atualizarTarifaInfo, 0); });

// ── A receber (pódio na janela de contestação) e débitos ─────────────────
function valorPrivado(centavos) {
    return h('strong', { 'data-valor-privado': String(centavos), text: Privacy.isHidden() ? 'R$ •••' : fmtBRL(centavos) });
}
function total(rotulo, centavos, classe) {
    return h('div', { class: 'ct-total ' + (classe || '') }, [h('small', { text: rotulo }), valorPrivado(centavos)]);
}

async function carregarReceber() {
    var box = document.getElementById('receberBox');
    var el = document.getElementById('receberConteudo');
    try {
        var r = (await API.carteiraPendentes(1, 20)).data || {};
        var itens = r.itens || [];
        box.hidden = !itens.length && !r.a_liberar_centavos && !r.retido_centavos;
        if (box.hidden) return;
        limpar(el);
        el.appendChild(h('div', { class: 'ct-totais' }, [
            total('A liberar', r.a_liberar_centavos || 0),
            r.retido_centavos ? total('Retido', r.retido_centavos, 'retido') : null
        ]));
        el.appendChild(h('p', { class: 'gs-hint', text: 'Prêmios e pagamentos do pódio ficam numa janela de contestação antes de entrar no saldo. ' +
            'Se alguém contestar o resultado, o valor fica retido até a denúncia ser julgada.' }));
        el.appendChild(h('ul', { class: 'ct-lista' }, itens.map(function(p) {
            var quando = p.status === 'a_liberar' ? 'Libera em ' + fmtDataHora(p.liberar_em)
                : p.status === 'liberado' ? 'Liberado em ' + fmtDataHora(p.liberado_em)
                : p.status === 'retido' ? 'Aguardando o julgamento da contestação' : 'Cancelado pela anulação do resultado';
            return h('li', { class: 'ct-item' }, [
                h('div', { class: 'ct-item-info' }, [
                    h('strong', { text: Moderacao.TIPOS_PENDENTE[p.tipo] || p.tipo }),
                    h('span', { text: (p.titulo_desafio || 'Desafio #' + p.id_desafio) + ' · ' + quando })
                ]),
                h('div', { class: 'ct-item-valor' }, [valorPrivado(p.valor_centavos), h('div', null, Moderacao.selo(Moderacao.STATUS_PENDENTE[p.status] || p.status, p.status))])
            ]);
        })));
        if (r.total > itens.length) el.appendChild(h('p', { class: 'gs-hint', text: 'Mostrando os ' + itens.length + ' mais recentes de ' + r.total + '.' }));
    } catch (e) {
        box.hidden = false;
        limpar(el).appendChild(h('p', { class: 'gs-msg error show', text: 'Não foi possível carregar os valores a receber. ' + mensagemErro(e) }));
    }
}

async function carregarDebitos() {
    var box = document.getElementById('debitosBox');
    var el = document.getElementById('debitosConteudo');
    try {
        var r = (await API.carteiraDebitos()).data || {};
        var itens = r.itens || [];
        box.hidden = !itens.length;
        if (box.hidden) return;
        limpar(el);
        if (r.aberto_centavos > 0) {
            el.appendChild(h('div', { class: 'ct-totais' }, total('Débito em aberto', r.aberto_centavos, 'debito')));
            el.appendChild(h('p', { class: 'md-nota erro', text: 'Um resultado de desafio em que você recebeu valores foi anulado e o seu saldo não cobria o estorno. ' +
                'Enquanto houver débito o saque fica bloqueado; depósitos e prêmios que chegarem abatem o débito automaticamente.' }));
        } else {
            el.appendChild(h('p', { class: 'md-nota ok', text: 'Todos os seus débitos estão quitados.' }));
        }
        el.appendChild(h('ul', { class: 'ct-lista' }, itens.map(function(d) {
            return h('li', { class: 'ct-item' }, [
                h('div', { class: 'ct-item-info' }, [
                    h('strong', { text: d.titulo_desafio || 'Desafio #' + d.id_desafio }),
                    h('span', { text: d.motivo + ' · ' + fmtData(d.criado_em) + (d.quitado ? ' · quitado em ' + fmtData(d.quitado_em) : ' · falta ' + fmtBRL(d.saldo_restante_centavos)) })
                ]),
                h('div', { class: 'ct-item-valor' }, [valorPrivado(d.valor_centavos), h('div', null, Moderacao.selo(d.quitado ? 'Quitado' : 'Em aberto', d.quitado ? 'quitado' : 'aberto'))])
            ]);
        })));
    } catch (e) {
        box.hidden = false;
        limpar(el).appendChild(h('p', { class: 'gs-msg error show', text: 'Não foi possível carregar os débitos. ' + mensagemErro(e) }));
    }
}

async function recarregarTudo() {
    await Promise.all([carregarSaldo(), carregarExtrato(), carregarReceber(), carregarDebitos()]);
}

// ── Eventos ──────────────────────────────────────────────────────────────
document.getElementById('saldoToggle').addEventListener('click', function() { Privacy.toggle(); });
document.addEventListener('privacyChange', renderSaldo);
document.getElementById('btnAbrirSaque').addEventListener('click', function() { abrirSaque(true); });
document.getElementById('saqueCancelar').addEventListener('click', function() { abrirSaque(false); msgSaque(''); });
document.getElementById('saqueForm').addEventListener('submit', enviarSaque);
document.getElementById('extratoAnt').addEventListener('click', function() { if (pagina > 1) { pagina--; carregarExtrato(); } });
document.getElementById('extratoProx').addEventListener('click', function() { pagina++; carregarExtrato(); });

saldoCentavos = (Auth.getUser() || {}).saldo_centavos || 0;
renderSaldo();
Auth.refresh(); // CPF/e-mail (pré-preenchem a chave PIX) ficam só em memória
recarregarTudo();
renderBottomNav('/carteira');
