// Script da página depositar.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
if (!Auth.require()) throw new Error('unauthenticated');
checkLGPDConsent();

var user = Auth.getUser() || {};
if (user.perfil) document.body.dataset.perfil = user.perfil;

var $ = function(id) { return document.getElementById(id); };
var VALIDADE_MAX_MS = 15 * 60 * 1000; // teto do polling quando o backend não informa a expiração
var chipCentavos = 0;
var valorPixCentavos = 0;
var asaasId = '';
var pollTimer = null;
var expiraEm = 0;
var sandbox = false;

// Retorno ao fluxo de origem (ex.: /depositar?voltar=/desafios?id=7). Só caminhos internos.
var destino = (function() {
    var v = new URLSearchParams(location.search).get('voltar');
    return Notificacoes._linkSeguro(v) ? v : null;
})();
if (destino) {
    $('linkVoltar').href = destino;
    $('linkVoltar').textContent = '← Voltar ao que você estava fazendo';
    $('btnConcluido').href = destino;
    $('btnConcluido').textContent = 'Voltar e continuar';
}

fetch('/health').then(function(r) { return r.json(); }).then(function(d) { sandbox = !!d.asaas_teste; }).catch(function() {});

// ── Saldo ─────────────────────────────────────────────────────────────────
function renderSaldo() {
    var oculto = Privacy.isHidden();
    $('walletSaldo').textContent = oculto ? 'R$ •••' : (user.saldo_centavos == null ? '…' : fmtBRL(user.saldo_centavos));
    $('eyeIconWallet').innerHTML = oculto // SVG estático
        ? '<path d="M17.94 17.94A10.07 10.07 0 0112 20c-7 0-11-8-11-8a18.45 18.45 0 015.06-5.94"/><path d="M9.9 4.24A9.12 9.12 0 0112 4c7 0 11 8 11 8a18.5 18.5 0 01-2.16 3.19"/><line x1="1" y1="1" x2="23" y2="23"/>'
        : '<path d="M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z"/><circle cx="12" cy="12" r="3"/>';
}
$('btnOlho').addEventListener('click', function() { Privacy.toggle(); });
document.addEventListener('privacyChange', renderSaldo);
renderSaldo();
Auth.refresh().then(function(fresh) { if (fresh) { user = fresh; renderSaldo(); semCpf(); } });

// Contas sem CPF (academia, instrutor) informam o CPF do pagador na hora; o PIX exige CPF válido
function semCpf() { $('campoCpf').hidden = !!user.cpf; }
semCpf();
$('depCpf').addEventListener('blur', function() {
    erroCampo(this, this.value && !Valid.cpf(this.value) ? 'CPF inválido. Confira os dígitos.' : '');
});

// ── Valor ────────────────────────────────────────────────────────────────
[25, 50, 100, 200].forEach(function(reais) {
    $('depChips').appendChild(h('button', { type: 'button', class: 'valor-chip', 'aria-pressed': 'false', text: 'R$ ' + reais,
        onClick: function() { selecionarChip(reais * 100, this); } }));
});
function selecionarChip(centavos, btn) {
    chipCentavos = centavos;
    document.querySelectorAll('#depChips .valor-chip').forEach(function(b) { b.setAttribute('aria-pressed', b === btn ? 'true' : 'false'); });
    Mask.definirCentavos($('depValor'), centavos);
}
$('depValor').addEventListener('input', function() {
    chipCentavos = 0;
    document.querySelectorAll('#depChips .valor-chip').forEach(function(b) { b.setAttribute('aria-pressed', 'false'); });
});

function mostrarPasso(id) {
    document.querySelectorAll('.passo').forEach(function(p) { p.classList.toggle('ativo', p.id === id); });
    var t = id === 'stepQR' ? $('qrValor') : $('depValor');
    if (t && id === 'stepValor') t.focus();
}

// ── Gerar PIX (valor em centavos) ─────────────────────────────────────────
$('stepValor').addEventListener('submit', async function(e) {
    e.preventDefault();
    var msg = $('depMsg');
    msg.className = 'gs-msg';
    var valor = Mask.centavos($('depValor'));
    if (valor < 500) { msg.className = 'gs-msg error show'; msg.textContent = 'O valor mínimo do depósito é R$ 5,00.'; return; }
    var cpf = user.cpf || $('depCpf').value;
    if (!Valid.cpf(cpf)) { msg.className = 'gs-msg error show'; msg.textContent = 'Informe um CPF válido para gerar o PIX.'; if (!user.cpf) $('depCpf').focus(); return; }
    var btn = $('btnGerarPix');
    btnLoading(btn, true);
    try {
        var d = (await API.gerarPix(valor, cpf)).data;
        asaasId = d.asaas_payment_id;
        valorPixCentavos = valor;
        $('qrImg').src = 'data:image/png;base64,' + d.qrcode_base64;
        $('pixPayload').textContent = d.payload;
        $('qrValor').textContent = fmtBRL(valor);
        var exp = d.expiration_date ? new Date(d.expiration_date).getTime() : NaN;
        expiraEm = (isFinite(exp) && exp > Date.now()) ? Math.min(exp, Date.now() + VALIDADE_MAX_MS) : Date.now() + VALIDADE_MAX_MS;
        $('qrValidade').textContent = 'Válido até ' + new Date(expiraEm).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
        definirEstado('aguardando');
        mostrarPasso('stepQR');
        iniciarPolling();
    } catch (err) {
        msg.className = 'gs-msg error show';
        msg.textContent = mensagemErro(err);
    }
    btnLoading(btn, false);
});

function definirEstado(estado) {
    var box = $('statusBox');
    box.className = 'status-box' + (estado === 'ok' ? ' ok' : (estado === 'expirado' ? ' expirado' : ''));
    $('statusText').textContent = {
        aguardando: 'Aguardando pagamento…',
        ok: 'Pagamento confirmado! Saldo atualizado.',
        expirado: 'Este QR Code expirou. Gere outro para continuar.'
    }[estado];
    $('btnConcluido').hidden = estado !== 'ok';
    $('btnNovoQR').hidden = estado !== 'expirado';
    $('btnSimular').hidden = !(sandbox && estado === 'aguardando');
    $('btnTrocar').hidden = estado === 'ok';
    $('copyBtn').hidden = estado !== 'aguardando';
    if (estado === 'ok') $('btnConcluido').focus();
}

function pararPolling() { if (pollTimer) { clearInterval(pollTimer); pollTimer = null; } }

async function verificar() {
    if (document.hidden || !asaasId) return; // pausa com a aba oculta; retoma no visibilitychange
    if (Date.now() > expiraEm) { pararPolling(); definirEstado('expirado'); return; }
    try {
        var r = await API.consultarPagamento(asaasId);
        if (r.data && r.data.status === 'received') {
            pararPolling();
            definirEstado('ok');
            var fresh = await Auth.refresh();
            if (fresh) { user = fresh; renderSaldo(); }
        }
    } catch (e) { /* silencioso: tenta de novo no próximo ciclo */ }
}
function iniciarPolling() { pararPolling(); pollTimer = setInterval(verificar, 4000); }
document.addEventListener('visibilitychange', function() { if (!document.hidden && pollTimer) verificar(); });
window.addEventListener('pagehide', pararPolling);

$('btnSimular').addEventListener('click', async function() {
    btnLoading(this, true);
    try { await API.simularPagamento(asaasId); } catch (e) { /* silencioso */ }
    btnLoading(this, false);
    verificar();
});
$('copyBtn').addEventListener('click', function() {
    navigator.clipboard.writeText($('pixPayload').textContent)
        .then(function() { showToast('Código PIX copiado!', 'success'); })
        .catch(function() { showToast('Não foi possível copiar. Selecione o código manualmente.', 'error'); });
});
function voltarAoValor() { pararPolling(); asaasId = ''; mostrarPasso('stepValor'); }
$('btnTrocar').addEventListener('click', voltarAoValor);
$('btnNovoQR').addEventListener('click', voltarAoValor);

renderBottomNav('/depositar');
