// Tela de denúncia: /denunciar?id=<usuário>&desafio=<id>
// Com ?desafio a pessoa é escolhida entre os participantes; as categorias de desafio só valem com ele.
// Cria a denúncia (JSON) e depois envia as imagens uma a uma (multipart), mostrando o progresso.
if (!Auth.require()) throw new Error('unauthenticated');

(function() {
    var $ = function(id) { return document.getElementById(id); };
    var params = new URLSearchParams(location.search);
    var idAlvo = parseInt(params.get('id'), 10) || 0;
    var idDesafio = parseInt(params.get('desafio'), 10) || 0;
    var arquivos = []; // {file, url}
    var enviando = false;
    var eu = null;

    $('btnVoltar').addEventListener('click', function() {
        if (history.length > 1) history.back(); else window.location.href = '/menu';
    });

    function estado(no) { limpar($('estado')); if (no) $('estado').appendChild(no); }

    function montarCategorias() {
        var sel = limpar($('categoria'));
        sel.appendChild(h('option', { value: '', text: '— Escolha —' }));
        Moderacao.CATEGORIAS.forEach(function(c) {
            var deDesafio = Moderacao.CATEGORIAS_DESAFIO.indexOf(c[0]) >= 0;
            sel.appendChild(h('option', { value: c[0], disabled: deDesafio && !idDesafio,
                text: c[1] + (deDesafio && !idDesafio ? ' (abra pelo desafio)' : '') }));
        });
        $('categoriaAjuda').textContent = idDesafio
            ? 'Denúncias de suborno, pódio ou presença seguram os prêmios do desafio até serem julgadas.'
            : 'Suborno, pódio e presença são denunciados pelo botão "Denunciar" dentro do desafio.';
    }

    function cartaoAlvo(nome, sub) {
        return h('div', { class: 'md-alvo' }, [
            h('div', { class: 'md-alvo-avatar', 'aria-hidden': 'true', text: (nome || '?').charAt(0).toUpperCase() }),
            h('div', null, [h('div', { class: 'md-alvo-nome', text: nome }), sub ? h('div', { class: 'md-alvo-sub', text: sub }) : null])
        ]);
    }

    async function carregar() {
        Skeleton.cards($('estado'), 2, 70);
        try {
            eu = await Auth.verificar();
            if (!eu) return;
            if (idAlvo && idAlvo === eu.id) throw new Error('Você não pode denunciar a si mesmo.');
            if (idDesafio) {
                var d = (await API.buscarDesafio(idDesafio)).data;
                var pessoas = (d.participantes || []).filter(function(p) { return p.id !== eu.id; });
                limpar($('alvo')).appendChild(cartaoAlvo(d.titulo, 'Desafio #' + d.id + (d.academia ? ' · ' + d.academia.nome_fantasia : '')));
                var sel = limpar($('pessoa'));
                sel.appendChild(h('option', { value: '', text: '— Escolha o participante —' }));
                pessoas.forEach(function(p) {
                    sel.appendChild(h('option', { value: String(p.id), selected: p.id === idAlvo, text: nomeCompleto(p) }));
                });
                // Instrutor designado (conta = instrutor.id_usuario): alvo típico de suborno_instrutor
                var idInstr = d.instrutor && d.instrutor.id_usuario;
                if (idInstr && idInstr !== eu.id && !pessoas.some(function(p) { return p.id === idInstr; })) {
                    pessoas.push({ id: idInstr });
                    sel.appendChild(h('option', { value: String(idInstr), selected: idInstr === idAlvo,
                        text: (d.instrutor.nome || 'Instrutor') + ' (instrutor do desafio)' }));
                }
                if (idAlvo && !pessoas.some(function(p) { return p.id === idAlvo; })) {
                    // Alvo vindo do link mas fora da lista (instrutor/academia): mantém a escolha
                    sel.appendChild(h('option', { value: String(idAlvo), selected: true, text: 'Usuário #' + idAlvo }));
                }
                if (!sel.options.length || sel.options.length === 1) {
                    estado(estadoVazio({ icone: '🛡️', titulo: 'Ninguém para denunciar neste desafio', texto: 'Só participantes do desafio podem ser escolhidos aqui.',
                        acao: { texto: 'Voltar ao desafio', href: '/desafios?id=' + idDesafio } }));
                    return;
                }
                $('campoPessoa').hidden = false;
            } else if (idAlvo) {
                var u = (await API.perfilPublico(idAlvo)).data || {};
                limpar($('alvo')).appendChild(cartaoAlvo(nomeCompleto(u), u.username ? '@' + u.username : null));
            } else {
                estado(estadoVazio({ icone: '🛡️', titulo: 'Escolha quem denunciar', texto: 'Abra o perfil da pessoa ou o desafio e toque em "Denunciar".',
                    acao: { texto: 'Ver desafios', href: '/desafios' } }));
                return;
            }
            estado(null);
            montarCategorias();
            $('formDenuncia').hidden = false;
        } catch (e) {
            estado(estadoVazio({ erro: true, titulo: 'Não foi possível abrir a denúncia', texto: mensagemErro(e),
                acao: { texto: 'Tentar de novo', onClick: carregar } }));
        }
    }

    // ── Imagens: validação local, prévias e remoção ──────────────────────────
    function renderPrevias() {
        var box = limpar($('previas'));
        arquivos.forEach(function(a, i) {
            box.appendChild(h('div', { class: 'md-previa' }, [
                h('img', { src: a.url, alt: 'Prévia ' + (i + 1) }),
                h('small', { text: a.file.name }),
                h('button', { type: 'button', class: 'md-previa-remover', 'aria-label': 'Remover imagem ' + (i + 1), text: '×', onClick: function() {
                    URL.revokeObjectURL(a.url);
                    arquivos.splice(i, 1);
                    renderPrevias();
                } })
            ]));
        });
        $('arquivos').disabled = arquivos.length >= 3;
    }
    $('arquivos').addEventListener('change', function() {
        var erros = [];
        Array.prototype.forEach.call(this.files, function(f) {
            if (arquivos.length >= 3) { erros.push('Máximo de 3 imagens.'); return; }
            if (Moderacao.MIMES_IMAGEM.indexOf(f.type) < 0) { erros.push(f.name + ': use JPG, PNG ou WebP.'); return; }
            if (f.size > Moderacao.EVIDENCIA_MAX_BYTES) { erros.push(f.name + ': maior que 2 MB.'); return; }
            if (f.size === 0) { erros.push(f.name + ': arquivo vazio.'); return; }
            arquivos.push({ file: f, url: URL.createObjectURL(f) });
        });
        this.value = '';
        renderPrevias();
        mostrarErro(erros.filter(function(v, i, a) { return a.indexOf(v) === i; }).join(' '));
    });

    $('descricao').addEventListener('input', function() { $('contador').textContent = this.value.trim().length + '/2000'; });

    function mostrarErro(msg) { $('erro').textContent = msg || ''; $('erro').classList.toggle('show', !!msg); }

    // ── Envio ────────────────────────────────────────────────────────────────
    $('formDenuncia').addEventListener('submit', async function(e) {
        e.preventDefault();
        if (enviando) return;
        mostrarErro('');
        var alvo = idDesafio ? parseInt($('pessoa').value, 10) : idAlvo;
        var cat = $('categoria').value;
        var desc = $('descricao').value.trim();
        if (!alvo) { erroCampo($('pessoa'), 'Escolha quem denunciar.'); $('pessoa').focus(); return; }
        erroCampo($('pessoa'), '');
        if (!cat) { erroCampo($('categoria'), 'Escolha a categoria.'); $('categoria').focus(); return; }
        erroCampo($('categoria'), '');
        if (desc.length < 20 || desc.length > 2000) { erroCampo($('descricao'), 'Descreva com 20 a 2000 caracteres.'); $('descricao').focus(); return; }
        erroCampo($('descricao'), '');

        var nomeAlvo = idDesafio ? $('pessoa').selectedOptions[0].textContent : ($('alvo').querySelector('.md-alvo-nome') || {}).textContent;
        var ok = await Modal.confirmar({
            titulo: 'Enviar denúncia?',
            mensagem: 'A denúncia não pode ser apagada depois de enviada. Sua identidade fica em sigilo.',
            linhas: [['Denunciado', nomeAlvo || '#' + alvo], ['Categoria', Moderacao.rotuloCategoria(cat)],
                ['Imagens', String(arquivos.length)]].concat(idDesafio ? [['Desafio', '#' + idDesafio]] : []),
            confirmar: 'Enviar denúncia'
        });
        if (!ok) return;

        enviando = true;
        var btn = $('btnEnviar');
        btnLoading(btn, true);
        var den;
        try {
            var corpo = { id_denunciado: alvo, categoria: cat, descricao: desc };
            if (idDesafio) corpo.id_desafio = idDesafio;
            den = (await API.criarDenuncia(corpo)).data;
        } catch (err) {
            enviando = false;
            btnLoading(btn, false);
            mostrarErro(err.status === 429 ? 'Você atingiu o limite de denúncias por dia. Tente amanhã.' : mensagemErro(err));
            return;
        }
        // Imagens depois da denúncia criada; falha numa delas não desfaz a denúncia
        var falhas = [];
        for (var i = 0; i < arquivos.length; i++) {
            try { await API.enviarEvidencia(den.id, arquivos[i].file); }
            catch (err) { falhas.push(arquivos[i].file.name + ': ' + mensagemErro(err)); }
        }
        arquivos.forEach(function(a) { URL.revokeObjectURL(a.url); });
        arquivos = [];
        $('formDenuncia').hidden = true;
        estado(h('div', { class: 'gs-card' }, [
            h('h2', { text: 'Denúncia #' + den.id + ' registrada' }),
            h('p', { class: 'md-item-texto', text: den.jurisdicao === 'academia'
                ? 'Ela será analisada pela academia do desafio. Se a academia não decidir no prazo, sobe para a equipe GymScore.'
                : 'Ela será analisada pela equipe GymScore. Você acompanha o andamento em "Minhas denúncias".' }),
            falhas.length ? h('p', { class: 'md-nota erro', text: 'Algumas imagens não foram enviadas (' + falhas.join('; ') + '). Você pode reenviá-las em "Minhas denúncias" enquanto a denúncia estiver aberta.' }) : null,
            h('div', { class: 'md-acoes' }, [
                h('a', { class: 'gs-btn gs-btn-primary', href: '/minhas-denuncias?id=' + den.id, text: 'Ver minhas denúncias' }),
                h('a', { class: 'gs-btn gs-btn-secondary', href: idDesafio ? '/desafios?id=' + idDesafio : '/menu', text: 'Voltar' })
            ])
        ]));
        showToast('Denúncia enviada.', 'success');
    });

    carregar();
    renderBottomNav('');
})();
