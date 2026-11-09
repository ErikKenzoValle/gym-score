// Script da página ingressos.html (extraído do HTML para permitir CSP sem 'unsafe-inline')
if (!Auth.require()) throw new Error('unauthenticated');

var user = Auth.getUser();
if (user) document.body.dataset.perfil = Modo.atual(user);

// Código de 32 hex em grupos de 4 para leitura humana (o QR leva o valor puro)
function codigoLegivel(c) { return String(c || '').replace(/(.{4})(?=.)/g, '$1 '); }

// QR escuro sobre fundo claro (leitores esperam esse contraste), com margem branca
function gerarQR(alvo, codigo) {
    new QRCode(alvo, { text: codigo, width: 220, height: 220, colorDark: '#000000', colorLight: '#ffffff', correctLevel: QRCode.CorrectLevel.M });
}

function renderIngresso(ing) {
    var ativo = ing.status === 'aberto' || ing.status === 'em_andamento';
    var area = h('div', { class: 'ingresso-qr-area', id: 'qr-area-' + ing.id_desafio, hidden: true });
    var gerado = false;
    var toggle = h('button', { type: 'button', class: 'ingresso-qr-toggle', 'aria-expanded': 'false', 'aria-controls': 'qr-area-' + ing.id_desafio,
        text: 'Mostrar QR Code' });
    toggle.addEventListener('click', function() {
        var abrir = area.hidden;
        area.hidden = !abrir;
        toggle.setAttribute('aria-expanded', abrir ? 'true' : 'false');
        toggle.textContent = abrir ? 'Ocultar QR Code' : 'Mostrar QR Code';
        if (abrir && !gerado) {
            gerado = true;
            var caixa = h('div', { class: 'qr-caixa', role: 'img', 'aria-label': 'QR Code do ingresso para ' + ing.titulo });
            area.appendChild(caixa);
            area.appendChild(h('div', { class: 'ingresso-codigo', text: codigoLegivel(ing.codigo_ingresso) }));
            area.appendChild(h('p', { class: 'gs-hint', text: 'Se a câmera não ler, informe este código ao instrutor.' }));
            try { gerarQR(caixa, ing.codigo_ingresso); }
            catch (e) { caixa.textContent = 'Não foi possível gerar o QR. Use o código abaixo.'; }
        }
    });

    var presenca = ing.presente
        ? h('span', { class: 'presenca ok', text: 'Presença confirmada' + (ing.presente_em ? ' às ' + new Date(ing.presente_em).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' }) : '') })
        : (ativo ? h('span', { class: 'presenca', text: 'Presença pendente' }) : null);

    return h('article', { class: 'ingresso-card' }, [
        h('div', { class: 'ingresso-card-header' }, [
            h('div', { style: { flex: '1', minWidth: '0' } }, [
                h('a', { class: 'ingresso-titulo', href: '/desafios?id=' + encodeURIComponent(ing.id_desafio), text: ing.titulo || ('Desafio #' + ing.id_desafio) }),
                h('div', { class: 'ingresso-meta' }, [
                    ing.academia ? h('span', { text: ing.academia.nome_fantasia }) : null,
                    h('span', { text: ing.data ? fmtDataHora(ing.data) : 'Data a definir' + '' }),
                    ing.local ? h('span', { text: ing.local }) : null,
                    ing.valor_entrada_centavos > 0 ? h('span', { text: 'Entrada paga: ' + fmtBRL(ing.valor_entrada_centavos) }) : null
                ]),
                presenca
            ]),
            badgeStatus(ing.status)
        ]),
        ativo ? toggle : null,
        ativo ? area : null
    ]);
}

async function carregarIngressos() {
    var cont = document.getElementById('ingressosLista');
    Skeleton.cards(cont, 2, 110);
    try {
        var lista = (await API.ingressos()).data || [];
        limpar(cont);
        cont.removeAttribute('aria-busy');
        if (lista.length === 0) {
            cont.appendChild(estadoVazio({ icone: '🎟', titulo: 'Você ainda não tem ingressos',
                texto: 'Inscreva-se em um desafio para gerar seu ingresso com QR Code.',
                acao: { texto: 'Ver desafios', href: '/desafios' } }));
            return;
        }
        lista.forEach(function(ing) { cont.appendChild(renderIngresso(ing)); });
    } catch (e) {
        limpar(cont);
        cont.removeAttribute('aria-busy');
        cont.appendChild(estadoVazio({ erro: true, titulo: 'Não foi possível carregar os ingressos', texto: mensagemErro(e),
            acao: { texto: 'Tentar de novo', onClick: carregarIngressos } }));
    }
}

carregarIngressos();
renderBottomNav('/ingressos');
