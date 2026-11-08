// GymScore Admin — Moderação: denúncias (fila, detalhe com evidências, assumir, decidir com punições),
// punições (aplicar, revogar), recursos (julgar), pagamentos pendentes do pódio e débitos.
// Toda escrita passa por Adm.acao (motivo obrigatório 3..500, vai para admin_auditoria).
'use strict';

(function() {
    var h = Adm.h;

    function aberta(d) { return d.status === 'aberta' || d.status === 'em_analise'; }
    function pessoa(u, id) {
        if (!u && !id) return '—';
        return Adm.linkUsuario(u ? u.id : id, u ? (u.nome + (u.username ? ' (@' + u.username + ')' : '')) : '#' + id);
    }
    // "2026-11-08T10:00" (datetime-local, hora local) -> ISO UTC
    function isoDe(v) { if (!v) return null; var d = new Date(v); return isNaN(d.getTime()) ? null : d.toISOString(); }
    function futuro(v) { var d = new Date(v); return !isNaN(d.getTime()) && d.getTime() > Date.now(); }
    function situacaoPunicao(p) {
        if (p.revogada_em) return Adm.badge('Revogada', 'revogada');
        return p.vigente ? Adm.badge('Vigente', 'vigente') : Adm.badge('Encerrada', 'encerrada');
    }
    function periodo(p) {
        return Adm.dataHora(p.inicio) + (p.fim ? ' → ' + Adm.dataHora(p.fim) : (p.tipo === 'banimento' || p.tipo === 'bloqueio_academia' || p.tipo === 'suspensao' ? ' → sem prazo' : ''));
    }

    // ─── Linha do tempo (moderacao_eventos) ─────────────────────────────────
    var ROTULOS_EVENTO = {
        denuncia_criada: 'Denúncia criada', evidencia_enviada: 'Evidência enviada', denuncia_assumida: 'Denúncia assumida',
        denuncia_encaminhada: 'Encaminhada ao admin', denuncia_escalonada: 'Escalonada por prazo', denuncia_decidida: 'Denúncia decidida',
        punicao_aplicada: 'Punição aplicada', punicao_revogada: 'Punição revogada', punicao_expirada: 'Punição expirada',
        recurso_aberto: 'Recurso aberto', recurso_julgado: 'Recurso julgado', pagamento_retido: 'Pagamento retido',
        pagamento_reaberto: 'Pagamento reaberto', pagamento_liberado: 'Pagamento liberado', pagamento_cancelado: 'Pagamento cancelado',
        pagamento_estornado: 'Pagamento estornado', podio_lancado: 'Pódio lançado', resultado_anulado: 'Resultado anulado',
        inscricoes_devolvidas: 'Inscrições devolvidas', debito_criado: 'Débito criado', debito_abatido: 'Débito abatido', debito_quitado: 'Débito quitado'
    };
    Adm.linhaTempo = function(eventos) {
        if (!eventos || !eventos.length) return Adm.estadoVazio('Sem eventos.');
        return Adm.tabela([
            { titulo: 'Quando', classe: 'nowrap', valor: function(e) { return Adm.dataHora(e.criado_em); } },
            { titulo: 'Ação', valor: function(e) { return ROTULOS_EVENTO[e.acao] || e.acao; } },
            { titulo: 'Ator', valor: function(e) {
                var papel = { admin: 'Admin', academia: 'Academia', usuario: 'Usuário', sistema: 'Sistema' }[e.ator_papel] || e.ator_papel;
                if (e.ator_papel === 'admin' && e.id_admin) return papel + ' #' + e.id_admin;
                if (e.id_ator) return h('span', null, [papel + ' ', Adm.linkUsuario(e.id_ator)]);
                return papel;
            } },
            { titulo: 'Entidade', valor: function(e) { return e.entidade + ' #' + e.id_entidade; } },
            { titulo: 'Justificativa', classe: 'adm-col-texto', valor: function(e) { return e.justificativa; } },
            { titulo: 'Dados', valor: function(e) {
                if (!e.dados) return '—';
                return h('details', { class: 'adm-detalhes' }, [h('summary', { text: 'ver' }), h('pre', { text: JSON.stringify(e.dados, null, 2) })]);
            } }
        ], eventos, { rotulo: 'Linha do tempo' });
    };
    // Monta a linha do tempo de um desafio num container (carrega sozinho)
    Adm.eventosDesafio = function(idDesafio) {
        var box = h('div', null, Adm.carregando());
        Adm.get('/moderacao/eventos', { id_desafio: idDesafio, limite: 100 }).then(function(p) {
            Adm.limpar(box).appendChild(Adm.linhaTempo((p && p.itens) || []));
        }, function(err) { Adm.limpar(box).appendChild(Adm.estadoErro(err)); });
        return box;
    };

    // ─── Evidências ─────────────────────────────────────────────────────────
    function galeria(d) {
        var evid = d.evidencias || [];
        if (!evid.length) return Adm.estadoVazio('Sem evidências.');
        return h('div', { class: 'adm-galeria' }, evid.map(function(ev) {
            var fig = h('figure', { class: 'adm-thumb' }, h('span', { class: 'adm-sub', text: 'Carregando…' }));
            Adm.imagemAutenticada('/denuncias/' + Adm.id(d.id) + '/evidencias/' + Adm.id(ev.ordem)).then(function(src) {
                var btn = h('button', { type: 'button', class: 'adm-thumb-btn', 'aria-label': 'Ampliar evidência ' + ev.ordem },
                    h('img', { src: src, alt: 'Evidência ' + ev.ordem }));
                btn.addEventListener('click', function() {
                    var m = Adm.modal({ titulo: 'Evidência ' + ev.ordem + ' da denúncia #' + d.id, largo: true,
                        corpo: h('img', { class: 'adm-img-grande', src: src, alt: 'Evidência ' + ev.ordem }),
                        rodape: h('div', { class: 'adm-acoes' }, h('button', { type: 'button', class: 'gs-btn gs-btn-secondary', text: 'Fechar', onClick: function() { m.fechar(); } })) });
                });
                Adm.limpar(fig).appendChild(btn);
                fig.appendChild(h('figcaption', { text: '#' + ev.ordem + ' · ' + ev.mime + ' · ' + Math.max(1, Math.round(ev.tamanho_bytes / 1024)) + ' KB' }));
            }, function(err) {
                Adm.limpar(fig).appendChild(h('span', { class: 'adm-texto-erro', text: err.message }));
            });
            return fig;
        }));
    }

    // ─── Punições: aplicar (direto ou na decisão) ───────────────────────────
    var TIPOS_APLICAVEIS = Adm.TIPOS_PUNICAO.filter(function(t) { return t[0] !== 'anulacao_resultado'; });

    // preset: {id_usuario, id_denuncia, nome}
    Adm.aplicarPunicao = function(preset, aoConcluir) {
        preset = preset || {};
        function ajustar(tipo, inputs, wraps) {
            wraps.fim.hidden = tipo === 'banimento' || tipo === '';
            wraps.id_academia.hidden = tipo !== 'bloqueio_academia' && tipo !== 'advertencia';
            wraps.fim.querySelector('label').firstChild.textContent = tipo === 'suspensao' ? 'Fim da suspensão' : 'Fim';
        }
        Adm.acao({
            titulo: 'Aplicar punição' + (preset.nome ? ' — ' + preset.nome : ''),
            mensagem: 'Suspensão e banimento derrubam as sessões do usuário na hora. Banimento impede o login (o usuário pode recorrer pela tela de login).',
            largo: true,
            campos: [
                { nome: 'id_usuario', rotulo: 'ID do usuário', tipo: 'number', valor: preset.id_usuario ? String(preset.id_usuario) : '' },
                { nome: 'tipo', rotulo: 'Tipo', tipo: 'select', opcoes: [['', '— Escolha —']].concat(TIPOS_APLICAVEIS), valor: '',
                    aoMudar: function(v, inputs, wraps) { ajustar(v, inputs, wraps); } },
                { nome: 'id_academia', rotulo: 'ID da academia (bloqueio; opcional na advertência)', tipo: 'number', obrigatorio: false },
                { nome: 'fim', rotulo: 'Fim', tipo: 'datetime-local', obrigatorio: false, ajuda: 'Obrigatório na suspensão; opcional no bloqueio.' },
                { nome: 'id_denuncia', rotulo: 'ID da denúncia relacionada', tipo: 'number', obrigatorio: false, valor: preset.id_denuncia ? String(preset.id_denuncia) : '' },
                { nome: 'justificativa', rotulo: 'Justificativa (vista pelo punido)', tipo: 'textarea', min: 3, max: 2000 }
            ],
            aoAbrir: function(inputs, wraps) { ajustar('', inputs, wraps); },
            validar: function(d) {
                if (!(parseInt(d.id_usuario, 10) > 0)) return 'Informe o ID do usuário.';
                if (d.tipo === 'suspensao' && !d.fim) return 'A suspensão precisa de data de fim.';
                if (d.tipo !== 'banimento' && d.fim && !futuro(d.fim)) return 'A data de fim precisa ser futura.';
                if (d.tipo === 'bloqueio_academia' && !(parseInt(d.id_academia, 10) > 0)) return 'O bloqueio precisa do ID da academia.';
                return '';
            },
            perigo: true, textoConfirmar: 'Aplicar punição',
            executar: function(d) {
                var corpo = { id_usuario: parseInt(d.id_usuario, 10), tipo: d.tipo, justificativa: d.justificativa, motivo: d.motivo };
                if ((d.tipo === 'bloqueio_academia' || d.tipo === 'advertencia') && d.id_academia) corpo.id_academia = parseInt(d.id_academia, 10);
                if (d.tipo !== 'banimento' && d.fim) corpo.fim = isoDe(d.fim);
                if (d.id_denuncia) corpo.id_denuncia = parseInt(d.id_denuncia, 10);
                return Adm.post('/punicoes', corpo);
            },
            sucesso: 'Punição aplicada.',
            aoConcluir: aoConcluir
        });
    };

    function revogar(p, aoConcluir) {
        Adm.acao({
            titulo: 'Revogar punição #' + p.id,
            mensagem: 'O estado da conta é recalculado (se houver outra suspensão/banimento vigente, ele continua valendo).',
            linhas: [['Usuário', p.usuario ? p.usuario.nome : '#' + p.id_usuario], ['Tipo', Adm.rotulo(Adm.TIPOS_PUNICAO, p.tipo)], ['Período', periodo(p)], ['Justificativa', p.justificativa]],
            textoConfirmar: 'Revogar',
            executar: function(d) { return Adm.post('/punicoes/' + Adm.id(p.id) + '/revogar', { motivo: d.motivo }); },
            sucesso: 'Punição revogada.',
            aoConcluir: aoConcluir
        });
    }

    // ─── Denúncia: assumir e decidir ────────────────────────────────────────
    function assumir(d, aoConcluir) {
        Adm.acao({
            titulo: 'Assumir a denúncia #' + d.id,
            mensagem: 'A denúncia passa para "em análise" com você como responsável.',
            linhas: [['Denunciado', d.denunciado ? d.denunciado.nome : '—'], ['Categoria', Adm.rotulo(Adm.CATEGORIAS_DENUNCIA, d.categoria)]],
            textoConfirmar: 'Assumir',
            executar: function(x) { return Adm.post('/denuncias/' + Adm.id(d.id) + '/assumir', { motivo: x.motivo }); },
            sucesso: 'Denúncia assumida.',
            aoConcluir: aoConcluir
        });
    }

    // Construtor de até 5 punições ao denunciado (só com "procedente")
    function construtorPunicoes() {
        var linhas = [];
        var lista = h('div', { class: 'adm-punicoes-lista' });
        var btnAdd = h('button', { type: 'button', class: 'gs-btn gs-btn-secondary adm-btn-sm', text: '+ Punição' });
        function addLinha() {
            if (linhas.length >= 5) return;
            var n = linhas.length + 1;
            var tipo = h('select', { class: 'gs-input', 'aria-label': 'Tipo da punição ' + n }, TIPOS_APLICAVEIS.map(function(t) { return h('option', { value: t[0], text: t[1] }); }));
            var acad = h('input', { class: 'gs-input', type: 'number', min: '1', placeholder: 'ID academia', 'aria-label': 'ID da academia da punição ' + n });
            var fim = h('input', { class: 'gs-input', type: 'datetime-local', 'aria-label': 'Fim da punição ' + n });
            var linha = { tipo: tipo, acad: acad, fim: fim };
            var rem = h('button', { type: 'button', class: 'gs-btn gs-btn-secondary adm-btn-sm', text: 'Remover', 'aria-label': 'Remover punição ' + n });
            var el = h('div', { class: 'adm-punicao-linha' }, [tipo, acad, fim, rem]);
            function refletir() {
                acad.hidden = tipo.value !== 'bloqueio_academia' && tipo.value !== 'advertencia';
                fim.hidden = tipo.value === 'banimento';
            }
            tipo.addEventListener('change', refletir);
            rem.addEventListener('click', function() { linhas.splice(linhas.indexOf(linha), 1); el.remove(); btnAdd.disabled = linhas.length >= 5; });
            refletir();
            linhas.push(linha);
            lista.appendChild(el);
            btnAdd.disabled = linhas.length >= 5;
        }
        btnAdd.addEventListener('click', addLinha);
        var box = h('fieldset', { class: 'adm-campo adm-fieldset', hidden: true }, [
            h('legend', { text: 'Punições ao denunciado (opcional, até 5)' }),
            h('small', { class: 'adm-sub adm-bloco', text: 'Suspensão exige fim; banimento não tem fim; bloqueio exige o ID da academia. Para anular o resultado use "Anular resultado" no desafio.' }),
            lista, btnAdd
        ]);
        return {
            el: box,
            validar: function() {
                for (var i = 0; i < linhas.length; i++) {
                    var l = linhas[i], t = l.tipo.value;
                    if (t === 'suspensao' && !l.fim.value) return 'Punição ' + (i + 1) + ': a suspensão precisa de fim.';
                    if (t !== 'banimento' && l.fim.value && !futuro(l.fim.value)) return 'Punição ' + (i + 1) + ': o fim precisa ser futuro.';
                    if (t === 'bloqueio_academia' && !(parseInt(l.acad.value, 10) > 0)) return 'Punição ' + (i + 1) + ': informe o ID da academia.';
                }
                return '';
            },
            valor: function() {
                return linhas.map(function(l) {
                    var p = { tipo: l.tipo.value };
                    if (!l.acad.hidden && l.acad.value) p.id_academia = parseInt(l.acad.value, 10);
                    if (!l.fim.hidden && l.fim.value) p.fim = isoDe(l.fim.value);
                    return p;
                });
            },
            mostrar: function(sim) { box.hidden = !sim; }
        };
    }

    function decidir(d, aoConcluir) {
        var pun = construtorPunicoes();
        Adm.acao({
            titulo: 'Decidir a denúncia #' + d.id, largo: true,
            mensagem: 'A decisão é definitiva. Se a denúncia retinha pagamentos do pódio, eles voltam a liberar (salvo outra denúncia aberta no desafio).',
            linhas: [['Denunciado', d.denunciado ? d.denunciado.nome : '—'], ['Categoria', Adm.rotulo(Adm.CATEGORIAS_DENUNCIA, d.categoria)]],
            campos: [
                { nome: 'status', rotulo: 'Resultado', tipo: 'select', opcoes: [['', '— Escolha —'], ['procedente', 'Procedente'], ['improcedente', 'Improcedente'], ['arquivada', 'Arquivada']],
                    aoMudar: function(v) { pun.mostrar(v === 'procedente'); } },
                { nome: 'decisao_texto', rotulo: 'Texto da decisão (o denunciante vê)', tipo: 'textarea', min: 3, max: 2000 }
            ],
            extra: pun.el,
            validar: function(x) { return x.status === 'procedente' ? pun.validar() : ''; },
            textoConfirmar: 'Confirmar decisão',
            executar: function(x) {
                var corpo = { status: x.status, decisao_texto: x.decisao_texto, motivo: x.motivo };
                if (x.status === 'procedente') { var ps = pun.valor(); if (ps.length) corpo.punicoes = ps; }
                return Adm.post('/denuncias/' + Adm.id(d.id) + '/decidir', corpo);
            },
            sucesso: 'Denúncia decidida.',
            aoConcluir: aoConcluir
        });
    }

    // ─── Detalhe da denúncia ────────────────────────────────────────────────
    Adm.abrirDenuncia = function(id, aoMudar) {
        Adm.detalhe('Denúncia #' + id, function() { return Adm.get('/denuncias/' + Adm.id(id)); }, function(det, m, recarregar) {
            var d = det.denuncia;
            function depois() { recarregar(); if (aoMudar) aoMudar(); }
            var comAdmin = d.jurisdicao === 'admin';
            return h('div', null, [
                h('div', { class: 'adm-detalhe-topo' }, [
                    h('div', { class: 'adm-badges' }, [h('strong', { class: 'adm-detalhe-nome', text: Adm.rotulo(Adm.CATEGORIAS_DENUNCIA, d.categoria) }),
                        Adm.badgeStatus(Adm.STATUS_DENUNCIA, d.status), Adm.badge(comAdmin ? 'Jurisdição: admin' : 'Jurisdição: academia', comAdmin ? 'admin' : 'pendente')]),
                    h('div', { class: 'adm-acoes' }, [
                        aberta(d) && comAdmin && d.status === 'aberta' ? Adm.botao('Assumir', function() { assumir(d, depois); }) : null,
                        aberta(d) && comAdmin ? Adm.botao('Decidir', function() { decidir(d, depois); }, 'gs-btn-primary') : null,
                        Adm.botao('Aplicar punição', function() { Adm.aplicarPunicao({ id_usuario: d.denunciado.id, id_denuncia: d.id, nome: d.denunciado.nome }, depois); }, 'gs-btn-danger'),
                        d.id_desafio ? Adm.botao('Abrir desafio', function() { Adm.abrirDesafio(d.id_desafio); }) : null
                    ])
                ]),
                !comAdmin && aberta(d) ? h('p', { class: 'adm-aviso', text: 'Com a academia julgadora até ' + Adm.dataHora(d.prazo_academia_ate) + ' (só leitura). Se ela não decidir, a denúncia sobe para o admin automaticamente.' }) : null,
                Adm.dl([
                    ['Denunciante (sigiloso)', pessoa(d.denunciante, d.id_denunciante)],
                    ['Denunciado', pessoa(d.denunciado)],
                    ['Desafio', Adm.linkDesafio(d.id_desafio)],
                    ['Academia julgadora', d.id_academia_julgadora ? '#' + d.id_academia_julgadora : '—'],
                    ['Prazo da academia', Adm.dataHora(d.prazo_academia_ate)],
                    ['Encaminhada em', Adm.dataHora(d.encaminhada_em)],
                    ['Responsável', d.id_responsavel_admin ? 'Admin #' + d.id_responsavel_admin : d.id_responsavel ? 'Academia (usuário #' + d.id_responsavel + ')' : '—'],
                    ['Criada em', Adm.dataHora(d.criado_em)],
                    ['Decidida em', Adm.dataHora(d.decidida_em)],
                    ['Advertências do denunciado (90 dias)', String(det.reincidencia_advertencias_90d || 0)]
                ]),
                h('h3', { class: 'adm-h3', text: 'Relato' }),
                h('p', { class: 'adm-texto-longo', text: d.descricao }),
                d.decisao_texto ? h('h3', { class: 'adm-h3', text: 'Decisão' }) : null,
                d.decisao_texto ? h('p', { class: 'adm-texto-longo', text: d.decisao_texto }) : null,
                h('h3', { class: 'adm-h3', text: 'Evidências (' + (d.evidencias || []).length + ')' }),
                galeria(d),
                h('h3', { class: 'adm-h3', text: 'Punições desta denúncia (' + (det.punicoes || []).length + ')' }),
                (det.punicoes && det.punicoes.length) ? Adm.tabela(colunasPunicao(depois), det.punicoes, { rotulo: 'Punições da denúncia' }) : Adm.estadoVazio('Nenhuma punição.'),
                h('h3', { class: 'adm-h3', text: 'Outras denúncias contra o denunciado (' + (det.outras_do_denunciado || []).length + ')' }),
                (det.outras_do_denunciado && det.outras_do_denunciado.length) ? Adm.tabela(colunasDenuncia(true), det.outras_do_denunciado,
                    { rotulo: 'Outras denúncias', aoClicar: function(x) { Adm.abrirDenuncia(x.id); } }) : Adm.estadoVazio('Nenhuma outra.'),
                h('h3', { class: 'adm-h3', text: 'Linha do tempo' }),
                Adm.linhaTempo(det.eventos || [])
            ]);
        });
    };

    function colunasDenuncia(curta) {
        return [
            { titulo: 'ID', valor: function(d) { return '#' + d.id; } },
            { titulo: 'Categoria', valor: function(d) { return Adm.rotulo(Adm.CATEGORIAS_DENUNCIA, d.categoria); } },
            curta ? null : { titulo: 'Denunciado', valor: function(d) { return pessoa(d.denunciado); } },
            curta ? null : { titulo: 'Denunciante', valor: function(d) { return pessoa(d.denunciante, d.id_denunciante); } },
            { titulo: 'Desafio', valor: function(d) { return Adm.linkDesafio(d.id_desafio); } },
            { titulo: 'Status', valor: function(d) { return Adm.badgeStatus(Adm.STATUS_DENUNCIA, d.status); } },
            { titulo: 'Jurisdição', valor: function(d) {
                if (d.jurisdicao === 'academia') return h('div', null, ['Academia', d.prazo_academia_ate && aberta(d) ? h('small', { class: 'adm-sub adm-bloco', text: 'prazo ' + Adm.dataHora(d.prazo_academia_ate) }) : null]);
                return d.encaminhada_em ? h('div', null, ['Admin', h('small', { class: 'adm-sub adm-bloco', text: 'veio da academia' })]) : 'Admin';
            } },
            { titulo: 'Evid.', classe: 'num', valor: function(d) { return String((d.evidencias || []).length); } },
            { titulo: 'Criada', classe: 'nowrap', valor: function(d) { return Adm.dataHora(d.criado_em); } }
        ].filter(Boolean);
    }

    function colunasPunicao(aoMudar) {
        return [
            { titulo: 'ID', valor: function(p) { return '#' + p.id; } },
            { titulo: 'Usuário', valor: function(p) { return pessoa(p.usuario, p.id_usuario); } },
            { titulo: 'Tipo', valor: function(p) { return Adm.rotulo(Adm.TIPOS_PUNICAO, p.tipo); } },
            { titulo: 'Escopo', valor: function(p) { return p.escopo === 'academia' ? 'Academia #' + p.id_academia : 'Global'; } },
            { titulo: 'Período', classe: 'nowrap', valor: periodo },
            { titulo: 'Situação', valor: situacaoPunicao },
            { titulo: 'Aplicada por', valor: function(p) { return p.aplicada_por === 'academia' ? 'Academia' : 'Admin' + (p.id_aplicador_admin ? ' #' + p.id_aplicador_admin : ''); } },
            { titulo: 'Justificativa', classe: 'adm-col-texto', valor: function(p) { return p.justificativa; } },
            { titulo: 'Origem', valor: function(p) {
                return p.id_denuncia ? h('button', { type: 'button', class: 'adm-link-btn', text: 'Denúncia #' + p.id_denuncia, onClick: function() { Adm.abrirDenuncia(p.id_denuncia); } })
                    : p.id_desafio ? h('span', null, ['Desafio ', Adm.linkDesafio(p.id_desafio)]) : 'Direta';
            } },
            { titulo: 'Recurso', valor: function(p) { return p.recurso ? Adm.badgeStatus(Adm.STATUS_RECURSO, p.recurso.status) : '—'; } },
            { titulo: 'Ações', valor: function(p) {
                if (p.revogada_em || p.tipo === 'anulacao_resultado') return p.tipo === 'anulacao_resultado' && !p.revogada_em ? h('small', { class: 'adm-sub', text: 'irrevogável (só por recurso)' }) : '—';
                return Adm.botao('Revogar', function() { revogar(p, aoMudar); });
            } }
        ];
    }
    Adm.colunasPunicao = colunasPunicao;

    // ─── Telas ──────────────────────────────────────────────────────────────
    Adm.telas.denuncias = function(raiz, params) {
        var lista = Adm.lista(raiz, params, {
            tela: 'denuncias', titulo: 'Denúncias', rotulo: 'Denúncias',
            descricao: 'Fila do admin e acompanhamento das que estão com a academia (só leitura). O denunciante é sigiloso: só aparece aqui.',
            caminho: '/denuncias', padroes: { status: 'aberta,em_analise' },
            filtros: [
                { nome: 'status', rotulo: 'Status', tipo: 'select', opcoes: [['aberta,em_analise', 'Em aberto']].concat(Adm.STATUS_DENUNCIA) },
                { nome: 'jurisdicao', rotulo: 'Jurisdição', tipo: 'select', opcoes: Adm.JURISDICAO },
                { nome: 'categoria', rotulo: 'Categoria', tipo: 'select', opcoes: Adm.CATEGORIAS_DENUNCIA },
                { nome: 'id_denunciado', rotulo: 'ID denunciado', tipo: 'numero' },
                { nome: 'id_denunciante', rotulo: 'ID denunciante', tipo: 'numero' },
                { nome: 'id_desafio', rotulo: 'ID desafio', tipo: 'numero' }
            ],
            colunas: colunasDenuncia(false).concat([{ titulo: 'Ações', valor: function(d) {
                return Adm.botao('Ver', function() { Adm.abrirDenuncia(d.id, function() { lista.recarregar(); }); });
            } }]),
            aoClicar: function(d) { Adm.abrirDenuncia(d.id, function() { lista.recarregar(); }); }
        });
    };

    Adm.telas.punicoes = function(raiz, params) {
        var lista = Adm.lista(raiz, params, {
            tela: 'punicoes', titulo: 'Punições', rotulo: 'Punições',
            descricao: 'Aplicar direto (sem denúncia) ou revogar. Anulação de resultado só se aplica pelo desafio e não é revogável.',
            caminho: '/punicoes',
            botoes: h('button', { type: 'button', class: 'gs-btn gs-btn-danger adm-btn-sm', text: 'Aplicar punição', onClick: function() { Adm.aplicarPunicao({}, function() { lista.recarregar(); }); } }),
            filtros: [
                { nome: 'id_usuario', rotulo: 'ID usuário', tipo: 'numero' },
                { nome: 'tipo', rotulo: 'Tipo', tipo: 'select', opcoes: Adm.TIPOS_PUNICAO },
                { nome: 'escopo', rotulo: 'Escopo', tipo: 'select', opcoes: [['global', 'Global'], ['academia', 'Academia']] },
                { nome: 'id_academia', rotulo: 'ID academia', tipo: 'numero' },
                { nome: 'vigente', rotulo: 'Só vigentes', tipo: 'checkbox' }
            ],
            colunas: colunasPunicao(function() { lista.recarregar(); })
        });
    };

    function julgar(item, aoConcluir) {
        var r = item.recurso, p = item.punicao;
        Adm.acao({
            titulo: 'Julgar recurso #' + r.id, largo: true,
            mensagem: p.tipo === 'anulacao_resultado'
                ? 'Deferir revoga a punição no histórico do usuário; o financeiro da anulação não é desfeito.'
                : 'Deferir revoga a punição (e recalcula suspensão/banimento). Indeferir mantém tudo como está.',
            linhas: [['Usuário', p.usuario ? p.usuario.nome : '#' + p.id_usuario], ['Punição', Adm.rotulo(Adm.TIPOS_PUNICAO, p.tipo) + ' · ' + periodo(p)],
                ['Justificativa da punição', p.justificativa], ['Recurso', r.texto]],
            campos: [
                { nome: 'status', rotulo: 'Decisão', tipo: 'select', opcoes: [['', '— Escolha —'], ['deferido', 'Deferir (revoga a punição)'], ['indeferido', 'Indeferir']] },
                { nome: 'decisao_texto', rotulo: 'Texto da decisão (o usuário vê)', tipo: 'textarea', min: 3, max: 2000 }
            ],
            textoConfirmar: 'Julgar recurso',
            executar: function(x) { return Adm.post('/recursos/' + Adm.id(r.id) + '/julgar', { status: x.status, decisao_texto: x.decisao_texto, motivo: x.motivo }); },
            sucesso: 'Recurso julgado.',
            aoConcluir: aoConcluir
        });
    }

    Adm.telas.recursos = function(raiz, params) {
        var lista = Adm.lista(raiz, params, {
            tela: 'recursos', titulo: 'Recursos', rotulo: 'Recursos',
            descricao: 'Recursos dos punidos (inclui banidos, que recorrem pela tela de login). Um por punição; julgado só pelo admin.',
            caminho: '/recursos', padroes: { status: 'pendente' },
            filtros: [{ nome: 'status', rotulo: 'Status', tipo: 'select', opcoes: Adm.STATUS_RECURSO }],
            colunas: [
                { titulo: 'ID', valor: function(i) { return '#' + i.recurso.id; } },
                { titulo: 'Usuário', valor: function(i) { return pessoa(i.punicao.usuario, i.punicao.id_usuario); } },
                { titulo: 'Punição', valor: function(i) { return h('div', null, [Adm.rotulo(Adm.TIPOS_PUNICAO, i.punicao.tipo) + ' #' + i.punicao.id, h('small', { class: 'adm-sub adm-bloco', text: periodo(i.punicao) })]); } },
                { titulo: 'Situação da punição', valor: function(i) { return situacaoPunicao(i.punicao); } },
                { titulo: 'Recurso', classe: 'adm-col-texto', valor: function(i) { return i.recurso.texto; } },
                { titulo: 'Status', valor: function(i) { return Adm.badgeStatus(Adm.STATUS_RECURSO, i.recurso.status); } },
                { titulo: 'Decisão', classe: 'adm-col-texto', valor: function(i) { return i.recurso.decisao_texto; } },
                { titulo: 'Aberto em', classe: 'nowrap', valor: function(i) { return Adm.dataHora(i.recurso.criado_em); } },
                { titulo: 'Ações', valor: function(i) {
                    return i.recurso.status === 'pendente' ? Adm.botao('Julgar', function() { julgar(i, function() { lista.recarregar(); }); }, 'gs-btn-primary') : '—';
                } }
            ]
        });
    };

    Adm.telas.pagamentos = function(raiz, params) {
        Adm.lista(raiz, params, {
            tela: 'pagamentos', titulo: 'Pagamentos pendentes', rotulo: 'Pagamentos pendentes do pódio',
            descricao: 'Créditos do pódio na janela de contestação. Retido = denúncia aberta sobre o desafio. O job libera os vencidos.',
            caminho: '/pagamentos-pendentes', padroes: { status: 'a_liberar,retido' },
            filtros: [
                { nome: 'status', rotulo: 'Status', tipo: 'select', opcoes: [['a_liberar,retido', 'A liberar + retidos']].concat(Adm.STATUS_PENDENTE) },
                { nome: 'tipo', rotulo: 'Tipo', tipo: 'select', opcoes: Adm.TIPOS_PENDENTE },
                { nome: 'id_desafio', rotulo: 'ID desafio', tipo: 'numero' },
                { nome: 'id_usuario', rotulo: 'ID usuário', tipo: 'numero' }
            ],
            colunas: [
                { titulo: 'ID', valor: function(p) { return '#' + p.id; } },
                { titulo: 'Desafio', valor: function(p) { return h('div', null, [Adm.linkDesafio(p.id_desafio), h('small', { class: 'adm-sub adm-bloco', text: p.titulo_desafio || '' })]); } },
                { titulo: 'Beneficiário', valor: function(p) { return p.id_usuario ? pessoa(p.usuario, p.id_usuario) : 'Plataforma'; } },
                { titulo: 'Tipo', valor: function(p) { return Adm.rotulo(Adm.TIPOS_PENDENTE, p.tipo); } },
                { titulo: 'Valor', classe: 'num', valor: function(p) { return Adm.reais(p.valor_centavos); } },
                { titulo: 'Status', valor: function(p) { return Adm.badgeStatus(Adm.STATUS_PENDENTE, p.status); } },
                { titulo: 'Libera em', classe: 'nowrap', valor: function(p) { return Adm.dataHora(p.liberar_em); } },
                { titulo: 'Liberado em', classe: 'nowrap', valor: function(p) { return Adm.dataHora(p.liberado_em); } },
                { titulo: 'Retido por', valor: function(p) {
                    return p.id_denuncia_retencao ? h('button', { type: 'button', class: 'adm-link-btn', text: 'Denúncia #' + p.id_denuncia_retencao, onClick: function() { Adm.abrirDenuncia(p.id_denuncia_retencao); } }) : '—';
                } },
                { titulo: 'Transação', valor: function(p) { return Adm.linkTransacao(p.id_transacao); } }
            ]
        });
    };

    Adm.telas.debitos = function(raiz, params) {
        Adm.lista(raiz, params, {
            tela: 'debitos', titulo: 'Débitos', rotulo: 'Débitos de usuários',
            descricao: 'O que a anulação de resultado não conseguiu estornar do saldo. Débito aberto bloqueia o saque; créditos novos abatem automaticamente.',
            caminho: '/debitos', padroes: { aberto: 'true' },
            filtros: [
                { nome: 'aberto', rotulo: 'Situação', tipo: 'select', opcoes: [['true', 'Em aberto'], ['false', 'Quitados']] },
                { nome: 'id_usuario', rotulo: 'ID usuário', tipo: 'numero' }
            ],
            colunas: [
                { titulo: 'ID', valor: function(d) { return '#' + d.id; } },
                { titulo: 'Usuário', valor: function(d) { return pessoa(d.usuario, d.id_usuario); } },
                { titulo: 'Desafio', valor: function(d) { return h('div', null, [Adm.linkDesafio(d.id_desafio), h('small', { class: 'adm-sub adm-bloco', text: d.titulo_desafio || '' })]); } },
                { titulo: 'Valor', classe: 'num', valor: function(d) { return Adm.reais(d.valor_centavos); } },
                { titulo: 'Restante', classe: 'num', valor: function(d) { return Adm.reais(d.saldo_restante_centavos); } },
                { titulo: 'Situação', valor: function(d) { return d.quitado ? Adm.badge('Quitado', 'quitado') : Adm.badge('Em aberto', 'em_aberto'); } },
                { titulo: 'Motivo', classe: 'adm-col-texto', valor: function(d) { return d.motivo; } },
                { titulo: 'Criado', classe: 'nowrap', valor: function(d) { return Adm.dataHora(d.criado_em); } },
                { titulo: 'Quitado em', classe: 'nowrap', valor: function(d) { return Adm.dataHora(d.quitado_em); } }
            ]
        });
    };
})();
