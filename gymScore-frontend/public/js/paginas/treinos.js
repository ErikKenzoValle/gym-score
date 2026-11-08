// Script da página treinos.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
if (!Auth.require()) throw new Error('unauthenticated');

// ── Catálogo de exercícios por grupo muscular ────────────────────────────
var CATALOGO = {
    peito:     { label:'Peito',     exs:['Supino Reto','Supino Inclinado','Supino Declinado','Crucifixo com Halteres','Crucifixo Máquina','Cross Over','Flexão de Braço','Supino Máquina'] },
    ombro:     { label:'Ombro',     exs:['Desenvolvimento com Halteres','Desenvolvimento Máquina','Elevação Lateral','Elevação Frontal','Elevação Posterior','Encolhimento de Ombros','Face Pull'] },
    costas:    { label:'Costas',    exs:['Pulldown','Remada Curvada','Remada Sentada','Barra Fixa','Remada Unilateral','Levantamento Terra','Lombar Máquina','Remada Máquina','Remada com Halteres','Puxada Supinada','Puxada Triângulo'] },
    abdominal: { label:'Abdômen',   exs:['Abdominal Reto','Abdominal Infra','Prancha','Abdominal Bicicleta','Elevação de Pernas','Abdominal Máquina'] },
    triceps:   { label:'Tríceps',   exs:['Tríceps Testa','Tríceps Pulley','Tríceps Banco','Tríceps Máquina','Supino Fechado','Tríceps Francês','Tríceps Cross'] },
    biceps:    { label:'Bíceps',    exs:['Rosca Direta','Rosca Martelo','Rosca Concentrada','Rosca Scott','Bíceps Máquina','Rosca Alternada','Rosca com Barra'] },
    pernas:    { label:'Pernas',    exs:['Agachamento Livre','Leg Press','Cadeira Extensora','Cadeira Abdutora','Cadeira Adutora','Stiff','Gêmeos Máquina','Cadeira Flexora'] },
    antebraco: { label:'Antebraço', exs:['Rosca Invertida','Rosca Punho','Flexão de Punho','Flexão de Punho Reversa'] }
};
var GRUPOS_ORDEM = ['peito','ombro','costas','abdominal','triceps','biceps','pernas','antebraco'];

var treinos = [];
var selecionados = []; // {nome, grupo_muscular} na ordem de seleção

// ── Carregamento ─────────────────────────────────────────────────────────
async function carregar() {
    var status = limpar(document.getElementById('statusTreinos'));
    status.appendChild(h('p', { class: 'gs-hint', style: { textAlign: 'center', padding: '24px' }, text: 'Carregando treinos...' }));
    try {
        var resp = await API.listarTreinos();
        treinos = resp.data || [];
    } catch (e) {
        treinos = [];
        limpar(status).appendChild(estadoVazio({ erro: true, titulo: 'Não foi possível carregar os treinos', texto: mensagemErro(e),
            acao: { texto: 'Tentar de novo', onClick: carregar } }));
        limpar(document.getElementById('treinosList'));
        carregarRadar();
        return;
    }
    renderTreinos();
    carregarRadar();
}

function renderTreinos() {
    var list = limpar(document.getElementById('treinosList'));
    var status = limpar(document.getElementById('statusTreinos'));
    if (treinos.length === 0) {
        status.appendChild(estadoVazio({ icone: '🏋️', titulo: 'Você ainda não tem treinos',
            texto: 'Crie o seu primeiro ou importe pelo código de um amigo.',
            acao: { texto: 'Criar treino', onClick: abrirCriar }, secundaria: { texto: 'Importar por código', onClick: abrirImportar } }));
        return;
    }
    treinos.forEach(function(t) {
        var qtd = (t.exercicios || []).length;
        list.appendChild(h('div', { class: 'treino-card' }, [
            h('h3', { text: t.nome }),
            t.descricao ? h('div', { class: 'meta', text: t.descricao }) : null,
            h('div', { class: 'meta' }, [qtd + ' exercício' + (qtd !== 1 ? 's' : '') + ' · código ', h('strong', { text: t.codigo })]),
            h('div', { class: 'treino-actions' }, [
                h('button', { type: 'button', class: 'btn-exec', text: 'Executar', onClick: function() { iniciarGuia(t.id); } }),
                h('button', { type: 'button', class: 'btn-share', text: 'Compartilhar', onClick: function() { compartilhar(t.codigo); } }),
                h('button', { type: 'button', class: 'btn-del', 'aria-label': 'Excluir treino ' + t.nome, text: 'Excluir', onClick: function() { excluirTreino(t.id); } })
            ])
        ]));
    });
}

// ── Radar SVG ────────────────────────────────────────────────────────────
async function carregarRadar() {
    var dados = {};
    try {
        var resp = await API.radarTreinos();
        (resp.data || []).forEach(function(r) { dados[r.grupo_muscular] = r.total; });
    } catch (e) {}
    desenharRadar(dados);
}

function desenharRadar(dados) {
    var svg = document.getElementById('radarSvg');
    var cx = 160, cy = 150, raio = 110;
    var n = GRUPOS_ORDEM.length;
    var maxVal = 1;
    GRUPOS_ORDEM.forEach(function(g) { if ((dados[g]||0) > maxVal) maxVal = dados[g]; });

    function ponto(i, fator) {
        var ang = (Math.PI*2 * i/n) - Math.PI/2;
        return [cx + raio*fator*Math.cos(ang), cy + raio*fator*Math.sin(ang)];
    }

    var svgParts = [];
    // Anéis de grade
    [0.25,0.5,0.75,1].forEach(function(f) {
        var pts = GRUPOS_ORDEM.map(function(_,i){ var p=ponto(i,f); return p[0].toFixed(1)+','+p[1].toFixed(1); }).join(' ');
        svgParts.push('<polygon points="'+pts+'" fill="none" stroke="rgba(127,132,153,0.25)" stroke-width="1"/>');
    });
    // Eixos + labels
    GRUPOS_ORDEM.forEach(function(g,i) {
        var pe = ponto(i,1);
        svgParts.push('<line x1="'+cx+'" y1="'+cy+'" x2="'+pe[0].toFixed(1)+'" y2="'+pe[1].toFixed(1)+'" stroke="rgba(127,132,153,0.25)" stroke-width="1"/>');
        var pl = ponto(i,1.18);
        var anchor = pl[0] < cx-5 ? 'end' : (pl[0] > cx+5 ? 'start' : 'middle');
        svgParts.push('<text x="'+pl[0].toFixed(1)+'" y="'+pl[1].toFixed(1)+'" font-size="10" fill="var(--text-muted,#7B8499)" text-anchor="'+anchor+'" dominant-baseline="middle">'+CATALOGO[g].label+'</text>');
    });
    // Polígono de dados
    var dataPts = GRUPOS_ORDEM.map(function(g,i){
        var f = (dados[g]||0)/maxVal;
        if (f < 0.04) f = 0.04; // mostra um ponto mínimo
        var p = ponto(i,f);
        return p[0].toFixed(1)+','+p[1].toFixed(1);
    }).join(' ');
    svgParts.push('<polygon points="'+dataPts+'" fill="rgba(34,197,94,0.30)" stroke="#22c55e" stroke-width="2"/>');

    // Auditado: o markup do radar só usa números calculados e rótulos fixos de CATALOGO (sem dados do usuário)
    svg.innerHTML = svgParts.join('');
}

// ── Criar treino ─────────────────────────────────────────────────────────
function abrirCriar() {
    selecionados = [];
    document.getElementById('treinoNome').value = '';
    document.getElementById('treinoDesc').value = '';
    document.getElementById('criarErro').style.display = 'none';
    renderCatalogo();
    atualizarSelCount();
    abrirModal('modalCriar');
}

function renderCatalogo() {
    var cont = limpar(document.getElementById('catalogo'));
    GRUPOS_ORDEM.forEach(function(g) {
        var chips = CATALOGO[g].exs.map(function(ex) {
            var sel = selecionados.some(function(s){ return s.nome === ex && s.grupo_muscular === g; });
            return h('span', { class: 'cat-chip' + (sel ? ' sel' : ''), role: 'button', tabindex: '0',
                text: ex, onClick: function(e) { toggleEx(g, ex, e.currentTarget); },
                onKeydown: function(e) { if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); toggleEx(g, ex, e.currentTarget); } } });
        });
        cont.appendChild(h('details', { class: 'cat-grupo' }, [
            h('summary', { text: CATALOGO[g].label }),
            h('div', { class: 'cat-items' }, chips)
        ]));
    });
}

function toggleEx(grupo, nome, el) {
    var idx = selecionados.findIndex(function(s){ return s.nome === nome && s.grupo_muscular === grupo; });
    if (idx >= 0) { selecionados.splice(idx,1); el.classList.remove('sel'); }
    else { selecionados.push({ nome:nome, grupo_muscular:grupo }); el.classList.add('sel'); }
    atualizarSelCount();
}

function atualizarSelCount() {
    document.getElementById('selCount').textContent =
        selecionados.length + ' exercício' + (selecionados.length !== 1 ? 's' : '') + ' selecionado' + (selecionados.length !== 1 ? 's' : '');
}

async function salvarTreino() {
    var nome = document.getElementById('treinoNome').value.trim();
    var desc = document.getElementById('treinoDesc').value.trim();
    var err = document.getElementById('criarErro');
    err.style.display = 'none';
    if (!nome) { err.textContent = 'Dê um nome ao treino.'; err.style.display='block'; return; }
    if (selecionados.length === 0) { err.textContent = 'Escolha ao menos um exercício.'; err.style.display='block'; return; }
    try {
        await API.criarTreino({ nome:nome, descricao:desc, exercicios:selecionados });
        fecharModal('modalCriar');
        showToast('Treino criado!', 'success');
        carregar();
    } catch (e) {
        err.textContent = mensagemErro(e); err.style.display='block';
    }
}

// ── Importar / compartilhar ──────────────────────────────────────────────
function abrirImportar() {
    document.getElementById('codigoImport').value = '';
    document.getElementById('importErro').style.display = 'none';
    abrirModal('modalImportar');
}

async function importarTreino() {
    var codigo = document.getElementById('codigoImport').value.trim().toUpperCase();
    var err = document.getElementById('importErro');
    err.style.display = 'none';
    if (!codigo) { err.textContent = 'Digite o código.'; err.style.display='block'; return; }
    try {
        await API.importarTreino(codigo);
        fecharModal('modalImportar');
        showToast('Treino importado!', 'success');
        carregar();
    } catch (e) {
        err.textContent = mensagemErro(e); err.style.display='block';
    }
}

function compartilhar(codigo) {
    var texto = 'Importe meu treino no GymScore com o código: ' + codigo;
    if (navigator.share) {
        navigator.share({ title:'Treino GymScore', text:texto }).catch(function(){});
    } else if (navigator.clipboard) {
        navigator.clipboard.writeText(codigo).then(function(){ showToast('Código ' + codigo + ' copiado!', 'success'); });
    } else {
        alert(texto);
    }
}

async function excluirTreino(id) {
    var ok = await Modal.confirmar({ titulo: 'Excluir este treino?', mensagem: 'Esta ação não pode ser desfeita.', confirmar: 'Excluir', cancelar: 'Manter', perigo: true });
    if (!ok) return;
    try {
        await API.deletarTreino(id);
        showToast('Treino excluído.', 'success');
        carregar();
    } catch (e) { showToast(mensagemErro(e), 'error'); }
}

// ── Modo guiado (um exercício por vez) ───────────────────────────────────
var guiaTreino = null, guiaIdx = 0;

async function iniciarGuia(id) {
    var t = treinos.find(function(x){ return x.id === id; });
    if (!t || !(t.exercicios||[]).length) { showToast('Treino sem exercícios.', 'error'); return; }
    guiaTreino = t;
    guiaIdx = 0;
    document.getElementById('guiaTitulo').textContent = t.nome;
    abrirModal('modalGuia');
    renderGuia();
}

function renderGuia() {
    var total = guiaTreino.exercicios.length;
    var bar = document.getElementById('guiaBar');
    var prog = document.getElementById('guiaProg');
    var cont = limpar(document.getElementById('guiaConteudo'));

    if (guiaIdx >= total) {
        bar.style.width = '100%';
        prog.textContent = total + ' / ' + total;
        cont.appendChild(h('div', { class: 'guia-done' }, [
            h('div', { class: 'big', 'aria-hidden': 'true', text: '🎉' }),
            h('h3', { style: { color: 'var(--success-text)', margin: '6px 0' }, text: 'Treino concluído!' }),
            h('p', { style: { color: 'var(--text-muted)', fontSize: '0.9rem' }, text: 'Mandou bem. Seu radar foi atualizado.' }),
            h('button', { class: 'gs-btn gs-btn-primary', style: { marginTop: '16px' }, text: 'Voltar aos treinos', onClick: fecharGuia })
        ]));
        return;
    }

    var ex = guiaTreino.exercicios[guiaIdx];
    var grupoLabel = (CATALOGO[ex.grupo_muscular] && CATALOGO[ex.grupo_muscular].label) || ex.grupo_muscular;
    bar.style.width = ((guiaIdx/total)*100).toFixed(0) + '%';
    prog.textContent = guiaIdx + ' / ' + total;
    cont.appendChild(h('div', { class: 'guia-ex' }, [
        h('span', { class: 'grupo-tag', text: grupoLabel }),
        h('p', { class: 'nome', text: ex.nome }),
        h('p', { class: 'passo', text: 'Exercício ' + (guiaIdx+1) + ' de ' + total })
    ]));
    cont.appendChild(h('button', { class: 'gs-btn gs-btn-primary gs-btn-full', text: 'Concluí, próximo', onClick: concluirAtual }));
    if (guiaIdx > 0) {
        cont.appendChild(h('button', { class: 'gs-btn gs-btn-secondary gs-btn-full', style: { marginTop: '8px' }, text: 'Voltar', onClick: voltarGuia }));
    }
}

async function concluirAtual() {
    var ex = guiaTreino.exercicios[guiaIdx];
    try { await API.concluirExercicio(guiaTreino.id, ex.grupo_muscular); } catch (e) {}
    guiaIdx++;
    renderGuia();
    if (guiaIdx >= guiaTreino.exercicios.length) carregarRadar();
}

function voltarGuia() { if (guiaIdx > 0) { guiaIdx--; renderGuia(); } }
function fecharGuia() { fecharModal('modalGuia'); guiaTreino = null; }

// ── Util ─────────────────────────────────────────────────────────────────
function abrirModal(id) { Modal.abrir(document.getElementById(id)); }
function fecharModal(id) { Modal.fechar(document.getElementById(id)); }

document.getElementById('btnImportar').addEventListener('click', abrirImportar);
document.getElementById('btnCriar').addEventListener('click', abrirCriar);
document.getElementById('btnSalvar').addEventListener('click', salvarTreino);
document.getElementById('btnConfirmarImport').addEventListener('click', importarTreino);
document.getElementById('guiaFechar').addEventListener('click', fecharGuia);
document.querySelectorAll('[data-fechar]').forEach(function(b) {
    b.addEventListener('click', function() { fecharModal(b.dataset.fechar); });
});

carregar();
renderBottomNav('/treinos');
