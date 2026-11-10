// GymScore — Nível GymScore + gráficos de teia (radar) em SVG próprio, sem biblioteca externa.
// Rodada 6 (F6-3). Usado por perfil.js (próprio) e perfil-publico.js (público).
// Consome GET /api/usuarios/:id/estatisticas. Depende de api.js (h, limpar, _req, estadoVazio, Skeleton, fmtDataHora).
// Todo texto vindo da API entra por textContent (h({text}) / createTextNode), nunca innerHTML.
var Teia = (function() {
    var SVG_NS = 'http://www.w3.org/2000/svg';
    var _seq = 0;

    // Rótulos de reserva (a API manda `eixos` com a ordem e os rótulos oficiais)
    var EIXOS_PADRAO = {
        atributos: [
            { chave: 'forca', rotulo: 'Força' }, { chave: 'resistencia', rotulo: 'Resistência' },
            { chave: 'velocidade', rotulo: 'Velocidade' }, { chave: 'agilidade', rotulo: 'Agilidade' },
            { chave: 'mobilidade', rotulo: 'Mobilidade' }, { chave: 'constancia', rotulo: 'Constância' }
        ],
        categorias: [
            { chave: 'corrida', rotulo: 'Corrida' }, { chave: 'musculacao', rotulo: 'Musculação' },
            { chave: 'crossfit', rotulo: 'Crossfit' }, { chave: 'funcional', rotulo: 'Funcional' },
            { chave: 'ciclismo', rotulo: 'Ciclismo' }, { chave: 'natacao', rotulo: 'Natação' },
            { chave: 'calistenia', rotulo: 'Calistenia' }, { chave: 'luta', rotulo: 'Luta' }
        ]
    };

    function estatisticas(idUsuario) {
        return _req('/api/usuarios/' + encodeURIComponent(idUsuario) + '/estatisticas');
    }

    function s(tag, attrs, filhos) {
        var el = document.createElementNS(SVG_NS, tag);
        Object.keys(attrs || {}).forEach(function(k) {
            if (attrs[k] !== null && attrs[k] !== undefined) el.setAttribute(k, attrs[k]);
        });
        (filhos || []).forEach(function(f) {
            if (f == null) return;
            el.appendChild(typeof f === 'string' ? document.createTextNode(f) : f);
        });
        return el;
    }

    function valor(v) {
        var n = Math.round(Number(v) || 0);
        return Math.max(0, Math.min(100, n));
    }

    function ponto(i, n, r) {
        var ang = -Math.PI / 2 + i * 2 * Math.PI / n; // começa no topo, sentido horário
        return [r * Math.cos(ang), r * Math.sin(ang)];
    }
    function fmt(n) { return (Math.round(n * 10) / 10).toString(); }
    function pontos(lista) { return lista.map(function(p) { return fmt(p[0]) + ',' + fmt(p[1]); }).join(' '); }

    // Radar: eixos [{chave, rotulo}], valores {chave: 0..100}. Retorna <figure> com SVG + tabela alternativa.
    function radar(o) {
        var id = 'teia' + (++_seq);
        var eixos = o.eixos && o.eixos.length ? o.eixos : [];
        var n = eixos.length;
        var R = 92;
        var vals = eixos.map(function(e) { return valor((o.valores || {})[e.chave]); });

        var resumo = eixos.map(function(e, i) { return e.rotulo + ' ' + vals[i]; }).join(', ');
        var svg = s('svg', {
            class: 'teia-svg', viewBox: '-185 -128 370 262', role: 'img', focusable: 'false',
            'aria-labelledby': id + '-t ' + id + '-d', preserveAspectRatio: 'xMidYMid meet'
        }, [
            s('title', { id: id + '-t' }, [o.titulo]),
            s('desc', { id: id + '-d' }, ['Valores de 0 a 100: ' + resumo + '.'])
        ]);

        // Grade: anéis 25/50/75/100 + raios
        var grade = s('g', { class: 'teia-grade', 'aria-hidden': 'true' });
        [25, 50, 75, 100].forEach(function(nivel) {
            var anel = [];
            for (var i = 0; i < n; i++) anel.push(ponto(i, n, R * nivel / 100));
            grade.appendChild(s('polygon', { points: pontos(anel), class: nivel === 100 ? 'teia-anel teia-anel-ext' : 'teia-anel' }));
        });
        for (var i = 0; i < n; i++) {
            var p = ponto(i, n, R);
            grade.appendChild(s('line', { x1: 0, y1: 0, x2: fmt(p[0]), y2: fmt(p[1]), class: 'teia-raio' }));
        }
        // Escala (50) no raio do topo; o anel externo é 100
        grade.appendChild(s('text', { x: 3, y: fmt(-R * 0.5 + 3), class: 'teia-escala' }, ['50']));
        svg.appendChild(grade);

        // Área do atleta (mínimo visual de 2% para o polígono não sumir no zero)
        var area = vals.map(function(v, i) { return ponto(i, n, R * Math.max(v, 2) / 100); });
        var dados = s('g', { class: 'teia-dados', 'aria-hidden': 'true' });
        dados.appendChild(s('polygon', { points: pontos(area), class: 'teia-area' }));
        area.forEach(function(p, i) {
            if (vals[i] > 0) dados.appendChild(s('circle', { cx: fmt(p[0]), cy: fmt(p[1]), r: 3.2, class: 'teia-ponto' }));
        });
        svg.appendChild(dados);

        // Rótulos com o valor (legível sem passar o mouse)
        var rot = s('g', { class: 'teia-rotulos', 'aria-hidden': 'true' });
        eixos.forEach(function(e, i) {
            var p = ponto(i, n, R + 12);
            var x = p[0], y = p[1];
            var anchor = Math.abs(x) < 4 ? 'middle' : (x > 0 ? 'start' : 'end');
            var y0 = y < -4 ? y - 14 : (y > 4 ? y + 4 : y - 4); // em cima: sobe as 2 linhas; embaixo: desce
            rot.appendChild(s('text', { x: fmt(x), y: fmt(y0), 'text-anchor': anchor, class: 'teia-rotulo' }, [
                s('tspan', { x: fmt(x), dy: 0 }, [e.rotulo]),
                s('tspan', { x: fmt(x), dy: 14, class: 'teia-valor' }, [String(vals[i])])
            ]));
        });
        svg.appendChild(rot);

        // Tabela alternativa (leitores de tela, e visível no botão "Ver em tabela")
        var tabela = h('table', { class: 'teia-tabela', id: id + '-tab', hidden: true }, [
            h('caption', { text: o.titulo + ' (0 a 100)' }),
            h('thead', null, h('tr', null, [h('th', { scope: 'col', text: 'Eixo' }), h('th', { scope: 'col', text: 'Valor' })])),
            h('tbody', null, eixos.map(function(e, i) {
                return h('tr', null, [h('th', { scope: 'row', text: e.rotulo }), h('td', { text: String(vals[i]) })]);
            }))
        ]);
        var btn = h('button', { type: 'button', class: 'teia-toggle', 'aria-expanded': 'false', 'aria-controls': id + '-tab', text: 'Ver em tabela' });
        btn.addEventListener('click', function() {
            var abrir = tabela.hidden;
            tabela.hidden = !abrir;
            btn.setAttribute('aria-expanded', abrir ? 'true' : 'false');
            btn.textContent = abrir ? 'Ocultar tabela' : 'Ver em tabela';
        });

        return h('figure', { class: 'teia-fig' }, [
            h('figcaption', { class: 'teia-titulo', text: o.titulo }),
            o.subtitulo ? h('p', { class: 'teia-sub', text: o.subtitulo }) : null,
            svg,
            btn,
            tabela
        ]);
    }

    // Cartão do nível: número, barra até o próximo nível e composição dos pontos
    function cartaoNivel(e, proprio) {
        var nivel = e.nivel || 1, max = e.nivel_max || 10;
        var ini = e.nivel_inicio_em || 0, prox = e.proximo_nivel_em;
        var pct = prox != null && prox > ini ? Math.max(0, Math.min(100, Math.round((e.pontuacao - ini) / (prox - ini) * 100))) : 100;
        var c = e.composicao || {};
        var linhas = [];
        if (c.pontos_podios) linhas.push(['Pódios', c.pontos_podios, [c.primeiros ? c.primeiros + '× 1º' : '', c.segundos ? c.segundos + '× 2º' : '', c.terceiros ? c.terceiros + '× 3º' : '', c.outros_premiados ? c.outros_premiados + '× demais' : ''].filter(Boolean).join(', ')]);
        if (c.pontos_presencas) linhas.push(['Presenças em desafios', c.pontos_presencas, c.presencas + (c.presencas === 1 ? ' desafio' : ' desafios')]);
        if (c.pontos_treinos) linhas.push(['Treinos', c.pontos_treinos, c.dias_treino + (c.dias_treino === 1 ? ' dia' : ' dias') + ' com treino']);

        var barraTxt = prox != null
            ? (e.faltam_pontos || 0) + ' pts para o nível ' + (nivel + 1)
            : 'Nível máximo alcançado';
        return h('div', { class: 'nivel-card' }, [
            h('div', { class: 'nivel-topo' }, [
                h('div', { class: 'nivel-num', 'aria-hidden': 'true', text: String(nivel) }),
                h('div', { class: 'nivel-info' }, [
                    h('div', { class: 'nivel-rot', text: 'Nível GymScore ' + nivel + ' de ' + max }),
                    h('div', { class: 'nivel-pts', text: e.pontuacao + ' pontos' })
                ])
            ]),
            h('div', { class: 'nivel-barra', role: 'progressbar', 'aria-label': 'Progresso para o próximo nível',
                'aria-valuemin': '0', 'aria-valuemax': '100', 'aria-valuenow': String(pct), 'aria-valuetext': barraTxt },
                h('span', { class: 'nivel-fill', style: { width: pct + '%' } })),
            h('div', { class: 'nivel-legenda' }, [
                h('span', { text: barraTxt }),
                prox != null ? h('span', { text: 'Próximo: ' + prox + ' pts' }) : null
            ]),
            linhas.length ? h('ul', { class: 'nivel-comp', 'aria-label': 'De onde vêm os pontos' }, linhas.map(function(l) {
                return h('li', null, [h('span', null, [l[0], l[2] ? h('small', { text: ' (' + l[2] + ')' }) : null]), h('strong', { text: '+' + l[1] })]);
            })) : h('p', { class: 'teia-nota', text: proprio
                ? 'Você ainda não tem pontos. Registre treinos e participe de desafios para subir de nível.'
                : 'Ainda sem pontos.' })
        ]);
    }

    function comoCalculamos(e) {
        var d = e.dados || {};
        var janela = d.janela_dias || 180;
        return h('details', { class: 'teia-como' }, [
            h('summary', { text: 'Como calculamos' }),
            e.base_calculo ? h('p', { text: e.base_calculo }) : null,
            h('p', { text: 'Nos últimos ' + janela + ' dias: ' + (d.exercicios || 0) + ' exercícios concluídos, ' + (d.dias_treino || 0) +
                ' dias com treino, ' + (d.desafios || 0) + ' desafios com presença' + (d.podios ? ' (' + d.podios + ' no pódio)' : '') + '.' }),
            e.calculado_em ? h('p', { class: 'teia-nota', text: 'Atualizado em ' + fmtDataHora(e.calculado_em) + '.' }) : null
        ]);
    }

    // Monta a seção completa (nível + 2 teias + "como calculamos") com carregamento/erro/vazio.
    // opts: { proprio: bool, nome: string }
    async function montar(container, idUsuario, opts) {
        opts = opts || {};
        limpar(container);
        container.setAttribute('aria-busy', 'true');
        Skeleton.cards(container, 2, 120);
        var e;
        try {
            e = (await estatisticas(idUsuario)).data || {};
        } catch (err) {
            limpar(container).removeAttribute('aria-busy');
            container.appendChild(estadoVazio({ erro: true, titulo: 'Não foi possível carregar o nível e os gráficos', texto: mensagemErro(err),
                acao: { texto: 'Tentar de novo', onClick: function() { montar(container, idUsuario, opts); } } }));
            return null;
        }
        limpar(container).removeAttribute('aria-busy');
        var eixos = e.eixos || {};
        container.appendChild(cartaoNivel(e, opts.proprio));
        if (e.sem_dados) {
            container.appendChild(h('p', { class: 'md-nota', text: opts.proprio
                ? (e.mensagem || 'Ainda não há treinos nem desafios recentes para montar o gráfico.')
                : 'Ainda não há treinos nem desafios recentes para montar o gráfico.' }));
        }
        container.appendChild(h('div', { class: 'teia-grid' }, [
            radar({ titulo: 'Atributos', subtitulo: 'Onde ' + (opts.proprio ? 'você é' : (opts.nome || 'o atleta') + ' é') + ' mais forte',
                eixos: (eixos.atributos && eixos.atributos.length) ? eixos.atributos : EIXOS_PADRAO.atributos, valores: e.atributos }),
            radar({ titulo: 'Categorias de desafio', subtitulo: 'Desempenho por modalidade',
                eixos: (eixos.categorias && eixos.categorias.length) ? eixos.categorias : EIXOS_PADRAO.categorias, valores: e.categorias })
        ]));
        container.appendChild(comoCalculamos(e));
        return e;
    }

    return { estatisticas: estatisticas, radar: radar, cartaoNivel: cartaoNivel, montar: montar };
})();
