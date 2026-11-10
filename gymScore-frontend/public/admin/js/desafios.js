// GymScore Admin — Desafios: lista (com vencidos), detalhe financeiro e cancelamento pelo admin.
'use strict';

(function() {
    var h = Adm.h;

    function ativo(d) { return d.status === 'aberto' || d.status === 'em_andamento'; }

    function cancelar(d, aoConcluir) {
        Adm.acao({
            titulo: 'Cancelar desafio #' + d.id,
            mensagem: 'Estorna as inscrições, devolve a reserva da academia, recusa as candidaturas e notifica os envolvidos. Não dá para desfazer.',
            linhas: [['Título', d.titulo], ['Status', Adm.rotulo(Adm.STATUS_DESAFIO, d.status)], ['Evento', horario(d)],
                ['Academia', d.nome_academia || (d.id_academia ? '#' + d.id_academia : '—')]],
            perigo: true, textoConfirmar: 'Cancelar desafio',
            executar: function(x) { return Adm.post('/desafios/' + Adm.id(d.id) + '/cancelar', { motivo: x.motivo }); },
            sucesso: 'Desafio cancelado.',
            aoConcluir: aoConcluir
        });
    }

    function pct(n) { return (Number(n) || 0) + '%'; }

    // Rodada 5: endereço estruturado (cai no `local` texto nos desafios antigos)
    function enderecoDesafio(e, local) {
        if (!e || !(e.logradouro || e.cidade || e.cep)) return local || '—';
        var cep = e.cep && /^\d{8}$/.test(e.cep) ? e.cep.slice(0, 5) + '-' + e.cep.slice(5) : e.cep;
        var rua = [e.logradouro, e.numero].filter(Boolean).join(', ') + (e.complemento ? ' — ' + e.complemento : '');
        return [rua, [e.bairro, [e.cidade, e.uf].filter(Boolean).join('/')].filter(Boolean).join(' · '), cep ? 'CEP ' + cep : ''].filter(Boolean).join(' · ');
    }
    // Início–fim ("10/12/2026 08:00 – 10:00"; fim em outro dia com a data)
    function horario(d) {
        if (!d.data_evento) return '—';
        if (!d.data_fim_evento) return Adm.dataHora(d.data_evento);
        var ini = new Date(d.data_evento), fim = new Date(d.data_fim_evento);
        var hora = fim.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
        return Adm.dataHora(d.data_evento) + ' – ' + (ini.toDateString() === fim.toDateString() ? hora : Adm.dataHora(d.data_fim_evento));
    }
    function pctBP(bp) { return (bp / 100).toFixed(2).replace('.', ',').replace(/,00$/, '') + '%'; }
    function faixa(min, max, sufixo) {
        if (min == null && max == null) return 'Sem limite';
        if (min != null && max != null) return min + ' a ' + max + sufixo;
        return min != null ? 'a partir de ' + min + sufixo : 'até ' + max + sufixo;
    }

    // Anulação do resultado (superadmin): punição anulacao_resultado + financeiro nos dois modos (Q3/Q6)
    function anular(det, aoConcluir) {
        var d = det.desafio;
        var participantes = det.participantes || [];
        Adm.acao({
            titulo: 'Anular resultado do desafio #' + d.id, largo: true, perigo: true,
            mensagem: 'Irreversível no financeiro. Pagamentos a liberar/retidos são cancelados; os já liberados são estornados do saldo e o que faltar vira débito do usuário (bloqueia saque).',
            linhas: [['Título', d.titulo], ['Pódio lançado em', Adm.dataHora(d.podio_lancado_em)], ['Academia', d.nome_academia || (d.id_academia ? '#' + d.id_academia : '—')]],
            campos: [
                { nome: 'modo', rotulo: 'Modo', tipo: 'select', opcoes: [['', '— Escolha —'], ['relancar_podio', 'Relançar pódio (desafio volta a "em andamento"; a academia relança)'],
                    ['devolver_inscricoes', 'Devolver inscrições (entrada integral a todos; reserva volta à academia)']],
                    ajuda: 'Relançar só é possível se nenhum pagamento do pódio já foi liberado.' },
                { nome: 'id_usuario_punido', rotulo: 'Responsável pela fraude', tipo: 'select',
                    opcoes: [['', '— Escolha —']].concat(participantes.map(function(p) { return [String(p.id_usuario), Adm.nome(p) + ' (#' + p.id_usuario + ')']; })).concat([['outro', 'Outro usuário (informar ID)']]),
                    aoMudar: function(v, inputs, wraps) { wraps.id_outro.hidden = v !== 'outro'; } },
                { nome: 'id_outro', rotulo: 'ID do usuário responsável', tipo: 'number' },
                { nome: 'id_denuncia', rotulo: 'ID da denúncia de origem (vira procedente)', tipo: 'number', obrigatorio: false },
                { nome: 'justificativa', rotulo: 'Justificativa (vista pelo punido e pelos inscritos)', tipo: 'textarea', min: 3, max: 2000 }
            ],
            aoAbrir: function(inputs, wraps) { wraps.id_outro.hidden = true; },
            validar: function(x) {
                if (x.id_usuario_punido === 'outro' && !(parseInt(x.id_outro, 10) > 0)) return 'Informe o ID do usuário responsável.';
                return '';
            },
            textoConfirmar: 'Anular resultado',
            executar: function(x) {
                var corpo = { modo: x.modo, justificativa: x.justificativa, motivo: x.motivo,
                    id_usuario_punido: parseInt(x.id_usuario_punido === 'outro' ? x.id_outro : x.id_usuario_punido, 10) };
                if (x.id_denuncia) corpo.id_denuncia = parseInt(x.id_denuncia, 10);
                return Adm.post('/desafios/' + Adm.id(d.id) + '/anular-resultado', corpo);
            },
            sucesso: 'Resultado anulado.',
            aoConcluir: function(r) { resumoAnulacao(r); if (aoConcluir) aoConcluir(); }
        });
    }

    function resumoAnulacao(r) {
        if (!r) return;
        var m = Adm.modal({ titulo: 'Anulação do desafio #' + r.id_desafio, corpo: h('div', null, [
            Adm.dl([
                ['Modo', Adm.rotulo(Adm.MODOS_ANULACAO, r.modo)],
                ['Status do desafio', Adm.rotulo(Adm.STATUS_DESAFIO, r.status_desafio)],
                ['Punição', '#' + r.id_punicao],
                ['Pendentes cancelados', r.pendentes_cancelados + ' (' + Adm.reais(r.cancelado_centavos) + ')'],
                ['Estornado dos saldos', Adm.reais(r.estornado_centavos)],
                ['Taxa estornada', Adm.reais(r.taxa_estornada_centavos)],
                ['Inscrições devolvidas', Adm.reais(r.inscricoes_devolvidas_centavos)],
                ['Reserva devolvida', Adm.reais(r.reserva_devolvida_centavos)]
            ]),
            (r.debitos && r.debitos.length) ? h('h3', { class: 'adm-h3', text: 'Débitos criados' }) : null,
            (r.debitos && r.debitos.length) ? Adm.tabela([
                { titulo: 'Usuário', valor: function(x) { return Adm.linkUsuario(x.id_usuario, x.usuario ? x.usuario.nome : null); } },
                { titulo: 'Valor', classe: 'num', valor: function(x) { return Adm.reais(x.valor_centavos); } },
                { titulo: 'Motivo', classe: 'adm-col-texto', valor: function(x) { return x.motivo; } }
            ], r.debitos, { rotulo: 'Débitos criados' }) : null
        ]), rodape: h('div', { class: 'adm-acoes' }, h('button', { type: 'button', class: 'gs-btn gs-btn-primary', text: 'Fechar', onClick: function() { m.fechar(); } })) });
    }

    function seloAnulacao(d) {
        if (!d.resultado_anulado_em) return null;
        return Adm.badge('Resultado anulado · ' + Adm.rotulo(Adm.MODOS_ANULACAO, d.resultado_anulacao_modo), 'failed');
    }

    Adm.abrirDesafio = function(id, aoMudar) {
        Adm.detalhe('Desafio #' + id, function() { return Adm.get('/desafios/' + Adm.id(id)); }, function(det, m, recarregar) {
            var d = det.desafio, f = det.financeiro || {};
            var totais = f.totais_por_tipo || {};
            var tiposComValor = Object.keys(totais);
            return h('div', null, [
                h('div', { class: 'adm-detalhe-topo' }, [
                    h('div', { class: 'adm-badges' }, [h('strong', { class: 'adm-detalhe-nome', text: d.titulo }), ' ', Adm.badgeStatus(Adm.STATUS_DESAFIO, d.status),
                        d.vencido ? Adm.badge('Vencido', 'failed') : null, seloAnulacao(d)]),
                    h('div', { class: 'adm-acoes' }, [
                        ativo(d) ? Adm.botao('Cancelar desafio', function() {
                            cancelar(d, function() { recarregar(); if (aoMudar) aoMudar(); });
                        }, 'gs-btn-danger') : null,
                        d.pode_anular_resultado ? h('button', { type: 'button', class: 'gs-btn adm-btn-sm gs-btn-danger adm-so-super', text: 'Anular resultado',
                            onClick: function() { anular(det, function() { recarregar(); if (aoMudar) aoMudar(); }); } }) : null
                    ])
                ]),
                d.resultado_anulado_em ? h('p', { class: 'adm-aviso erro', text: 'Resultado anulado em ' + Adm.dataHora(d.resultado_anulado_em) +
                    (d.resultado_anulacao_modo === 'relancar_podio' ? ' (relançar pódio): aguardando a academia relançar.' : ' (inscrições devolvidas).') }) : null,
                Adm.dl([
                    ['Categoria', d.categoria],
                    ['Horário', horario(d)],
                    ['Local', enderecoDesafio(det.endereco, det.local)],
                    ['Entrada', Adm.reais(d.valor_entrada_centavos)],
                    ['Vagas', String(d.vagas)],
                    ['Academia', d.nome_academia || (d.id_academia ? '#' + d.id_academia : '—')],
                    ['Criador', Adm.linkUsuario(d.id_criador)],
                    ['Equipe de instrutores', (det.equipe || []).length + ' de ' + (d.instrutores_vagas || 1)],
                    ['Premiados', (det.posicoes_podio || '—') + ((det.divisao_premio_bp || []).length ? ' · ' + det.divisao_premio_bp.map(function(bp, i) { return (i + 1) + 'º ' + pctBP(bp); }).join(' / ') : '')],
                    ['Nível GymScore', faixa(det.nivel_min, det.nivel_max, '')],
                    ['Idade', faixa(det.idade_min, det.idade_max, ' anos')],
                    ['Reserva da academia', Adm.reais(d.reserva_academia_centavos)],
                    ['Criado em', Adm.dataHora(d.criado_em)],
                    ['Pódio lançado em', Adm.dataHora(d.podio_lancado_em)]
                ]),
                h('h3', { class: 'adm-h3', text: 'Financeiro' }),
                Adm.dl([
                    ['Prêmio fixo', Adm.reais(f.premio_fixo_centavos)],
                    ['Divide o pote', f.dividir_pote ? 'Sim (' + pct(f.percentual_pote) + ')' : 'Não'],
                    ['Taxa da plataforma', pct(f.taxa_plataforma_pct)],
                    ['Pagamento por instrutor', ({ fixo: 'Fixo', percentual: 'Percentual', fixo_percentual: 'Fixo + percentual' }[f.instrutor_modo_pagamento] || Adm.texto(f.instrutor_modo_pagamento)) +
                        (f.instrutor_fixo_centavos ? ' · fixo ' + Adm.reais(f.instrutor_fixo_centavos) : '') +
                        (f.instrutor_pct ? ' · ' + pct(f.instrutor_pct) : '') + ' · ' + (f.instrutores_vagas || d.instrutores_vagas || 1) + ' vaga(s)'],
                    f.instrutor_selecao ? ['Seleção do instrutor (legado)', f.instrutor_selecao] : null,
                    f.instrutor_teto_centavos != null ? ['Teto do lance (legado)', Adm.reais(f.instrutor_teto_centavos)] : null,
                    ['Receita da plataforma', Adm.reais(f.receita_plataforma_centavos)]
                ]),
                tiposComValor.length ? h('div', { class: 'adm-cartoes adm-cartoes-mini' }, tiposComValor.map(function(t) {
                    return Adm.cartao(Adm.rotulo(Adm.TIPOS_TRANSACAO, t), Adm.reais(totais[t]));
                })) : null,

                det.resumo ? h('h3', { class: 'adm-h3', text: 'Resumo' }) : null,
                det.resumo ? h('p', { class: 'adm-texto-longo', text: det.resumo }) : null,
                det.regras ? h('details', { class: 'adm-detalhes' }, [h('summary', { text: 'Regras completas' }), h('p', { class: 'adm-texto-longo', text: det.regras })]) : null,

                h('h3', { class: 'adm-h3', text: 'Equipe de instrutores (' + (det.equipe || []).length + ' de ' + (d.instrutores_vagas || 1) + ')' }),
                (det.equipe && det.equipe.length) ? Adm.tabela([
                    { titulo: 'Instrutor', valor: function(m) { return h('div', null, [Adm.linkUsuario(m.id_usuario, m.nome), h('small', { class: 'adm-sub adm-bloco', text: 'Instrutor #' + m.id }) ]); } },
                    { titulo: 'Papel', valor: function(m) { return m.responsavel ? Adm.badge('Responsável', 'aprovada') : 'Membro'; } },
                    { titulo: 'Origem', valor: function(m) { return m.origem === 'indicado' ? 'Indicado pela academia' : m.origem === 'candidatura' ? 'Candidatura' : Adm.texto(m.origem); } },
                    { titulo: 'Vínculo', valor: function(m) { return m.vinculado ? 'Vinculado' : 'Autônomo'; } },
                    { titulo: 'Entrou em', classe: 'nowrap', valor: function(m) { return Adm.dataHora(m.entrou_em); } }
                ], det.equipe, { rotulo: 'Equipe de instrutores' }) : Adm.estadoVazio('Equipe vazia.'),

                h('h3', { class: 'adm-h3', text: 'Participantes (' + (det.participantes || []).length + ')' }),
                (det.participantes && det.participantes.length) ? Adm.tabela([
                    { titulo: 'Usuário', valor: function(p) { return h('div', null, [Adm.linkUsuario(p.id_usuario, Adm.nome(p)), h('small', { class: 'adm-sub adm-bloco', text: p.email })]); } },
                    { titulo: 'Perfil', valor: function(p) { return Adm.rotulo(Adm.PERFIS, p.perfil); } },
                    { titulo: 'Inscrito em', classe: 'nowrap', valor: function(p) { return Adm.dataHora(p.inscrito_em); } },
                    { titulo: 'Presença', valor: function(p) {
                        if (!p.presente) return 'Ausente';
                        return (p.presenca_modo === 'qr' ? 'QR' : p.presenca_modo === 'manual' ? 'Manual' : 'Sim') + ' · ' + Adm.dataHora(p.presente_em) +
                            (p.presenca_marcada_por ? ' · por #' + p.presenca_marcada_por : '');
                    } },
                    { titulo: 'Prêmio', classe: 'num', valor: function(p) { return p.premio_centavos ? Adm.reais(p.premio_centavos) : '—'; } },
                    { titulo: 'Vencedor', valor: function(p) { return p.vencedor ? 'Sim' : '—'; } }
                ], det.participantes, { rotulo: 'Participantes' }) : Adm.estadoVazio('Sem inscritos.'),

                (det.podio && det.podio.length) ? h('h3', { class: 'adm-h3', text: 'Pódio' }) : null,
                (det.podio && det.podio.length) ? Adm.tabela([
                    { titulo: 'Posição', valor: function(p) { return p.posicao + 'º'; } },
                    { titulo: 'Atleta', valor: function(p) { return Adm.nome(p); } },
                    { titulo: 'Marca', valor: function(p) { return p.marca; } },
                    { titulo: 'Prêmio', classe: 'num', valor: function(p) { return Adm.reais(p.premio_centavos); } }
                ], det.podio, { rotulo: 'Pódio' }) : null,

                (det.candidaturas && det.candidaturas.length) ? h('h3', { class: 'adm-h3', text: 'Candidaturas de instrutores' }) : null,
                (det.candidaturas && det.candidaturas.length) ? Adm.tabela([
                    { titulo: 'Instrutor', valor: function(c) { return c.nome_instrutor || '#' + c.id_instrutor; } },
                    (det.candidaturas || []).some(function(c) { return c.lance_centavos != null; })
                        ? { titulo: 'Lance (legado)', classe: 'num', valor: function(c) { return c.lance_centavos != null ? Adm.reais(c.lance_centavos) : '—'; } } : null,
                    { titulo: 'Status', valor: function(c) { return Adm.texto(c.status); } },
                    { titulo: 'Mensagem', classe: 'adm-col-texto', valor: function(c) { return c.mensagem; } },
                    { titulo: 'Data', classe: 'nowrap', valor: function(c) { return Adm.dataHora(c.criado_em); } }
                ].filter(Boolean), det.candidaturas, { rotulo: 'Candidaturas' }) : null,

                h('h3', { class: 'adm-h3', text: 'Ledger do desafio (' + Adm.num(det.ledger_total) + (det.ledger_total > 100 ? ', mostrando 100' : '') + ')' }),
                (det.ledger && det.ledger.length) ? Adm.tabela(Adm.colunasTransacao(), det.ledger, { rotulo: 'Ledger do desafio' }) : Adm.estadoVazio('Sem transações.'),
                det.ledger_total > 100 ? Adm.link('Ver todas em Transações', 'transacoes', { id_desafio: d.id }) : null,

                h('h3', { class: 'adm-h3', text: 'Pagamentos do pódio' }),
                h('div', { class: 'adm-acoes' }, [
                    Adm.botao('Pagamentos do pódio deste desafio', function() { m.fechar(); Adm.navegar('pagamentos', { id_desafio: String(d.id), status: '' }); }),
                    Adm.botao('Denúncias deste desafio', function() { m.fechar(); Adm.navegar('denuncias', { id_desafio: String(d.id), status: '' }); })
                ]),
                h('h3', { class: 'adm-h3', text: 'Linha do tempo da moderação' }),
                Adm.eventosDesafio(d.id)
            ]);
        });
    };

    Adm.telas.desafios = function(raiz, params) {
        var lista = Adm.lista(raiz, params, {
            tela: 'desafios', titulo: 'Desafios', descricao: 'Vencido = data do evento + carência no passado sem encerramento.',
            caminho: '/desafios',
            filtros: [
                { nome: 'status', rotulo: 'Status', tipo: 'select', opcoes: Adm.STATUS_DESAFIO },
                { nome: 'id_academia', rotulo: 'ID academia', tipo: 'numero' },
                { nome: 'vencidos', rotulo: 'Só vencidos', tipo: 'checkbox' }
            ],
            colunas: [
                { titulo: 'ID', valor: function(d) { return '#' + d.id; } },
                { titulo: 'Título', classe: 'adm-col-texto', valor: function(d) { return h('div', null, [h('strong', { text: d.titulo }), h('small', { class: 'adm-sub adm-bloco', text: d.categoria || '' })]); } },
                { titulo: 'Status', valor: function(d) { return h('div', { class: 'adm-badges' }, [Adm.badgeStatus(Adm.STATUS_DESAFIO, d.status), d.vencido ? Adm.badge('Vencido', 'failed') : null, seloAnulacao(d)]); } },
                { titulo: 'Evento', classe: 'nowrap', valor: function(d) { return horario(d); } },
                { titulo: 'Entrada', classe: 'num', valor: function(d) { return Adm.reais(d.valor_entrada_centavos); } },
                { titulo: 'Vagas', classe: 'num', valor: function(d) { return String(d.vagas); } },
                { titulo: 'Academia', valor: function(d) { return d.nome_academia || (d.id_academia ? '#' + d.id_academia : '—'); } },
                { titulo: 'Reserva', classe: 'num', valor: function(d) { return Adm.reais(d.reserva_academia_centavos); } },
                { titulo: 'Ações', valor: function(d) {
                    return h('div', { class: 'adm-acoes adm-acoes-linha' }, [
                        Adm.botao('Ver', function() { Adm.abrirDesafio(d.id, function() { lista.recarregar(); }); }),
                        ativo(d) ? Adm.botao('Cancelar', function() { cancelar(d, function() { lista.recarregar(); }); }, 'gs-btn-danger') : null
                    ]);
                } }
            ],
            aoClicar: function(d) { Adm.abrirDesafio(d.id, function() { lista.recarregar(); }); }
        });
    };
})();
