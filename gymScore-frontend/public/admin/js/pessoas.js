// GymScore Admin — Usuários (estado de moderação e punições) e Academias/Instrutores (aprovação de cadastro).
'use strict';

(function() {
    var h = Adm.h;

    // Estado de moderação: banido (login recusado) > suspenso (até data) > ativo
    function statusUsuario(u) {
        if (u.banido_em) return Adm.badge('Banido', 'banido');
        if (u.suspenso) return h('span', { class: 'adm-badges' }, [Adm.badge('Suspenso', 'suspenso'), h('small', { class: 'adm-sub', text: 'até ' + Adm.dataHora(u.suspenso_ate) })]);
        return Adm.badge('Ativo', 'ativo');
    }

    // "Bloquear" virou aplicar punição (suspensão/banimento/...). Banido: revogar os banimentos vigentes.
    function botoesModeracao(u, aoConcluir) {
        return [
            Adm.botao('Aplicar punição', function() { Adm.aplicarPunicao({ id_usuario: u.id, nome: Adm.nome(u) }, aoConcluir); }, 'gs-btn-danger'),
            u.banido_em ? Adm.botao('Revogar banimento', function() { revogarBanimento(u, aoConcluir); }) : null
        ];
    }
    function revogarBanimento(u, aoConcluir) {
        Adm.acao({
            titulo: 'Revogar banimento — ' + Adm.nome(u),
            mensagem: 'Revoga os banimentos vigentes do usuário; ele volta a conseguir entrar (suspensões vigentes continuam valendo).',
            linhas: [['E-mail', u.email], ['Banido em', Adm.dataHora(u.banido_em)], ['Motivo', u.motivo_bloqueio || '—']],
            textoConfirmar: 'Revogar banimento',
            executar: function(d) { return Adm.post('/usuarios/' + Adm.id(u.id) + '/desbloquear', { motivo: d.motivo }); },
            sucesso: function(r) { return r && r.alterado === false ? 'Nada mudou (não havia banimento vigente).' : 'Banimento revogado.'; },
            aoConcluir: aoConcluir
        });
    }

    // Seção que carrega sozinha (punições/débitos do usuário no detalhe)
    function secaoAsync(carregar, render) {
        var box = h('div', null, Adm.carregando());
        carregar().then(function(d) { Adm.limpar(box).appendChild(render(d)); }, function(err) { Adm.limpar(box).appendChild(Adm.estadoErro(err)); });
        return box;
    }

    // ─── Detalhe de usuário (modal com ledger paginado) ──────────────────────
    Adm.abrirUsuario = function(id, aoMudar) {
        var pagina = 1;
        Adm.detalhe('Usuário #' + id, function() { return Adm.get('/usuarios/' + Adm.id(id), { pagina: pagina, limite: 20 }); }, function(d, m, recarregar) {
            var u = d.usuario;
            var ledger = d.ledger || { itens: [], total: 0, pagina: 1, limite: 20 };
            var paginas = Math.max(1, Math.ceil((ledger.total || 0) / (ledger.limite || 20)));
            return h('div', null, [
                h('div', { class: 'adm-detalhe-topo' }, [
                    h('div', null, [h('strong', { class: 'adm-detalhe-nome', text: Adm.nome(u) }), ' ', statusUsuario(u)]),
                    h('div', { class: 'adm-acoes' }, botoesModeracao(u, function() { recarregar(); if (aoMudar) aoMudar(); }))
                ]),
                Adm.dl([
                    ['E-mail', u.email],
                    ['Username', u.username],
                    ['CPF', u.cpf_mascarado],
                    ['Perfil', Adm.rotulo(Adm.PERFIS, u.perfil)],
                    ['Saldo', Adm.reais(u.saldo_centavos)],
                    ['Soma do ledger', h('span', null, [Adm.reais(d.soma_ledger_centavos), ' ',
                        d.ledger_confere ? Adm.badge('confere', 'ok') : Adm.badge('diverge do saldo', 'failed')])],
                    u.banido_em ? ['Banido em', Adm.dataHora(u.banido_em)] : null,
                    u.suspenso ? ['Suspenso até', Adm.dataHora(u.suspenso_ate)] : null,
                    u.motivo_bloqueio ? ['Motivo (moderação)', u.motivo_bloqueio] : null,
                    ['Última atividade', Adm.dataHora(u.ultima_vez)],
                    ['Cadastro', Adm.dataHora(u.criado_em)]
                ]),
                d.academia ? h('h3', { class: 'adm-h3', text: 'Cadastro de academia' }) : null,
                d.academia ? Adm.dl(linhasCadastro(d.academia)) : null,
                d.instrutor ? h('h3', { class: 'adm-h3', text: 'Cadastro de instrutor' }) : null,
                d.instrutor ? Adm.dl(linhasCadastro(d.instrutor)) : null,
                h('h3', { class: 'adm-h3', text: 'Desafios (' + (d.desafios || []).length + ')' }),
                (d.desafios && d.desafios.length) ? Adm.tabela([
                    { titulo: 'Desafio', valor: function(x) { return Adm.linkDesafio(x.id_desafio); } },
                    { titulo: 'Título', classe: 'adm-col-texto', valor: function(x) { return x.titulo; } },
                    { titulo: 'Status', valor: function(x) { return Adm.badgeStatus(Adm.STATUS_DESAFIO, x.status); } },
                    { titulo: 'Evento', classe: 'nowrap', valor: function(x) { return Adm.dataHora(x.data_evento); } },
                    { titulo: 'Presente', valor: function(x) { return x.presente ? 'Sim' : 'Não'; } },
                    { titulo: 'Prêmio', classe: 'num', valor: function(x) { return x.premio_centavos ? Adm.reais(x.premio_centavos) : '—'; } },
                    { titulo: 'Vencedor', valor: function(x) { return x.vencedor ? 'Sim' : '—'; } }
                ], d.desafios, { rotulo: 'Desafios do usuário' }) : Adm.estadoVazio('Sem participação em desafios.'),
                h('h3', { class: 'adm-h3', text: 'Punições' }),
                secaoAsync(function() { return Adm.get('/punicoes', { id_usuario: u.id, limite: 50 }); }, function(p) {
                    var itens = (p && p.itens) || [];
                    return itens.length ? Adm.tabela(Adm.colunasPunicao(function() { recarregar(); if (aoMudar) aoMudar(); }), itens, { rotulo: 'Punições do usuário' }) : Adm.estadoVazio('Nenhuma punição.');
                }),
                h('h3', { class: 'adm-h3', text: 'Débitos' }),
                secaoAsync(function() { return Adm.get('/debitos', { id_usuario: u.id, limite: 50 }); }, function(p) {
                    var itens = (p && p.itens) || [];
                    return itens.length ? Adm.tabela([
                        { titulo: 'Desafio', valor: function(x) { return Adm.linkDesafio(x.id_desafio); } },
                        { titulo: 'Valor', classe: 'num', valor: function(x) { return Adm.reais(x.valor_centavos); } },
                        { titulo: 'Restante', classe: 'num', valor: function(x) { return Adm.reais(x.saldo_restante_centavos); } },
                        { titulo: 'Situação', valor: function(x) { return x.quitado ? Adm.badge('Quitado', 'quitado') : Adm.badge('Em aberto', 'em_aberto'); } },
                        { titulo: 'Criado', classe: 'nowrap', valor: function(x) { return Adm.dataHora(x.criado_em); } }
                    ], itens, { rotulo: 'Débitos do usuário' }) : Adm.estadoVazio('Nenhum débito.');
                }),
                h('h3', { class: 'adm-h3', text: 'Ledger (' + Adm.num(ledger.total) + ')' }),
                (ledger.itens && ledger.itens.length) ? Adm.tabela(Adm.colunasTransacao(true), ledger.itens, { rotulo: 'Ledger do usuário' }) : Adm.estadoVazio('Sem transações.'),
                paginas > 1 ? h('div', { class: 'adm-paginacao' }, [
                    h('span', { class: 'adm-pag-info', text: 'Página ' + ledger.pagina + ' de ' + paginas }),
                    h('div', { class: 'adm-acoes' }, [
                        Adm.botao('‹ Anterior', function() { pagina = Math.max(1, pagina - 1); recarregar(); }),
                        Adm.botao('Próxima ›', function() { pagina = Math.min(paginas, pagina + 1); recarregar(); })
                    ])
                ]) : null
            ]);
        });
    };

    // F4-4: endereço estruturado (B4-8); cadastros antigos só têm o texto `endereco`
    function enderecoCadastro(c) {
        if (!(c.logradouro || c.cidade || c.cep)) return c.endereco ? [c.endereco] : [];
        var cep = c.cep && /^\d{8}$/.test(c.cep) ? c.cep.slice(0, 5) + '-' + c.cep.slice(5) : c.cep;
        var rua = [c.logradouro, c.numero].filter(Boolean).join(', ') + (c.complemento ? ' — ' + c.complemento : '');
        var cid = [c.cidade, c.uf].filter(Boolean).join('/');
        return [rua, [c.bairro, cid].filter(Boolean).join(' · '), cep ? 'CEP ' + cep : ''].filter(Boolean);
    }
    function linhasCadastro(c) {
        return [
            ['ID', '#' + c.id],
            ['Status', Adm.badgeStatus(Adm.STATUS_CADASTRO, c.status)],
            c.nome_fantasia ? ['Nome fantasia', c.nome_fantasia] : null,
            c.razao_social ? ['Razão social', c.razao_social] : null,
            c.cnpj ? ['CNPJ', c.cnpj] : null,
            c.cref ? ['CREF', c.cref] : null,
            c.especialidade ? ['Especialidade', c.especialidade] : null,
            c.telefone ? ['Telefone', c.telefone] : null,
            enderecoCadastro(c).length ? ['Endereço', enderecoCadastro(c).join(' · ')] : null,
            c.tipo === 'instrutor' ? ['Academia vinculada', c.id_academia ? '#' + c.id_academia : 'Autônomo'] : null,
            c.motivo_rejeicao ? ['Motivo da rejeição', c.motivo_rejeicao] : null,
            ['Aprovado em', Adm.dataHora(c.aprovado_em)],
            ['Solicitado em', Adm.dataHora(c.criado_em)]
        ];
    }

    // ─── Usuários ────────────────────────────────────────────────────────────
    Adm.telas.usuarios = function(raiz, params) {
        var lista = Adm.lista(raiz, params, {
            tela: 'usuarios', titulo: 'Usuários', descricao: 'Busca por nome, e-mail ou username. CPF sempre mascarado.',
            caminho: '/usuarios',
            filtros: [
                { nome: 'q', rotulo: 'Buscar', tipo: 'texto', placeholder: 'nome, e-mail, username', largo: true },
                { nome: 'perfil', rotulo: 'Perfil', tipo: 'select', opcoes: Adm.PERFIS },
                { nome: 'situacao', rotulo: 'Situação', tipo: 'select', opcoes: [['ativo', 'Ativos'], ['suspenso', 'Suspensos'], ['banido', 'Banidos']] }
            ],
            colunas: [
                { titulo: 'ID', valor: function(u) { return '#' + u.id; } },
                { titulo: 'Nome', valor: function(u) { return h('div', null, [h('strong', { text: Adm.nome(u) }), u.username ? h('small', { class: 'adm-sub adm-bloco', text: '@' + u.username }) : null]); } },
                { titulo: 'E-mail', valor: function(u) { return u.email; } },
                { titulo: 'CPF', classe: 'mono nowrap', valor: function(u) { return u.cpf_mascarado; } },
                { titulo: 'Perfil', valor: function(u) { return Adm.rotulo(Adm.PERFIS, u.perfil); } },
                { titulo: 'Saldo', classe: 'num', valor: function(u) { return Adm.reais(u.saldo_centavos); } },
                { titulo: 'Situação', valor: statusUsuario },
                { titulo: 'Última atividade', classe: 'nowrap', valor: function(u) { return Adm.dataHora(u.ultima_vez); } },
                { titulo: 'Ações', valor: function(u) {
                    return h('div', { class: 'adm-acoes adm-acoes-linha' }, [
                        Adm.botao('Ver', function() { Adm.abrirUsuario(u.id, function() { lista.recarregar(); }); })
                    ].concat(botoesModeracao(u, function() { lista.recarregar(); })));
                } }
            ],
            aoClicar: function(u) { Adm.abrirUsuario(u.id, function() { lista.recarregar(); }); }
        });
    };

    // ─── Academias / Instrutores ─────────────────────────────────────────────
    Adm.telas.cadastros = function(raiz, params) {
        var tipo = params.tipo === 'instrutores' ? 'instrutores' : 'academias';
        var ehAcademia = tipo === 'academias';
        var lista;
        var abas = h('div', { class: 'gs-tabs', role: 'tablist' }, [['academias', 'Academias'], ['instrutores', 'Instrutores']].map(function(a) {
            return h('button', { type: 'button', role: 'tab', class: 'gs-tab' + (a[0] === tipo ? ' active' : ''), 'aria-selected': a[0] === tipo ? 'true' : 'false',
                text: a[1], onClick: function() { Adm.navegar('cadastros', { tipo: a[0] }); } });
        }));

        function nomeCadastro(c) {
            return ehAcademia ? (c.nome_fantasia || c.razao_social || Adm.nome(c.usuario)) : Adm.nome(c.usuario);
        }
        function decidir(c, aprovar) {
            Adm.acao({
                titulo: (aprovar ? 'Aprovar ' : 'Rejeitar ') + (ehAcademia ? 'academia' : 'instrutor') + ' — ' + nomeCadastro(c),
                mensagem: aprovar ? 'O cadastro passa a operar normalmente e o usuário é notificado.' :
                    'O motivo é mostrado ao usuário como motivo da rejeição.',
                linhas: linhasCadastro(c).concat([['Usuário', c.usuario ? Adm.nome(c.usuario) + ' (' + c.usuario.email + ')' : '—']]).filter(function(l) {
                    return l && l[0] !== 'Aprovado em';
                }),
                perigo: !aprovar, textoConfirmar: aprovar ? 'Aprovar' : 'Rejeitar',
                executar: function(d) { return Adm.post('/' + tipo + '/' + Adm.id(c.id) + (aprovar ? '/aprovar' : '/rejeitar'), { motivo: d.motivo }); },
                sucesso: aprovar ? 'Cadastro aprovado.' : 'Cadastro rejeitado.',
                aoConcluir: function() { lista.recarregar(); }
            });
        }

        var colunas = [
            { titulo: 'ID', valor: function(c) { return '#' + c.id; } },
            { titulo: ehAcademia ? 'Academia' : 'Instrutor', valor: function(c) {
                return h('div', null, [h('strong', { text: nomeCadastro(c) }),
                    ehAcademia && c.razao_social ? h('small', { class: 'adm-sub adm-bloco', text: c.razao_social }) : null]);
            } },
            { titulo: 'Usuário', valor: function(c) {
                return c.usuario ? h('div', null, [Adm.linkUsuario(c.usuario.id, Adm.nome(c.usuario)), h('small', { class: 'adm-sub adm-bloco', text: c.usuario.email })]) : '—';
            } },
            ehAcademia ? { titulo: 'CNPJ', classe: 'mono nowrap', valor: function(c) { return c.cnpj; } } :
                { titulo: 'CREF', classe: 'mono nowrap', valor: function(c) { return c.cref; } },
            { titulo: 'Contato / endereço', classe: 'adm-col-texto', valor: function(c) {
                var linhas = enderecoCadastro(c);
                if (c.telefone) linhas.unshift('Tel. ' + c.telefone);
                if (!linhas.length) return '—';
                return h('div', null, linhas.map(function(t, i) { return h(i ? 'small' : 'span', { class: i ? 'adm-sub adm-bloco' : 'adm-bloco', text: t }); }));
            } },
            ehAcademia ? null : { titulo: 'Especialidade', valor: function(c) { return c.especialidade; } },
            ehAcademia ? null : { titulo: 'Academia', valor: function(c) { return c.id_academia ? '#' + c.id_academia : 'Autônomo'; } },
            { titulo: 'Status', valor: function(c) { return Adm.badgeStatus(Adm.STATUS_CADASTRO, c.status); } },
            { titulo: 'Motivo da rejeição', classe: 'adm-col-texto', valor: function(c) { return c.motivo_rejeicao; } },
            { titulo: 'Solicitado em', classe: 'nowrap', valor: function(c) { return Adm.dataHora(c.criado_em); } },
            { titulo: 'Ações', valor: function(c) {
                if (c.status !== 'pendente') return '—';
                return h('div', { class: 'adm-acoes adm-acoes-linha' }, [
                    Adm.botao('Aprovar', function() { decidir(c, true); }, 'gs-btn-primary'),
                    Adm.botao('Rejeitar', function() { decidir(c, false); }, 'gs-btn-danger')
                ]);
            } }
        ].filter(Boolean);

        lista = Adm.lista(raiz, params, {
            tela: 'cadastros', titulo: 'Academias e instrutores', descricao: 'Cadastros novos só operam depois de aprovados.',
            caminho: '/' + tipo, antes: abas, padroes: { tipo: tipo, status: 'pendente' },
            params: function(p) { return { status: p.status || 'todos' }; },
            filtros: [{ nome: 'status', rotulo: 'Status', tipo: 'select', opcoes: Adm.STATUS_CADASTRO }],
            colunas: colunas
        });
    };
})();
