// Minhas punições (GET /api/punicoes/minhas) com o recurso de cada uma (POST /api/punicoes/:id/recurso).
// O recurso não suspende a punição; um por punição, dentro do prazo (recorrer_ate).
if (!Auth.require()) throw new Error('unauthenticated');

(function() {
    var LIMITE = 10;
    var pagina = 1, total = 0;
    var lista = document.getElementById('lista');
    var ul = null;

    function periodo(p) {
        if (p.tipo === 'banimento') return 'Desde ' + fmtDataHora(p.inicio);
        if (p.fim) return fmtDataHora(p.inicio) + ' até ' + fmtDataHora(p.fim);
        return 'Desde ' + fmtDataHora(p.inicio) + (p.tipo === 'advertencia' ? '' : ' (sem prazo)');
    }

    function situacao(p) {
        if (p.revogada_em) return Moderacao.selo('Revogada', 'quitado');
        if (p.vigente) return Moderacao.selo('Vigente', 'vigente');
        return Moderacao.selo('Encerrada', 'encerrada');
    }

    function item(p) {
        var li = h('li', { class: 'md-item', id: 'pun-' + p.id });
        preencher(li, p);
        return li;
    }

    function preencher(li, p) {
        limpar(li);
        var tipo = Moderacao.TIPOS_PUNICAO[p.tipo] || p.tipo;
        li.appendChild(h('div', { class: 'md-item-topo' }, [
            h('div', null, [
                h('div', { class: 'md-item-titulo', text: tipo + (p.escopo === 'academia' ? ' (em uma academia)' : '') }),
                h('div', { class: 'md-item-meta', text: 'Punição #' + p.id + ' · ' + periodo(p) + ' · aplicada por ' + (p.aplicada_por === 'academia' ? 'academia' : 'equipe GymScore') })
            ]),
            situacao(p)
        ]));
        li.appendChild(h('h3', { text: 'Justificativa' }));
        li.appendChild(h('p', { class: 'md-item-texto', text: p.justificativa }));
        if (p.tipo === 'bloqueio_academia') li.appendChild(h('p', { class: 'md-item-meta', text: 'Enquanto vigente, você não pode entrar em desafios nem se candidatar a vagas desta academia.' }));
        if (p.tipo === 'suspensao' && p.vigente) li.appendChild(h('p', { class: 'md-item-meta', text: 'Enquanto vigente, você não pode entrar em desafios nem fazer ações de gestão. Carteira e notificações continuam liberadas.' }));
        if (p.revogada_em) {
            li.appendChild(h('p', { class: 'md-nota ok', text: 'Revogada em ' + fmtDataHora(p.revogada_em) + (p.revogada_motivo ? ': ' + p.revogada_motivo : '.') }));
        }

        var r = p.recurso;
        if (r) {
            li.appendChild(h('h3', { text: 'Seu recurso · ' + fmtDataHora(r.criado_em) }));
            li.appendChild(h('div', { class: 'md-acoes' }, Moderacao.selo(Moderacao.STATUS_RECURSO[r.status] || r.status, r.status)));
            li.appendChild(h('p', { class: 'md-item-texto', text: r.texto }));
            if (r.julgado_em) {
                li.appendChild(h('p', { class: 'md-nota ' + (r.status === 'deferido' ? 'ok' : 'erro'),
                    text: 'Julgado em ' + fmtDataHora(r.julgado_em) + (r.decisao_texto ? ': ' + r.decisao_texto : '.') }));
            }
        } else if (p.pode_recorrer) {
            var btn = h('button', { type: 'button', class: 'gs-btn gs-btn-secondary', text: 'Recorrer' });
            btn.addEventListener('click', function() { recorrer(li, p); });
            li.appendChild(h('div', { class: 'md-acoes' }, btn));
            if (p.recorrer_ate) li.appendChild(h('p', { class: 'md-item-meta', text: 'Prazo para recorrer: até ' + fmtDataHora(p.recorrer_ate) + '.' }));
        } else if (!p.revogada_em) {
            li.appendChild(h('p', { class: 'md-item-meta', text: 'O prazo para recorrer desta punição terminou.' }));
        }
    }

    async function recorrer(li, p) {
        var r = await Moderacao.textoModal({
            titulo: 'Recorrer da punição #' + p.id,
            mensagem: 'A equipe GymScore revisa o caso. A punição continua valendo enquanto o recurso é analisado. Só é possível um recurso por punição.',
            linhas: [['Punição', Moderacao.TIPOS_PUNICAO[p.tipo] || p.tipo], ['Período', periodo(p)]],
            rotulo: 'Por que a punição deve ser revista', min: 20, max: 2000,
            placeholder: 'Explique o que aconteceu e por que você discorda.',
            confirmar: 'Enviar recurso',
            executar: function(texto) { return API.recorrer(p.id, texto); }
        });
        if (!r) return;
        showToast('Recurso enviado.', 'success');
        p.recurso = r.data;
        p.pode_recorrer = false;
        preencher(li, p);
    }

    async function carregar(reiniciar) {
        if (reiniciar) { pagina = 1; Skeleton.cards(lista, 2, 120); }
        try {
            var r = (await API.minhasPunicoes(pagina, LIMITE)).data || {};
            total = r.total || 0;
            if (reiniciar) {
                Skeleton.fim(lista);
                limpar(lista);
                if (!total) {
                    lista.appendChild(estadoVazio({ icone: '✅', titulo: 'Nenhuma punição', texto: 'Sua conta não tem advertências, suspensões ou bloqueios.' }));
                    document.getElementById('mais').hidden = true;
                    return;
                }
                ul = h('ul', { class: 'md-lista' });
                lista.appendChild(ul);
            }
            (r.itens || []).forEach(function(p) { ul.appendChild(item(p)); });
            document.getElementById('mais').hidden = pagina * LIMITE >= total;
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
