// Minhas denúncias: lista paginada (GET /api/denuncias/minhas), status, decisão e imagens.
// Enquanto a denúncia está "aberta" dá para completar as imagens (até 3). ?id=N destaca uma denúncia.
if (!Auth.require()) throw new Error('unauthenticated');

(function() {
    var LIMITE = 10;
    var pagina = 1, total = 0;
    var lista = document.getElementById('lista');
    var destaque = parseInt(new URLSearchParams(location.search).get('id'), 10) || 0;
    var ul = null;

    function item(d) {
        var li = h('li', { class: 'md-item', id: 'den-' + d.id });
        preencher(li, d);
        return li;
    }

    function preencher(li, d) {
        limpar(li);
        var evid = d.evidencias || [];
        li.appendChild(h('div', { class: 'md-item-topo' }, [
            h('div', null, [
                h('div', { class: 'md-item-titulo', text: Moderacao.rotuloCategoria(d.categoria) + ' · ' + (d.denunciado ? d.denunciado.nome : 'Usuário') }),
                h('div', { class: 'md-item-meta', text: 'Denúncia #' + d.id + ' · ' + fmtDataHora(d.criado_em) + (d.id_desafio ? ' · Desafio #' + d.id_desafio : '') })
            ]),
            Moderacao.seloDenuncia(d.status)
        ]));
        li.appendChild(h('p', { class: 'md-item-texto', text: d.descricao }));
        li.appendChild(h('p', { class: 'md-item-meta', text: d.jurisdicao === 'academia'
            ? 'Em análise pela academia do desafio' + (d.prazo_academia_ate ? ' (prazo ' + fmtDataHora(d.prazo_academia_ate) + ')' : '') + '.'
            : 'Em análise pela equipe GymScore' + (d.encaminhada_em ? ' (encaminhada em ' + fmtDataHora(d.encaminhada_em) + ')' : '') + '.' }));
        if (d.decidida_em) {
            li.appendChild(h('h3', { text: 'Decisão · ' + fmtDataHora(d.decidida_em) }));
            if (d.decisao_texto) li.appendChild(h('p', { class: 'md-item-texto', text: d.decisao_texto }));
            if (d.gerou_punicao) li.appendChild(h('p', { class: 'md-nota ok', text: 'A denúncia gerou punição ao denunciado.' }));
        }
        // A lista (/denuncias/minhas) já traz as evidências (MB6): miniaturas direto, sem buscar o detalhe
        li.appendChild(h('h3', { text: 'Imagens (' + evid.length + '/3)' }));
        if (evid.length) li.appendChild(Moderacao.galeria(evid, function(o) { return API.urlEvidenciaMinha(d.id, o); }));
        else li.appendChild(h('p', { class: 'md-item-meta', text: 'Nenhuma imagem enviada.' }));
        if (d.status === 'aberta' && evid.length < 3) li.appendChild(adicionarImagem(li, d));
    }

    // Envia mais uma imagem (validação local; o backend confere pelo conteúdo)
    function adicionarImagem(li, d) {
        var id = 'arq' + d.id;
        var input = h('input', { type: 'file', id: id, accept: 'image/jpeg,image/png,image/webp', class: 'gs-hint' });
        var msg = h('p', { class: 'gs-msg error', role: 'alert' });
        input.addEventListener('change', async function() {
            var f = input.files[0];
            msg.classList.remove('show');
            if (!f) return;
            var erro = Moderacao.MIMES_IMAGEM.indexOf(f.type) < 0 ? 'Use JPG, PNG ou WebP.' : f.size > Moderacao.EVIDENCIA_MAX_BYTES ? 'A imagem passa de 2 MB.' : '';
            if (erro) { msg.textContent = erro; msg.classList.add('show'); input.value = ''; return; }
            var ok = await Modal.confirmar({ titulo: 'Enviar imagem?', mensagem: 'A imagem é anexada à denúncia #' + d.id + ' e não pode ser removida depois.',
                linhas: [['Arquivo', f.name], ['Tamanho', Math.max(1, Math.round(f.size / 1024)) + ' KB']], confirmar: 'Enviar' });
            if (!ok) { input.value = ''; return; }
            input.disabled = true;
            try {
                await API.enviarEvidencia(d.id, f);
                showToast('Imagem enviada.', 'success');
                preencher(li, (await API.minhaDenuncia(d.id)).data);
            } catch (e) {
                input.disabled = false;
                input.value = '';
                msg.textContent = mensagemErro(e);
                msg.classList.add('show');
            }
        });
        return h('div', { class: 'gs-field' }, [h('label', { class: 'gs-label', for: id, text: 'Adicionar imagem' }), input, msg]);
    }

    async function carregar(reiniciar) {
        if (reiniciar) { pagina = 1; Skeleton.cards(lista, 3, 110); }
        try {
            var r = (await API.minhasDenuncias(pagina, LIMITE)).data || {};
            total = r.total || 0;
            if (reiniciar) {
                Skeleton.fim(lista);
                limpar(lista);
                if (!total) {
                    lista.appendChild(estadoVazio({ icone: '🛡️', titulo: 'Você não fez denúncias',
                        texto: 'Para denunciar, abra o perfil da pessoa ou o desafio e toque em "Denunciar".', acao: { texto: 'Ver desafios', href: '/desafios' } }));
                    document.getElementById('mais').hidden = true;
                    return;
                }
                ul = h('ul', { class: 'md-lista' });
                lista.appendChild(ul);
            }
            (r.itens || []).forEach(function(d) { ul.appendChild(item(d)); });
            document.getElementById('mais').hidden = pagina * LIMITE >= total;
            if (destaque) {
                var alvo = document.getElementById('den-' + destaque);
                if (alvo) {
                    alvo.scrollIntoView({ block: 'start' });
                    alvo.classList.add('destaque');
                    destaque = 0;
                }
            }
        } catch (e) {
            Skeleton.fim(lista);
            limpar(lista).appendChild(estadoVazio({ erro: true, titulo: 'Não foi possível carregar', texto: mensagemErro(e),
                acao: { texto: 'Tentar de novo', onClick: function() { carregar(true); } } }));
        }
    }

    document.getElementById('btnMais').addEventListener('click', function() { pagina++; carregar(false); });
    carregar(true);
    renderBottomNav('/perfil');
})();
