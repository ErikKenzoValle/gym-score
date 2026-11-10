// GymScore — blocos compartilhados das telas de gestão (academia e instrutor).
// Depende de js/api.js (h, limpar, API, Auth, Modal, Mask, estadoVazio, Skeleton, mensagemErro...)
// e de css/gestao.css. Nada de innerHTML com dado de usuário: tudo via h()/textContent.

var Gestao = {
    ROTULO_STATUS: { aberto: 'Aberto', em_andamento: 'Em andamento', encerrado: 'Encerrado', cancelado: 'Cancelado' },
    MEDALHAS: ['🥇', '🥈', '🥉', '🏅', '🏅'],

    // Biblioteca de QR de fallback (cdnjs, versão fixa + SRI) para navegadores sem BarcodeDetector
    LIB_QR: {
        url: 'https://cdnjs.cloudflare.com/ajax/libs/html5-qrcode/2.3.8/html5-qrcode.min.js',
        sri: 'sha512-r6rDA7W6ZeQhvl8S7yRVQUKVHdexq+GAlNkNNqVC7YyIV+NwqCTJe2hDWCiffTyRNOeGEzRRJ9ifvRm/HCzGYg=='
    },

    param: function(nome) { return new URLSearchParams(window.location.search).get(nome); },

    // Garante login e o papel da tela ('academia' ou 'instrutor'), decidindo pelo /usuarios/me (não pelo
    // cache gym_user, que só serve para exibição). Rodada 4: instrutor é MODO de uma conta de atleta
    // (Modo.atual), nunca perfil. Resolve com o usuário ou null:
    //  - conta de outro tipo / atleta sem pedido de instrutor → vai para a própria home (ou ao cadastro);
    //  - pedido de instrutor em análise/recusado → aviso do pedido (a conta de atleta segue normal);
    //  - instrutor aprovado no modo atleta → oferece a troca de modo (sem deslogar).
    exigirPerfil: async function(perfil) {
        if (!Auth.require()) return null;
        var u;
        try { u = await Auth.verificar(); }
        catch (e) { showToast('Não foi possível confirmar sua sessão. ' + mensagemErro(e), 'error'); return null; }
        if (!u) return null; // 401: _req já redirecionou para o login
        var modo = Modo.atual(u);
        if (modo === perfil) {
            document.body.dataset.perfil = perfil;
            return u;
        }
        if (perfil === 'instrutor' && u.perfil === 'atleta') {
            var st = Modo.pedidoInstrutor(u);
            if (!st) { window.location.href = '/cadastro-instrutor'; return null; }
            renderBottomNav(location.pathname);
            if (st !== 'aprovada') { CadastroStatus.mostrar(st, 'instrutor'); return null; }
            Gestao._oferecerModoInstrutor();
            return null;
        }
        window.location.href = Modo.home(modo);
        return null;
    },

    // Tela do modo instrutor aberta no modo atleta (instrutor aprovado): esconde o conteúdo e oferece a troca
    _oferecerModoInstrutor: function() {
        var raiz = document.querySelector('main') || document.body;
        Array.prototype.forEach.call(raiz.children, function(c) { c.hidden = true; });
        var btn = h('button', { type: 'button', class: 'gs-btn gs-btn-primary', text: 'Trocar para o modo instrutor' });
        btn.addEventListener('click', async function() {
            btnLoading(btn, true);
            try { await Modo.trocar('instrutor', { destino: location.pathname + location.search }); }
            catch (e) { btnLoading(btn, false); showToast(mensagemErro(e), 'error'); }
        });
        raiz.insertBefore(h('section', { class: 'gs-card gs-cadastro-status', role: 'status' }, [
            h('span', { class: 'gs-cadastro-icone', 'aria-hidden': 'true', text: '⇄' }),
            h('h1', { text: 'Você está no modo atleta' }),
            h('p', { text: 'Esta tela é do modo instrutor. Troque de modo para continuar; você não precisa entrar de novo.' }),
            h('div', { class: 'gs-cadastro-acoes' }, [btn, h('a', { class: 'gs-btn gs-btn-secondary', href: '/menu', text: 'Continuar como atleta' })])
        ]), raiz.firstChild);
    },

    // ─── Vínculo do instrutor com academia (convite / desvínculo) ────────────
    // Monta em `el` o convite pendente (aceitar/recusar) ou o vínculo atual (desvincular).
    // opts.soConvite: não mostra nada além do convite (tela de cadastro pendente).
    vinculo: function(el, opts) {
        opts = opts || {};
        async function acao(btn, fn, msgOk) {
            btnLoading(btn, true);
            try {
                await fn();
                showToast(msgOk, 'success');
                carregar();
            } catch (e) {
                btnLoading(btn, false);
                showToast(mensagemErro(e), 'error');
            }
        }
        async function carregar() {
            var data;
            try { data = (await API.meuConvite()).data || {}; }
            catch (e) { limpar(el); return; }
            limpar(el);
            var c = data.convite;
            if (c) {
                var nome = c.nome_fantasia || 'Uma academia';
                var bAceitar = h('button', { type: 'button', class: 'gs-btn gs-btn-primary', text: 'Aceitar' });
                var bRecusar = h('button', { type: 'button', class: 'gs-btn gs-btn-secondary', text: 'Recusar' });
                bAceitar.addEventListener('click', async function() {
                    var ok = await Modal.confirmar({ titulo: 'Aceitar convite', mensagem: 'Você passa a ser instrutor vinculado a ' + nome + '.' +
                        (data.id_academia ? ' O vínculo atual é substituído.' : ''), confirmar: 'Aceitar' });
                    if (ok) acao(bAceitar, API.aceitarConvite, 'Convite aceito.');
                });
                bRecusar.addEventListener('click', async function() {
                    var ok = await Modal.confirmar({ titulo: 'Recusar convite', mensagem: 'A academia ' + nome + ' será avisada.', confirmar: 'Recusar', perigo: true });
                    if (ok) acao(bRecusar, API.recusarConvite, 'Convite recusado.');
                });
                el.appendChild(h('section', { class: 'gs-card gs-vinculo convite', 'aria-label': 'Convite de academia' }, [
                    h('strong', { text: 'Convite de vínculo' }),
                    h('p', null, [h('b', { text: nome }), ' convidou você para ser instrutor vinculado' + (c.convite_em ? ' (' + fmtData(c.convite_em) + ')' : '') + '.']),
                    h('div', { class: 'gs-vinculo-acoes' }, [bAceitar, bRecusar])
                ]));
                return;
            }
            if (opts.soConvite) return;
            if (data.id_academia) {
                var bSair = h('button', { type: 'button', class: 'gs-btn gs-btn-secondary', text: 'Desvincular da academia' });
                bSair.addEventListener('click', async function() {
                    var ok = await Modal.confirmar({ titulo: 'Sair da academia', perigo: true, confirmar: 'Desvincular',
                        mensagem: 'Você passa a atuar como autônomo. Não é possível sair enquanto estiver designado para um desafio ativo.' });
                    if (ok) acao(bSair, API.sairDaAcademia, 'Você não está mais vinculado à academia.');
                });
                el.appendChild(h('section', { class: 'gs-card gs-vinculo', 'aria-label': 'Vínculo com academia' }, [
                    h('p', { text: 'Você é instrutor vinculado a uma academia.' }),
                    h('div', { class: 'gs-vinculo-acoes' }, [bSair])
                ]));
            } else {
                el.appendChild(h('p', { class: 'gs-hint gs-vinculo', text: 'Você atua como autônomo. Academias podem convidar você pelo seu e-mail.' }));
            }
        }
        carregar();
    },

    // Erro de carregamento com botão de tentar de novo (no lugar do conteúdo)
    erro: function(el, e, tentar) {
        Skeleton.fim(el);
        limpar(el).appendChild(estadoVazio({
            erro: true, titulo: 'Não foi possível carregar', texto: mensagemErro(e),
            acao: tentar ? { texto: 'Tentar de novo', onClick: tentar } : null
        }));
    },

    // ─── Formatação ──────────────────────────────────────────────────────────
    selo: function(status) {
        return h('span', { class: 'gx-status ' + status, text: Gestao.ROTULO_STATUS[status] || status });
    },
    quando: function(raw) {
        if (!raw) return 'Data a definir';
        var d = new Date(raw);
        if (isNaN(d.getTime())) return 'Data a definir';
        return d.toLocaleDateString('pt-BR', { weekday: 'short', day: '2-digit', month: 'short' }) + ' · ' +
            d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
    },
    // Rodada 5: início–fim do evento ("sáb., 10 de dez. · 08:00–10:00"; fim em outro dia mostra a data dele)
    horario: function(d) {
        var ini = d && d.data_evento ? new Date(d.data_evento) : null;
        if (!ini || isNaN(ini.getTime())) return 'Data a definir';
        var fim = d.data_fim_evento ? new Date(d.data_fim_evento) : null;
        var txt = Gestao.quando(d.data_evento);
        if (!fim || isNaN(fim.getTime())) return txt;
        var hora = fim.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
        return txt + '–' + (fim.toDateString() === ini.toDateString() ? hora
            : fim.toLocaleDateString('pt-BR', { day: '2-digit', month: 'short' }) + ' ' + hora);
    },
    // Endereço do evento em linhas de texto (estruturado; cai no `local` nos desafios antigos)
    enderecoLinhas: function(e, local) {
        if (!e || !(e.logradouro || e.cidade || e.cep)) return local ? [local] : [];
        var cep = e.cep && /^\d{8}$/.test(e.cep) ? e.cep.slice(0, 5) + '-' + e.cep.slice(5) : e.cep;
        var rua = [e.logradouro, e.numero].filter(Boolean).join(', ') + (e.complemento ? ' — ' + e.complemento : '');
        return [rua, [e.bairro, [e.cidade, e.uf].filter(Boolean).join('/')].filter(Boolean).join(' · '), cep ? 'CEP ' + cep : ''].filter(Boolean);
    },
    // Endereço curto para cartões: logradouro, número · bairro (ou o `local` antigo)
    enderecoCurto: function(d) {
        var e = d && d.endereco;
        if (!e || !e.logradouro) return d && d.local ? d.local : '';
        return [e.logradouro, e.numero].filter(Boolean).join(', ') + (e.bairro ? ' · ' + e.bairro : '');
    },
    // Condições de pagamento POR instrutor (rodada 5): "R$ 50,00 + 5% das inscrições"
    MODOS_PAGAMENTO: { fixo: 'Valor fixo', percentual: 'Percentual das inscrições', fixo_percentual: 'Fixo + percentual' },
    textoCondicoes: function(modo, fixoCentavos, pct) {
        var partes = [];
        if (modo !== 'percentual') partes.push(fmtBRL(fixoCentavos || 0));
        if (modo !== 'fixo') partes.push((pct || 0) + '% das inscrições');
        return partes.join(' + ') + ' por instrutor';
    },
    ROTULO_CANDIDATURA: { pendente: 'Pendente', escolhida: 'Na equipe', recusada: 'Recusada', retirada: 'Retirada', removida: 'Removida' },
    ROTULO_ORIGEM: { indicado: 'Indicado pela academia', candidatura: 'Por candidatura' },
    nome: function(p) { return nomeCompleto(p); },
    // "20 de 100 vagas" + barra
    barraVagas: function(d) {
        var vagas = d.vagas || 0, n = d.participantes_count || 0;
        var pct = vagas > 0 ? Math.min(100, Math.round(n / vagas * 100)) : 0;
        return h('div', null, [
            h('div', { class: 'gx-vagas' }, [h('span', { text: n + ' de ' + vagas + ' inscritos' }), h('span', { text: pct + '%' })]),
            h('div', { class: 'gs-progresso', role: 'progressbar', 'aria-valuemin': '0', 'aria-valuemax': String(vagas), 'aria-valuenow': String(n),
                'aria-label': 'Vagas preenchidas' }, h('span', { style: { width: pct + '%' } }))
        ]);
    },
    // Linha sobre a equipe de instrutores (rodada 5): tamanho, responsável e candidatos pendentes
    textoInstrutor: function(d) {
        var equipe = d.equipe || [], vagas = d.instrutores_vagas || 1;
        var resp = equipe.filter(function(m) { return m.responsavel; })[0] || d.instrutor;
        var partes = ['Equipe ' + equipe.length + '/' + vagas];
        if (resp) partes.push('responsável: ' + resp.nome);
        else if (!equipe.length) partes.push('sem instrutor');
        var n = d.financeiro ? d.financeiro.candidaturas_pendentes : 0;
        if (n && d.status === 'aberto') partes.push(n + (n === 1 ? ' candidato' : ' candidatos'));
        return partes.join(' · ');
    },

    // Equipe de instrutores em lista (responsável primeiro, como vem da API).
    // opts.acoes(membro) -> [Node] botões por membro; opts.voce: id_usuario do logado (marca "você").
    equipe: function(d, opts) {
        opts = opts || {};
        var equipe = d.equipe || [];
        var vagas = d.instrutores_vagas || 1;
        var livres = Math.max(0, vagas - equipe.length);
        var box = h('div', { class: 'gx-equipe', role: 'list', 'aria-label': 'Equipe de instrutores' });
        equipe.forEach(function(m) {
            var acoes = opts.acoes ? opts.acoes(m) : null;
            box.appendChild(h('div', { class: 'gx-atleta', role: 'listitem' }, [
                h('div', { class: 'gx-avatar', 'aria-hidden': 'true', text: (m.nome || '?').charAt(0).toUpperCase() }),
                h('div', { class: 'gx-atleta-info' }, [
                    h('div', { class: 'gx-atleta-nome', text: m.nome + (opts.voce && m.id_usuario === opts.voce ? ' (você)' : '') }),
                    h('div', { class: 'gx-atleta-sub', text: (Gestao.ROTULO_ORIGEM[m.origem] || m.origem) + (m.vinculado ? ' · vinculado' : ' · autônomo') })
                ]),
                m.responsavel ? h('span', { class: 'gx-selo-ok', text: 'Responsável' }) : null,
                acoes && acoes.length ? h('div', { class: 'gx-equipe-acoes' }, acoes) : null
            ]));
        });
        if (!equipe.length) box.appendChild(h('p', { class: 'gx-texto', text: 'Ninguém na equipe ainda.' }));
        if (livres && (d.status === 'aberto')) {
            box.appendChild(h('p', { class: 'gx-ajuda', text: livres + (livres === 1 ? ' vaga aberta' : ' vagas abertas') + ' para candidatura de instrutores.' }));
        }
        return box;
    },

    // Mesma divisão do backend (services/pote.go, rodada 6): fatias em pontos-base (10000 = 100%) por
    // posição, renormalizadas para as posições ocupadas; os centavos que sobram vão para o 1º.
    TABELA_PODIO: { 1: [100], 2: [70, 30], 3: [60, 30, 10], 4: [50, 25, 15, 10], 5: [40, 25, 15, 12, 8] },
    // Divisão padrão (= DivisaoPadraoBP do backend): tabela antiga até 5; acima, pesos lineares n..1.
    divisaoPadraoBP: function(n) {
        n = Math.max(1, Math.min(100, n || 1));
        if (Gestao.TABELA_PODIO[n]) return Gestao.TABELA_PODIO[n].map(function(p) { return p * 100; });
        var soma = n * (n + 1) / 2, out = [], dist = 0;
        for (var i = 0; i < n; i++) { out.push(Math.floor(10000 * (n - i) / soma)); dist += out[i]; }
        out[0] += 10000 - dist;
        return out;
    },
    // divisao: [pontos-base] do desafio (ou, em telas antigas, o nº de posições → divisão padrão)
    dividirPremio: function(total, divisao, presentes) {
        var bp = Array.isArray(divisao) && divisao.length ? divisao : Gestao.divisaoPadraoBP(divisao);
        presentes = Math.min(presentes, bp.length);
        if (presentes <= 0 || total < 0) return [];
        var fatias = bp.slice(0, presentes);
        var soma = fatias.reduce(function(a, b) { return a + b; }, 0);
        if (soma <= 0) return [];
        var premios = fatias.map(function(p) { return Math.floor(total * p / soma); });
        premios[0] += total - premios.reduce(function(a, b) { return a + b; }, 0);
        return premios;
    },
    // 6000 → "60%", 3333 → "33,33%"
    pctBP: function(bp) {
        var s = (bp / 100).toFixed(2).replace('.', ',').replace(/,00$/, '').replace(/(,\d)0$/, '$1');
        return s + '%';
    },
    medalha: function(posicao) { return Gestao.MEDALHAS[posicao - 1] || '🏅'; },

    // ─── Cartão de desafio (dashboard, lista da academia, agenda do instrutor) ──
    // acoes: [{texto, href | onClick, classe?: 'contorno' (padrão) | 'perigo' | '' (cheio)}]
    cardDesafio: function(d, acoes) {
        var premio = d.premiacao ? d.premiacao.premio_total_centavos : 0;
        return h('article', { class: 'gx-card', 'aria-label': d.titulo }, [
            h('div', { class: 'gx-linha' }, [h('h3', { text: d.titulo }), Gestao.selo(d.status)]),
            h('div', { class: 'gx-meta' }, [
                h('span', { text: Gestao.horario(d) }),
                Gestao.enderecoCurto(d) ? h('span', { text: Gestao.enderecoCurto(d) }) : null,
                d.categoria ? h('span', { text: d.categoria }) : null
            ]),
            h('div', { class: 'gx-meta' }, [
                h('span', { text: Gestao.textoInstrutor(d) }),
                h('span', { text: 'Prêmio: ' + fmtBRL(premio) })
            ]),
            Gestao.barraVagas(d),
            (acoes && acoes.length) ? h('div', { class: 'gx-acoes' }, acoes.map(function(a) {
                var attrs = { class: 'gx-btn pequeno ' + (a.classe === undefined ? 'contorno' : a.classe) }; // classe '' = botão cheio
                if (a.href) { attrs.href = a.href; return h('a', attrs, a.texto); }
                attrs.type = 'button';
                attrs.onClick = a.onClick;
                return h('button', attrs, a.texto);
            })) : null
        ]);
    },

    // Lista de pares rótulo/valor (<dl>); item: [rotulo, valor, classe?]
    lista: function(itens, classe) {
        return h('dl', { class: 'gx-sim-lista ' + (classe || '') }, itens.filter(Boolean).map(function(i) {
            return h('div', { class: i[2] || '' }, [h('dt', { text: i[0] }), h('dd', { text: i[1] })]);
        }));
    },

    // Quadro financeiro de um desafio (campo `financeiro`, só a academia dona recebe)
    quadroFinanceiro: function(f) {
        return Gestao.lista([
            ['Arrecadação das inscrições', fmtBRL(f.arrecadacao_centavos)],
            ['Prêmio fixo', fmtBRL(f.premio_fixo_centavos)],
            f.dividir_pote ? ['Parte do pote para atletas (' + f.percentual_pote + '%)', fmtBRL(f.pote_premio_centavos)] : null,
            ['Prêmio total dos atletas', fmtBRL(f.premio_total_centavos), 'total'],
            ['Taxa da plataforma (' + f.taxa_plataforma_pct + '%)', fmtBRL(f.taxa_plataforma_centavos)],
            ['Instrutores (' + (f.instrutores_pagos || 0) + ' × ' + Gestao.textoCondicoes(f.instrutor_modo_pagamento, f.instrutor_fixo_centavos, f.instrutor_pct).replace(' por instrutor', '') + ')',
                fmtBRL(f.instrutor_total_centavos)],
            f.instrutores_vagas ? ['Equipe', (f.equipe_count || 0) + ' de ' + f.instrutores_vagas + (f.instrutores_vagas === 1 ? ' instrutor' : ' instrutores')] : null,
            ['Você recebe de volta', fmtBRL(f.liquido_previsto_centavos), 'total'],
            ['Reservado do seu saldo', fmtBRL(f.reserva_centavos)],
            ['Resultado', fmtBRL(f.resultado_previsto_centavos), f.resultado_previsto_centavos < 0 ? 'neg' : 'pos']
        ]);
    },

    // ─── Presença: contador, leitor de QR, código digitado e lista ───────────
    // Monta em `raiz` a chamada do desafio `id`. opts.aoMudar(desafio) é chamado a cada mudança.
    presenca: function(raiz, d, opts) {
        opts = opts || {};
        var participantes = (d.participantes || []).slice();
        var contador = h('div', { class: 'gx-contador', 'aria-live': 'polite' });
        var feedback = h('div', { 'aria-live': 'polite' });
        var lista = h('div', { 'aria-label': 'Lista de atletas inscritos' });
        var areaLeitor = h('div', { hidden: true });
        var leitor = null;

        function presentes() { return participantes.filter(function(p) { return p.presente; }).length; }

        function mostrar(tipo, texto) {
            limpar(feedback).appendChild(h('div', { class: 'gx-confirma ' + tipo, role: tipo === 'erro' ? 'alert' : 'status', text: texto }));
        }

        function desenhar() {
            var ok = presentes(), total = participantes.length;
            contador.textContent = 'Presentes: ' + ok + ' de ' + total;
            limpar(lista);
            if (!total) {
                lista.appendChild(estadoVazio({ icone: '👥', titulo: 'Nenhum atleta inscrito', texto: 'Quando os atletas se inscreverem, eles aparecem aqui.' }));
                return;
            }
            participantes.forEach(function(p) {
                var nome = Gestao.nome(p);
                lista.appendChild(h('div', { class: 'gx-atleta' }, [
                    h('span', { class: 'gx-ponto' + (p.presente ? ' ok' : ''), 'aria-hidden': 'true' }),
                    h('div', { class: 'gx-atleta-info' }, [
                        h('div', { class: 'gx-atleta-nome', text: nome }),
                        p.presente && p.presente_em ? h('div', { class: 'gx-atleta-sub', text: 'Chegou às ' + new Date(p.presente_em).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' }) }) : null
                    ]),
                    p.presente
                        ? h('span', { class: 'gx-selo-ok', text: 'Presente' })
                        : (d.pode_marcar_presenca
                            ? h('button', { type: 'button', class: 'gx-btn pequeno contorno', 'aria-label': 'Marcar presença de ' + nome,
                                onClick: function(ev) { registrar(false, p.id, ev.currentTarget); }, text: 'Marcar' })
                            : h('span', { class: 'gx-selo-esp', text: 'Aguardando' }))
                ]));
            });
        }

        async function registrar(porCodigo, valor, btn) {
            if (btn) btn.disabled = true;
            try {
                var resp = await API.marcarPresencaPor(d.id, porCodigo, valor);
                var r = resp.data || {};
                participantes.forEach(function(p) {
                    if (p.id === r.id_usuario) { p.presente = true; p.presente_em = r.presente_em; }
                });
                desenhar();
                var nome = nomeCompleto(r);
                var placar = r.presentes_count + ' de ' + r.participantes_count + ' presentes';
                mostrar(r.ja_presente ? 'ja' : 'ok', nome + (r.ja_presente ? ' já estava presente' : ' confirmado(a)') + ' · ' + placar);
                if (opts.aoMudar && !r.ja_presente) opts.aoMudar(r);
            } catch (e) {
                mostrar('erro', mensagemErro(e));
                if (btn) btn.disabled = false;
            }
        }

        // ── QR por câmera
        var btnLeitor = h('button', { type: 'button', class: 'gx-btn cheio', onClick: alternarLeitor }, 'Escanear QR do ingresso');
        async function alternarLeitor() {
            if (leitor) { pararLeitor(); return; }
            btnLeitor.disabled = true;
            areaLeitor.hidden = false;
            try {
                leitor = await Gestao.iniciarLeitor(areaLeitor, function(codigo) {
                    if (!codigo) { mostrar('erro', 'QR inválido: não é um ingresso do GymScore.'); return null; }
                    return registrar(true, codigo);
                });
                btnLeitor.textContent = 'Parar câmera';
            } catch (e) {
                areaLeitor.hidden = true;
                mostrar('erro', mensagemErro(e) + ' Digite o código do ingresso ou marque na lista.');
            }
            btnLeitor.disabled = false;
        }
        function pararLeitor() {
            if (leitor) leitor.parar();
            leitor = null;
            limpar(areaLeitor);
            areaLeitor.hidden = true;
            btnLeitor.textContent = 'Escanear QR do ingresso';
        }

        // ── Código digitado (32 caracteres hex do QR)
        var campoCodigo = h('input', { type: 'text', id: 'gxCodigo', class: 'gs-input', maxlength: '40', autocomplete: 'off',
            autocapitalize: 'off', spellcheck: 'false', placeholder: 'Código do ingresso (32 caracteres)' });
        var formCodigo = h('form', { class: 'gx-codigo', novalidate: true, onSubmit: function(ev) {
            ev.preventDefault();
            var m = /^[0-9a-f]{32}$/i.exec(campoCodigo.value.trim());
            if (!m) { erroCampo(campoCodigo, 'O código tem 32 letras e números (0-9, a-f).'); return; }
            erroCampo(campoCodigo, '');
            registrar(true, m[0].toLowerCase()).then(function() { campoCodigo.value = ''; });
        } }, [h('div', { style: { flex: '1', minWidth: '0' } }, campoCodigo), h('button', { type: 'submit', class: 'gx-btn contorno' }, 'Confirmar')]);

        var cabecalho = [h('div', { class: 'gx-card' }, [contador, h('p', { class: 'gx-texto', text: 'Confira o nome na tela antes de liberar o atleta.' })])];
        if (d.pode_marcar_presenca) {
            cabecalho.push(h('div', { class: 'gx-card' }, [
                h('div', { class: 'gx-card-titulo', text: 'Ler QR do ingresso' }),
                btnLeitor, h('div', { style: { height: '12px' } }), areaLeitor,
                h('label', { class: 'gs-label', for: 'gxCodigo', text: 'Ou digite o código' }), formCodigo
            ]));
        } else {
            var motivo = d.status === 'aberto'
                ? 'A chamada abre no dia do evento ou depois que o desafio for iniciado.'
                : 'A chamada está encerrada para este desafio.';
            cabecalho.push(h('div', { class: 'gx-card' }, h('p', { class: 'gx-texto', text: motivo })));
        }
        limpar(raiz);
        [cabecalho, feedback, h('div', { class: 'gx-card' }, [h('div', { class: 'gx-card-titulo', text: 'Lista de atletas' }), lista])].forEach(function(b) {
            (Array.isArray(b) ? b : [b]).forEach(function(n) { raiz.appendChild(n); });
        });
        desenhar();
        return { parar: pararLeitor };
    },

    // ─── Leitor de QR ─────────────────────────────────────────────────────────
    // Tenta BarcodeDetector (câmera traseira); sem ele carrega html5-qrcode do cdnjs (versão fixa + SRI).
    // aoLer(codigo|null, textoBruto): codigo = 32 hex em minúsculas, ou null se o QR não for de ingresso.
    // A mesma leitura em menos de 4 s é ignorada (a câmera lê o mesmo QR várias vezes por segundo).
    // Devolve {parar}. Lança Error com mensagem amigável se não houver câmera/permissão.
    iniciarLeitor: async function(area, aoLer) {
        var ultimo = { texto: '', t: 0 }, ocupado = false, timer = null, stream = null, lib = null;

        function entregar(texto) {
            var agora = Date.now();
            if (ocupado || (ultimo.texto === texto && agora - ultimo.t < 4000)) return;
            ultimo = { texto: texto, t: agora };
            ocupado = true;
            var m = /[0-9a-f]{32}/i.exec(texto || '');
            Promise.resolve(aoLer(m ? m[0].toLowerCase() : null, texto)).then(function() { ocupado = false; }, function() { ocupado = false; });
        }
        function parar() {
            if (timer) clearInterval(timer);
            timer = null;
            if (stream) stream.getTracks().forEach(function(t) { t.stop(); });
            stream = null;
            if (lib) { var l = lib; lib = null; l.stop().then(function() { l.clear(); }, function() {}); }
        }
        function erroCamera(e) {
            var nome = e && e.name;
            if (nome === 'NotAllowedError' || nome === 'SecurityError') return new Error('Permita o acesso à câmera no navegador.');
            if (nome === 'NotFoundError' || nome === 'OverconstrainedError') return new Error('Nenhuma câmera encontrada neste aparelho.');
            return new Error('Não foi possível abrir a câmera.');
        }

        if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
            throw new Error('A câmera só funciona em conexão segura (HTTPS).');
        }

        if ('BarcodeDetector' in window) {
            var detector = null;
            try {
                var formatos = BarcodeDetector.getSupportedFormats ? await BarcodeDetector.getSupportedFormats() : ['qr_code'];
                if (formatos.indexOf('qr_code') >= 0) detector = new BarcodeDetector({ formats: ['qr_code'] });
            } catch (e) { detector = null; }
            if (detector) {
                try {
                    stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: { ideal: 'environment' } }, audio: false });
                } catch (e) { throw erroCamera(e); }
                var video = h('video', { playsinline: true, muted: true, autoplay: true, 'aria-label': 'Câmera lendo o QR do ingresso' });
                video.muted = true;
                video.srcObject = stream;
                area.appendChild(h('div', { class: 'gx-scan' }, [video, h('div', { class: 'gx-scan-mira', 'aria-hidden': 'true' })]));
                try { await video.play(); } catch (e) { /* autoplay já cuida */ }
                timer = setInterval(async function() {
                    if (video.readyState < 2) return;
                    try {
                        var achados = await detector.detect(video);
                        if (achados.length) entregar(achados[0].rawValue);
                    } catch (e) { /* próximo quadro */ }
                }, 250);
                return { parar: parar };
            }
        }

        // Fallback: biblioteca de QR com versão fixa e integridade verificada
        await Gestao.carregarLibQR();
        var alvo = h('div', { id: 'gxLeitorLib' });
        area.appendChild(h('div', { class: 'gx-scan' }, alvo));
        try {
            lib = new Html5Qrcode('gxLeitorLib');
            await lib.start({ facingMode: 'environment' }, { fps: 10, qrbox: { width: 220, height: 220 } },
                function(texto) { entregar(texto); }, function() { /* quadro sem QR */ });
        } catch (e) {
            lib = null;
            throw erroCamera(e);
        }
        return { parar: parar };
    },

    carregarLibQR: function() {
        if (window.Html5Qrcode) return Promise.resolve();
        if (Gestao._libQR) return Gestao._libQR;
        Gestao._libQR = new Promise(function(resolve, reject) {
            var s = document.createElement('script');
            s.src = Gestao.LIB_QR.url;
            s.integrity = Gestao.LIB_QR.sri;
            s.crossOrigin = 'anonymous';
            s.referrerPolicy = 'no-referrer';
            s.onload = function() { resolve(); };
            s.onerror = function() { Gestao._libQR = null; reject(new Error('Não foi possível carregar o leitor de QR. Verifique a conexão.')); };
            document.head.appendChild(s);
        });
        return Gestao._libQR;
    },

    // ─── Pódio: só presentes, N posições, marca obrigatória, prévia e confirmação ──
    // d = DesafioResponse com participantes. opts.aoFim(desafioEncerrado).
    podio: function(raiz, d, opts) {
        opts = opts || {};
        limpar(raiz);
        var presentes = (d.participantes || []).filter(function(p) { return p.presente; });
        if (d.status === 'encerrado') {
            raiz.appendChild(Gestao.podioFinal(d));
            return;
        }
        if (!d.pode_lancar_podio && d.aguardando_relancamento) {
            raiz.appendChild(estadoVazio({ icone: '⚖️', titulo: 'Resultado anulado',
                texto: 'A moderação anulou o resultado. Só a academia dona do desafio pode lançar o novo pódio.' }));
            return;
        }
        if (!d.pode_lancar_podio && d.voce_na_equipe && !d.voce_responsavel && d.status === 'em_andamento') {
            var resp = (d.equipe || []).filter(function(m) { return m.responsavel; })[0];
            raiz.appendChild(estadoVazio({ icone: '🏁', titulo: 'Só o responsável lança o pódio',
                texto: (resp ? resp.nome + ' é o responsável' : 'A academia ainda não definiu o responsável') + ' por este desafio. Você continua podendo fazer a chamada.' }));
            return;
        }
        if (!d.pode_lancar_podio) {
            raiz.appendChild(estadoVazio({
                icone: '🏁',
                titulo: d.status === 'aberto' ? 'Inicie o desafio primeiro' : 'Pódio indisponível',
                texto: d.status === 'aberto' ? 'O pódio só pode ser lançado com o desafio em andamento e ao menos um atleta presente.'
                    : (presentes.length ? 'Este desafio não aceita lançamento de pódio agora.' : 'Nenhum atleta presente. Faça a chamada antes de lançar o pódio.')
            }));
            return;
        }
        var n = Math.min(d.posicoes_podio, presentes.length);
        var total = d.premiacao ? d.premiacao.premio_total_centavos : 0;
        var premios = Gestao.dividirPremio(total, d.divisao_premio_bp && d.divisao_premio_bp.length ? d.divisao_premio_bp : d.posicoes_podio, n);
        var selects = [], marcas = [];

        raiz.appendChild(h('div', { class: 'gx-card destaque' }, h('p', { class: 'gx-texto' },
            n < d.posicoes_podio
                ? 'Só ' + presentes.length + (presentes.length === 1 ? ' atleta está presente' : ' atletas estão presentes') + ': o pódio terá ' + n +
                  (n === 1 ? ' posição' : ' posições') + ' e os prêmios são recalculados entre elas.'
                : 'Escolha os ' + n + (n === 1 ? ' colocado' : ' primeiros colocados') + ' entre os presentes e informe a marca de cada um (tempo, carga ou repetições).')));

        function opcoes(sel, i) {
            var usados = selects.map(function(s, j) { return j === i ? '' : s.value; });
            var atual = sel.value;
            limpar(sel).appendChild(h('option', { value: '', text: '— Selecionar atleta presente —' }));
            presentes.forEach(function(p) {
                if (usados.indexOf(String(p.id)) >= 0) return;
                sel.appendChild(h('option', { value: String(p.id), selected: String(p.id) === atual, text: Gestao.nome(p) }));
            });
        }
        function refletir() {
            selects.forEach(function(s, i) {
                opcoes(s, i);
                s.closest('.gx-posicao').classList.toggle('cheia', !!s.value);
            });
            var cheios = selects.filter(function(s, i) { return s.value && marcas[i].value.trim(); }).length;
            btn.disabled = cheios < n;
            btn.textContent = cheios < n ? 'Preencha as ' + n + ' posições (' + cheios + '/' + n + ')' : 'Lançar pódio';
        }

        for (var i = 0; i < n; i++) {
            (function(i) {
                var sel = h('select', { id: 'gxSel' + i, class: 'gs-input', onChange: refletir });
                var marca = h('input', { id: 'gxMarca' + i, class: 'gs-input', type: 'text', maxlength: '50', autocomplete: 'off',
                    placeholder: 'Ex.: 10:01, 80 kg, 42 reps', onInput: refletir });
                selects.push(sel);
                marcas.push(marca);
                raiz.appendChild(h('div', { class: 'gx-posicao' }, [
                    h('div', { class: 'gx-posicao-topo' }, [
                        h('span', { class: 'gx-medalha', 'aria-hidden': 'true', text: Gestao.medalha(i + 1) }),
                        h('span', { class: 'gx-posicao-rotulo', text: (i + 1) + 'º lugar' }),
                        h('span', { class: 'gx-premio', text: 'Prêmio ' + fmtBRL(premios[i]) })
                    ]),
                    h('div', { class: 'gx-par' }, [
                        h('div', null, [h('label', { class: 'gs-label', for: 'gxSel' + i, text: 'Atleta' }), sel]),
                        h('div', null, [h('label', { class: 'gs-label', for: 'gxMarca' + i, text: 'Marca' }), marca])
                    ])
                ]));
            })(i);
        }

        var aviso = h('p', { class: 'gs-msg error', role: 'alert' });
        var btn = h('button', { type: 'button', class: 'gx-btn cheio', disabled: true, onClick: confirmar }, '');
        raiz.appendChild(aviso);
        raiz.appendChild(btn);
        refletir();

        async function confirmar() {
            aviso.classList.remove('show');
            var posicoes = [], linhas = [];
            for (var k = 0; k < n; k++) {
                var marca = marcas[k].value.trim();
                posicoes.push({ id_participante: parseInt(selects[k].value, 10), marca: marca });
                var atleta = presentes.filter(function(p) { return String(p.id) === selects[k].value; })[0];
                linhas.push([(k + 1) + 'º ' + Gestao.nome(atleta), marca + ' · ' + fmtBRL(premios[k])]);
            }
            var ok = await Modal.confirmar({
                titulo: 'Lançar o pódio?',
                mensagem: (d.aguardando_relancamento ? 'Este é o novo pódio após a anulação do resultado. ' : '') +
                    'Isso encerra o desafio e não pode ser desfeito. Os prêmios e pagamentos ficam na janela de contestação antes de entrar nas carteiras.',
                linhas: linhas, confirmar: 'Lançar pódio'
            });
            if (!ok) return;
            btnLoading(btn, true);
            try {
                var resp = await API.lancarPodio(d.id, posicoes);
                showToast('Pódio lançado!', 'success');
                limpar(raiz).appendChild(Gestao.podioFinal(resp.data));
                if (opts.aoFim) opts.aoFim(resp.data);
            } catch (e) {
                btnLoading(btn, false);
                aviso.textContent = mensagemErro(e);
                aviso.classList.add('show');
                refletir();
            }
        }
    },

    // Pódio já lançado: posição, atleta, marca e prêmio
    podioFinal: function(d) {
        var podio = d.podio || [];
        if (!podio.length) return estadoVazio({ icone: '🏆', titulo: 'Sem pódio registrado', texto: 'Este desafio foi encerrado sem colocados.' });
        return h('div', { class: 'gx-card destaque' }, [
            h('div', { class: 'gx-card-titulo', text: 'Pódio' }),
            podio.map(function(p) {
                return h('div', { class: 'gx-atleta' }, [
                    h('span', { class: 'gx-medalha', 'aria-hidden': 'true', text: Gestao.medalha(p.posicao) }),
                    h('div', { class: 'gx-atleta-info' }, [
                        h('div', { class: 'gx-atleta-nome', text: p.posicao + 'º · ' + nomeCompleto(p) }),
                        h('div', { class: 'gx-atleta-sub', text: 'Marca: ' + p.marca })
                    ]),
                    h('span', { class: 'gx-premio', text: fmtBRL(p.premio_centavos) })
                ]);
            })
        ]);
    }
};
