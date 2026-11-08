// GymScore Admin — telas financeiras: Painel, Transações, Saques e Receita/Reservas.
'use strict';

(function() {
    var h = Adm.h;

    // Abre o usuário/desafio/transação num modal (as funções ficam em pessoas.js/desafios.js)
    function linkUsuario(id, texto) {
        if (!id) return '—';
        return h('button', { type: 'button', class: 'adm-link-btn', text: texto || ('#' + id), onClick: function() { Adm.abrirUsuario(id); } });
    }
    function linkDesafio(id) {
        if (!id) return '—';
        return h('button', { type: 'button', class: 'adm-link-btn', text: '#' + id, onClick: function() { Adm.abrirDesafio(id); } });
    }
    function linkTransacao(id) {
        if (!id) return '—';
        return h('button', { type: 'button', class: 'adm-link-btn', text: '#' + id, onClick: function() { Adm.abrirTransacao(id); } });
    }
    Adm.linkUsuario = linkUsuario;
    Adm.linkDesafio = linkDesafio;
    Adm.linkTransacao = linkTransacao;

    function revisao(t) {
        if (!t.em_revisao) return '—';
        return h('span', { class: 'adm-revisao', title: t.revisao_motivo || '' }, [Adm.badge('Em revisão', 'pending'),
            t.revisao_motivo ? h('small', { class: 'adm-sub', text: ' ' + t.revisao_motivo }) : null]);
    }

    // Colunas de transação reaproveitadas no detalhe de usuário e de desafio
    Adm.colunasTransacao = function(semUsuario) {
        return [
            { titulo: 'ID', valor: function(t) { return linkTransacao(t.id); } },
            { titulo: 'Data', classe: 'nowrap', valor: function(t) { return Adm.dataHora(t.criado_em); } },
            semUsuario ? null : { titulo: 'Usuário', valor: function(t) { return linkUsuario(t.id_usuario); } },
            { titulo: 'Tipo', valor: function(t) { return Adm.rotulo(Adm.TIPOS_TRANSACAO, t.tipo); } },
            { titulo: 'Valor', classe: 'num', valor: function(t) { return Adm.valor(t.valor_centavos); } },
            { titulo: 'Status', valor: function(t) { return Adm.badgeStatus(Adm.STATUS_TRANSACAO, t.status); } },
            { titulo: 'Desafio', valor: function(t) { return linkDesafio(t.id_desafio); } },
            { titulo: 'Revisão', valor: revisao },
            { titulo: 'Descrição', classe: 'adm-col-texto', valor: function(t) { return t.descricao; } }
        ].filter(Boolean);
    };

    // ─── Detalhe de transação ────────────────────────────────────────────────
    Adm.abrirTransacao = function(id) {
        Adm.detalhe('Transação #' + id, function() { return Adm.get('/transacoes/' + Adm.id(id)); }, function(d) {
            var t = d.transacao;
            return h('div', null, [
                Adm.dl([
                    ['Tipo', Adm.rotulo(Adm.TIPOS_TRANSACAO, t.tipo)],
                    ['Valor', Adm.valor(t.valor_centavos)],
                    ['Status', Adm.badgeStatus(Adm.STATUS_TRANSACAO, t.status)],
                    ['Usuário', d.usuario ? linkUsuario(d.usuario.id, Adm.nome(d.usuario) + ' (' + d.usuario.email + ')') : linkUsuario(t.id_usuario)],
                    ['Desafio', linkDesafio(t.id_desafio)],
                    ['Transação de origem', linkTransacao(t.id_transacao_origem)],
                    ['Cobrança Asaas', t.asaas_payment_id],
                    ['Transferência Asaas', t.asaas_transfer_id],
                    ['Descrição', t.descricao],
                    ['Em revisão', t.em_revisao ? 'Sim — ' + (t.revisao_motivo || '') : 'Não'],
                    ['Criada em', Adm.dataHora(t.criado_em)],
                    ['Atualizada em', Adm.dataHora(t.atualizado_em)]
                ]),
                d.origem ? h('h3', { class: 'adm-h3', text: 'Origem' }) : null,
                d.origem ? Adm.tabela(Adm.colunasTransacao(), [d.origem]) : null,
                (d.derivadas && d.derivadas.length) ? h('h3', { class: 'adm-h3', text: 'Derivadas (ex.: tarifa do saque)' }) : null,
                (d.derivadas && d.derivadas.length) ? Adm.tabela(Adm.colunasTransacao(), d.derivadas) : null
            ]);
        });
    };

    // ─── Painel ──────────────────────────────────────────────────────────────
    Adm.telas.painel = function(raiz, params) {
        var conteudo = h('div', null, Adm.carregando());
        raiz.appendChild(h('section', { class: 'adm-tela' }, [
            h('header', { class: 'adm-tela-topo' }, h('div', null, [
                h('h1', { class: 'adm-titulo', text: 'Painel' }),
                h('p', { class: 'adm-sub', text: 'Saúde financeira da plataforma e pendências. O período filtra só a receita.' })
            ])),
            Adm.filtros('painel', [
                { nome: 'de', rotulo: 'Receita de', tipo: 'data' },
                { nome: 'ate', rotulo: 'até', tipo: 'data' }
            ], params, {}),
            conteudo
        ]));

        async function carregar() {
            Adm.limpar(conteudo).appendChild(Adm.carregando());
            try {
                var d = await Adm.get('/dashboard', { de: params.de, ate: params.ate });
                Adm.limpar(conteudo).appendChild(render(d));
            } catch (err) {
                if (err.status === 401) return;
                Adm.limpar(conteudo).appendChild(Adm.estadoErro(err, carregar));
            }
        }

        function pendencia(rotulo, n, tela, filtro) {
            var alerta = Number(n) > 0;
            return h('a', { class: 'adm-pendencia' + (alerta ? ' alerta' : ''), href: '#' + tela + Adm.qs(filtro) }, [
                h('strong', { text: Adm.num(n) }), h('span', { text: rotulo })
            ]);
        }

        function render(d) {
            var f = d.financeiro || {}, c = d.conferencia || {}, r = d.receita || {}, p = d.pendencias || {};
            var porOrigem = r.por_origem || {};
            var status = d.desafios_por_status || {};
            return h('div', { class: 'adm-painel' }, [
                h('h2', { class: 'adm-h2', text: 'Pendências' }),
                h('div', { class: 'adm-pendencias' }, [
                    pendencia('saques pendentes', p.saques_pendentes, 'saques', { status: 'pending' }),
                    pendencia('saques em revisão', p.saques_em_revisao, 'saques', { status: 'pending', em_revisao: 'true' }),
                    pendencia('desafios vencidos', p.desafios_vencidos, 'desafios', { vencidos: 'true' }),
                    pendencia('academias pendentes', p.academias_pendentes, 'cadastros', { tipo: 'academias', status: 'pendente' }),
                    pendencia('instrutores pendentes', p.instrutores_pendentes, 'cadastros', { tipo: 'instrutores', status: 'pendente' }),
                    pendencia('denúncias na fila do admin', p.denuncias_admin, 'denuncias', { status: 'aberta,em_analise', jurisdicao: 'admin' }),
                    pendencia('escalonadas pela academia', p.denuncias_escalonadas, 'denuncias', { status: 'aberta,em_analise', jurisdicao: 'admin' }),
                    pendencia('denúncias com academias', p.denuncias_academia, 'denuncias', { status: 'aberta,em_analise', jurisdicao: 'academia' }),
                    pendencia('recursos a julgar', p.recursos_pendentes, 'recursos', { status: 'pendente' }),
                    pendencia('pagamentos retidos', p.pagamentos_retidos, 'pagamentos', { status: 'retido' }),
                    pendencia('débitos em aberto', p.debitos_abertos, 'debitos', { aberto: 'true' })
                ]),

                h('h2', { class: 'adm-h2', text: 'Conferência do caixa' }),
                h('div', { class: 'adm-conferencia ' + (c.confere ? 'ok' : 'erro'), role: c.confere ? null : 'alert' }, [
                    h('strong', { text: c.confere ? 'Confere' : 'Divergência de ' + Adm.reais(c.divergencia_centavos) }),
                    h('p', { text: 'Saldos + custódia + receitas + pendentes do pódio − débitos = ' + Adm.reais(c.lado_aplicacoes_centavos) +
                        ' · Depósitos − saques + ajustes = ' + Adm.reais(c.lado_origens_centavos) }),
                    h('p', { class: c.saldos_conferem ? '' : 'adm-texto-erro',
                        text: c.saldos_conferem ? 'Saldos das carteiras batem com o ledger.' :
                            'Saldos das carteiras diferem do ledger em ' + Adm.reais(c.saldos_vs_ledger_centavos) + '.' })
                ]),

                h('h2', { class: 'adm-h2', text: 'Financeiro' }),
                h('div', { class: 'adm-cartoes' }, [
                    Adm.cartao('Saldo em carteiras', Adm.reais(f.saldo_carteiras_centavos), 'Ledger: ' + Adm.reais(f.soma_ledger_centavos)),
                    Adm.cartao('Depósitos recebidos', Adm.reais(f.depositos_centavos)),
                    Adm.cartao('Saques', Adm.reais(f.saques_centavos), 'pendentes + concluídos'),
                    Adm.cartao('Ajustes', Adm.reais(f.ajustes_centavos), 'linhas de abertura legadas'),
                    Adm.cartao('Custódia em desafios', Adm.reais(f.custodia_total_centavos),
                        'Entradas ' + Adm.reais(f.custodia_entradas_centavos) + ' · Reservas ' + Adm.reais(f.custodia_reservas_centavos)),
                    Adm.cartao('Receita (todo o período)', Adm.reais(f.receitas_centavos), 'líquida de estornos de taxa'),
                    Adm.cartao('Pódio a liberar', Adm.reais((f.pendentes_usuarios_centavos || 0) + (f.pendentes_plataforma_centavos || 0)),
                        'usuários ' + Adm.reais(f.pendentes_usuarios_centavos) + ' · taxa ' + Adm.reais(f.pendentes_plataforma_centavos)),
                    Adm.cartao('Débitos em aberto', Adm.reais(f.debitos_abertos_centavos), 'anulações sem saldo para estornar')
                ]),

                h('h2', { class: 'adm-h2', text: 'Receita no período' + (r.de || r.ate ? ' (' + Adm.data(r.de) + ' a ' + Adm.data(r.ate) + ')' : '') }),
                h('div', { class: 'adm-cartoes' }, [
                    Adm.cartao('Total', Adm.reais(r.total_centavos), null, 'destaque')
                ].concat(Adm.ORIGENS_RECEITA.map(function(o) { return Adm.cartao(o[1], Adm.reais(porOrigem[o[0]] || 0)); }))),

                h('h2', { class: 'adm-h2', text: 'Desafios por status' }),
                h('div', { class: 'adm-pendencias' }, Adm.STATUS_DESAFIO.map(function(s) {
                    return h('a', { class: 'adm-pendencia', href: '#desafios' + Adm.qs({ status: s[0] }) }, [
                        h('strong', { text: Adm.num(status[s[0]] || 0) }), h('span', { text: s[1] })
                    ]);
                }))
            ]);
        }
        carregar();
    };

    // ─── Transações ──────────────────────────────────────────────────────────
    Adm.telas.transacoes = function(raiz, params) {
        Adm.lista(raiz, params, {
            tela: 'transacoes', titulo: 'Transações', descricao: 'Ledger completo. Clique numa linha para ver origem e derivadas.',
            caminho: '/transacoes',
            filtros: [
                { nome: 'tipo', rotulo: 'Tipo', tipo: 'select', opcoes: Adm.TIPOS_TRANSACAO },
                { nome: 'status', rotulo: 'Status', tipo: 'select', opcoes: Adm.STATUS_TRANSACAO },
                { nome: 'id_usuario', rotulo: 'ID usuário', tipo: 'numero' },
                { nome: 'id_desafio', rotulo: 'ID desafio', tipo: 'numero' },
                { nome: 'de', rotulo: 'De', tipo: 'data' },
                { nome: 'ate', rotulo: 'Até', tipo: 'data' },
                { nome: 'em_revisao', rotulo: 'Só em revisão', tipo: 'checkbox' }
            ],
            colunas: Adm.colunasTransacao(),
            aoClicar: function(t) { Adm.abrirTransacao(t.id); }
        });
    };

    // ─── Saques ──────────────────────────────────────────────────────────────
    Adm.telas.saques = function(raiz, params, ctx) {
        var lista;
        function acaoSaque(s, tipo) {
            var cfg = {
                reconciliar: { titulo: 'Reconciliar saque #' + s.id, ok: 'Consultar Asaas',
                    msg: 'Consulta a transferência no Asaas e aplica o status (concluído ou falhou).' },
                reembolsar: { titulo: 'Reembolsar saque #' + s.id, ok: 'Reembolsar', perigo: true,
                    msg: 'Devolve o valor do saque e a tarifa à carteira do usuário. Confira antes no painel do Asaas, pela referência, que a transferência não foi paga.' },
                'marcar-pago': { titulo: 'Marcar saque #' + s.id + ' como pago', ok: 'Marcar como pago', perigo: true,
                    msg: 'Conclui o saque mantendo a tarifa. Use só se o PIX foi realmente pago no Asaas.' }
            }[tipo];
            Adm.acao({
                titulo: cfg.titulo, mensagem: cfg.msg, perigo: cfg.perigo, textoConfirmar: cfg.ok,
                linhas: [
                    ['Usuário', s.usuario ? Adm.nome(s.usuario) + ' (' + s.usuario.email + ')' : '#' + s.id_usuario],
                    ['Valor do saque', Adm.reais(s.valor_saque_centavos)],
                    ['Tarifa', Adm.reais(s.tarifa_centavos)],
                    ['Referência Asaas', s.referencia_asaas],
                    ['Transferência Asaas', s.asaas_transfer_id || 'não registrada']
                ],
                executar: function(d) { return Adm.post('/saques/' + Adm.id(s.id) + '/' + tipo, { motivo: d.motivo }); },
                sucesso: function(r) {
                    if (r && r.alterado === false) return 'Nada mudou' + (r.status_asaas ? ' (Asaas: ' + r.status_asaas + ')' : '') + '.';
                    return 'Saque atualizado' + (r && r.status_asaas ? ' (Asaas: ' + r.status_asaas + ')' : '') + '.';
                },
                aoConcluir: function() { lista.recarregar(); }
            });
        }
        lista = Adm.lista(raiz, params, {
            tela: 'saques', titulo: 'Saques', descricao: 'Reembolsar e marcar como pago são exclusivos de superadmin.',
            caminho: '/saques', padroes: { status: 'pending' },
            filtros: [
                { nome: 'status', rotulo: 'Status', tipo: 'select', opcoes: [['pending', 'Pendente'], ['concluido', 'Concluído'], ['failed', 'Falhou']] },
                { nome: 'em_revisao', rotulo: 'Só em revisão', tipo: 'checkbox' }
            ],
            colunas: [
                { titulo: 'ID', valor: function(s) { return linkTransacao(s.id); } },
                { titulo: 'Data', classe: 'nowrap', valor: function(s) { return Adm.dataHora(s.criado_em); } },
                { titulo: 'Usuário', valor: function(s) {
                    return s.usuario ? h('div', null, [linkUsuario(s.usuario.id, Adm.nome(s.usuario)), h('small', { class: 'adm-sub adm-bloco', text: s.usuario.email })]) : linkUsuario(s.id_usuario);
                } },
                { titulo: 'Valor', classe: 'num', valor: function(s) { return Adm.reais(s.valor_saque_centavos); } },
                { titulo: 'Tarifa', classe: 'num', valor: function(s) { return Adm.reais(s.tarifa_centavos); } },
                { titulo: 'Status', valor: function(s) { return Adm.badgeStatus(Adm.STATUS_TRANSACAO, s.status); } },
                { titulo: 'Revisão', valor: revisao },
                { titulo: 'Referência', classe: 'mono', valor: function(s) { return s.referencia_asaas; } },
                { titulo: 'Transferência', classe: 'mono', valor: function(s) { return s.asaas_transfer_id; } },
                { titulo: 'Ações', valor: function(s) {
                    if (s.status !== 'pending') return '—';
                    return h('div', { class: 'adm-acoes adm-acoes-linha' }, [
                        s.asaas_transfer_id ? Adm.botao('Reconciliar', function() { acaoSaque(s, 'reconciliar'); }) : null,
                        ctx.superadmin ? Adm.botao('Reembolsar', function() { acaoSaque(s, 'reembolsar'); }, 'gs-btn-danger') : null,
                        ctx.superadmin ? Adm.botao('Marcar pago', function() { acaoSaque(s, 'marcar-pago'); }) : null
                    ]);
                } }
            ]
        });
    };

    // ─── Receita e reservas ──────────────────────────────────────────────────
    Adm.telas.receita = function(raiz, params) {
        var visao = params.visao === 'reservas' ? 'reservas' : 'receitas';
        var abas = h('div', { class: 'gs-tabs', role: 'tablist' }, [['receitas', 'Receitas da plataforma'], ['reservas', 'Reservas das academias']].map(function(a) {
            return h('button', { type: 'button', role: 'tab', class: 'gs-tab' + (a[0] === visao ? ' active' : ''), 'aria-selected': a[0] === visao ? 'true' : 'false',
                text: a[1], onClick: function() { Adm.navegar('receita', { visao: a[0] === 'receitas' ? '' : a[0] }); } });
        }));
        if (visao === 'reservas') {
            Adm.lista(raiz, params, {
                tela: 'receita', titulo: 'Receita', descricao: 'Reserva vigente no desafio × valor ainda retido no ledger.',
                caminho: '/reservas', antes: abas, padroes: { visao: 'reservas' },
                params: function(p) { return { status: p.status, id_academia: p.id_academia }; },
                filtros: [
                    { nome: 'status', rotulo: 'Status do desafio', tipo: 'select', opcoes: Adm.STATUS_DESAFIO },
                    { nome: 'id_academia', rotulo: 'ID academia', tipo: 'numero' }
                ],
                colunas: [
                    { titulo: 'Desafio', valor: function(r) { return linkDesafio(r.id_desafio); } },
                    { titulo: 'Título', classe: 'adm-col-texto', valor: function(r) { return r.titulo; } },
                    { titulo: 'Status', valor: function(r) { return Adm.badgeStatus(Adm.STATUS_DESAFIO, r.status); } },
                    { titulo: 'Academia', valor: function(r) { return r.id_academia ? '#' + r.id_academia : '—'; } },
                    { titulo: 'Criador', valor: function(r) { return linkUsuario(r.id_criador); } },
                    { titulo: 'Reserva', classe: 'num', valor: function(r) { return Adm.reais(r.reserva_centavos); } },
                    { titulo: 'Retido no ledger', classe: 'num', valor: function(r) { return Adm.reais(r.retido_centavos); } }
                ]
            });
            return;
        }
        Adm.lista(raiz, params, {
            tela: 'receita', titulo: 'Receita', descricao: 'Taxas de inscrição e tarifas de saque registradas em plataforma_receitas.',
            caminho: '/receitas', antes: abas,
            filtros: [
                { nome: 'origem', rotulo: 'Origem', tipo: 'select', opcoes: Adm.ORIGENS_RECEITA },
                { nome: 'id_desafio', rotulo: 'ID desafio', tipo: 'numero' },
                { nome: 'de', rotulo: 'De', tipo: 'data' },
                { nome: 'ate', rotulo: 'Até', tipo: 'data' }
            ],
            topo: function(d) {
                var tot = d.totais_por_origem || {};
                return h('div', { class: 'adm-cartoes' }, [Adm.cartao('Total filtrado', Adm.reais(d.total_centavos), null, 'destaque')].concat(
                    Adm.ORIGENS_RECEITA.map(function(o) { return Adm.cartao(o[1], Adm.reais(tot[o[0]] || 0)); })));
            },
            colunas: [
                { titulo: 'ID', valor: function(r) { return '#' + r.id; } },
                { titulo: 'Data', classe: 'nowrap', valor: function(r) { return Adm.dataHora(r.criado_em); } },
                { titulo: 'Origem', valor: function(r) { return Adm.rotulo(Adm.ORIGENS_RECEITA, r.origem); } },
                { titulo: 'Valor', classe: 'num', valor: function(r) { return Adm.valor(r.valor_centavos); } },
                { titulo: 'Desafio', valor: function(r) { return linkDesafio(r.id_desafio); } },
                { titulo: 'Transação', valor: function(r) { return linkTransacao(r.id_transacao); } }
            ]
        });
    };
})();
