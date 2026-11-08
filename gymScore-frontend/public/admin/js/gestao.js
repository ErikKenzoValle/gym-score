// GymScore Admin — Administradores (só superadmin), Auditoria e troca da própria senha.
'use strict';

(function() {
    var h = Adm.h;
    var PAPEIS = [['admin', 'Admin'], ['superadmin', 'Superadmin']];

    var ACOES_AUDITORIA = [
        'login', 'login_falhou', 'logout', 'trocar_senha', 'saque_reconciliar', 'saque_reembolsar', 'saque_marcar_pago',
        'usuario_bloquear', 'usuario_desbloquear', 'desafio_cancelar', 'academia_aprovar', 'academia_rejeitar',
        'instrutor_aprovar', 'instrutor_rejeitar', 'admin_criar', 'admin_desativar', 'admin_reativar',
        'admin_trocar_papel', 'admin_resetar_senha', 'cli_criar_superadmin', 'cli_resetar_senha'
    ].map(function(a) { return [a, a.replace(/_/g, ' ')]; });
    var ALVOS = [['administrador', 'Administrador'], ['transacao', 'Transação'], ['usuario', 'Usuário'],
        ['desafio', 'Desafio'], ['academia', 'Academia'], ['instrutor', 'Instrutor']];

    // Mostra a senha temporária uma única vez (não fica salva em lugar nenhum do front)
    function mostrarSenhaTemporaria(admin, senha) {
        var codigo = h('code', { class: 'adm-senha-temp', text: senha });
        var copiar = h('button', { type: 'button', class: 'gs-btn gs-btn-secondary adm-btn-sm', text: 'Copiar' });
        copiar.addEventListener('click', async function() {
            try { await navigator.clipboard.writeText(senha); Adm.toast('Senha copiada.', 'success'); }
            catch (e) { Adm.toast('Não deu para copiar; selecione e copie manualmente.', 'error'); }
        });
        var m = Adm.modal({
            titulo: 'Senha temporária de ' + admin.nome,
            corpo: [
                h('p', { class: 'adm-modal-msg', text: 'Ela é mostrada só agora. Entregue por um canal seguro; a pessoa deve trocá-la no primeiro acesso.' }),
                Adm.dl([['E-mail', admin.email], ['Papel', Adm.rotulo(PAPEIS, admin.papel)]]),
                h('div', { class: 'adm-senha-box' }, [codigo, copiar])
            ],
            rodape: h('div', { class: 'adm-acoes' }, h('button', { type: 'button', class: 'gs-btn gs-btn-primary', text: 'Já anotei', onClick: function() { m.fechar(); } }))
        });
    }

    Adm.telas.administradores = function(raiz, params, ctx) {
        var lista;
        function recarregar() { lista.recarregar(); }

        function novo() {
            Adm.acao({
                titulo: 'Novo administrador',
                mensagem: 'Uma senha temporária é gerada e mostrada uma única vez.',
                campos: [
                    { nome: 'nome', rotulo: 'Nome', max: 100 },
                    { nome: 'email', rotulo: 'E-mail', tipo: 'email', max: 150 },
                    { nome: 'papel', rotulo: 'Papel', tipo: 'select', opcoes: PAPEIS, valor: 'admin' }
                ],
                textoConfirmar: 'Criar',
                executar: function(d) { return Adm.post('/administradores', { nome: d.nome, email: d.email, papel: d.papel, motivo: d.motivo }); },
                sucesso: 'Administrador criado.',
                aoConcluir: function(r) { recarregar(); if (r && r.senha_temporaria) mostrarSenhaTemporaria(r.admin, r.senha_temporaria); }
            });
        }

        function acao(a, tipo) {
            var cfg = {
                desativar: { t: 'Desativar ' + a.nome, msg: 'A conta perde o acesso na hora.', ok: 'Desativar', perigo: true },
                reativar: { t: 'Reativar ' + a.nome, msg: 'A conta volta a conseguir entrar.', ok: 'Reativar' },
                'resetar-senha': { t: 'Resetar a senha de ' + a.nome, msg: 'Gera uma senha temporária e derruba as sessões abertas dessa conta.', ok: 'Resetar senha', perigo: true },
                papel: { t: 'Trocar o papel de ' + a.nome, msg: 'A sessão da conta cai e ela entra de novo com o papel novo.', ok: 'Trocar papel' }
            }[tipo];
            Adm.acao({
                titulo: cfg.t, mensagem: cfg.msg, perigo: cfg.perigo, textoConfirmar: cfg.ok,
                linhas: [['E-mail', a.email], ['Papel atual', Adm.rotulo(PAPEIS, a.papel)]],
                campos: tipo === 'papel' ? [{ nome: 'papel', rotulo: 'Novo papel', tipo: 'select', opcoes: PAPEIS, valor: a.papel === 'admin' ? 'superadmin' : 'admin' }] : null,
                executar: function(d) {
                    var corpo = { motivo: d.motivo };
                    if (tipo === 'papel') corpo.papel = d.papel;
                    return Adm.post('/administradores/' + Adm.id(a.id) + '/' + tipo, corpo);
                },
                sucesso: function(r) { return r && r.alterado === false ? 'Nada mudou.' : 'Administrador atualizado.'; },
                aoConcluir: function(r) {
                    recarregar();
                    if (tipo === 'resetar-senha' && r && r.senha_temporaria) mostrarSenhaTemporaria(r.admin || a, r.senha_temporaria);
                }
            });
        }

        lista = Adm.lista(raiz, params, {
            tela: 'administradores', titulo: 'Administradores', descricao: 'Gestão de contas do painel (somente superadmin).',
            caminho: '/administradores',
            botoes: h('button', { type: 'button', class: 'gs-btn gs-btn-primary', text: '+ Novo administrador', onClick: novo }),
            filtros: [
                { nome: 'papel', rotulo: 'Papel', tipo: 'select', opcoes: PAPEIS },
                { nome: 'ativo', rotulo: 'Situação', tipo: 'select', opcoes: [['true', 'Ativos'], ['false', 'Inativos']] }
            ],
            colunas: [
                { titulo: 'ID', valor: function(a) { return '#' + a.id; } },
                { titulo: 'Nome', valor: function(a) { return h('div', null, [h('strong', { text: a.nome }), a.id === ctx.admin.id ? h('small', { class: 'adm-sub', text: ' (você)' }) : null]); } },
                { titulo: 'E-mail', valor: function(a) { return a.email; } },
                { titulo: 'Papel', valor: function(a) { return Adm.badge(Adm.rotulo(PAPEIS, a.papel), a.papel); } },
                { titulo: 'Situação', valor: function(a) { return a.ativo ? Adm.badge('Ativo', 'ativo') : Adm.badge('Inativo', 'inativo'); } },
                { titulo: 'Último login', classe: 'nowrap', valor: function(a) { return Adm.dataHora(a.ultimo_login_em); } },
                { titulo: 'Criado em', classe: 'nowrap', valor: function(a) { return Adm.dataHora(a.criado_em); } },
                { titulo: 'Ações', valor: function(a) {
                    if (a.id === ctx.admin.id) return h('span', { class: 'adm-sub', text: 'Use "Minha senha" no topo' });
                    return h('div', { class: 'adm-acoes adm-acoes-linha' }, [
                        a.ativo ? Adm.botao('Desativar', function() { acao(a, 'desativar'); }, 'gs-btn-danger') :
                            Adm.botao('Reativar', function() { acao(a, 'reativar'); }),
                        Adm.botao('Trocar papel', function() { acao(a, 'papel'); }),
                        Adm.botao('Resetar senha', function() { acao(a, 'resetar-senha'); })
                    ]);
                } }
            ]
        });
    };

    Adm.telas.auditoria = function(raiz, params) {
        Adm.lista(raiz, params, {
            tela: 'auditoria', titulo: 'Auditoria', descricao: 'Registro imutável de toda ação de administrador.',
            caminho: '/auditoria',
            filtros: [
                { nome: 'acao', rotulo: 'Ação', tipo: 'select', todos: 'Todas', opcoes: ACOES_AUDITORIA },
                { nome: 'alvo_tipo', rotulo: 'Alvo', tipo: 'select', opcoes: ALVOS },
                { nome: 'alvo_id', rotulo: 'ID do alvo', tipo: 'numero' },
                { nome: 'id_admin', rotulo: 'ID do admin', tipo: 'numero' },
                { nome: 'de', rotulo: 'De', tipo: 'data' },
                { nome: 'ate', rotulo: 'Até', tipo: 'data' }
            ],
            colunas: [
                { titulo: 'Data', classe: 'nowrap', valor: function(a) { return Adm.dataHora(a.criado_em); } },
                { titulo: 'Admin', valor: function(a) { return h('div', null, [h('strong', { text: a.admin_nome || '#' + a.id_admin }), a.admin_email ? h('small', { class: 'adm-sub adm-bloco', text: a.admin_email }) : null]); } },
                { titulo: 'Ação', classe: 'mono', valor: function(a) { return a.acao; } },
                { titulo: 'Alvo', valor: function(a) { return Adm.rotulo(ALVOS, a.alvo_tipo) + (a.alvo_id ? ' #' + a.alvo_id : ''); } },
                { titulo: 'Motivo', classe: 'adm-col-texto', valor: function(a) { return a.motivo; } },
                { titulo: 'IP', classe: 'mono', valor: function(a) { return a.ip; } },
                { titulo: 'Detalhes', valor: function(a) {
                    if (!a.detalhes || (typeof a.detalhes === 'object' && !Object.keys(a.detalhes).length)) return '—';
                    return h('details', { class: 'adm-detalhes' }, [h('summary', { text: 'ver' }), h('pre', { text: JSON.stringify(a.detalhes, null, 2) })]);
                } }
            ]
        });
    };

    // Troca da própria senha (não exige motivo: a auditoria registra 'trocar_senha')
    Adm.trocarMinhaSenha = function() {
        var erro = h('p', { class: 'adm-erro', role: 'alert', hidden: true });
        function campo(id, rotulo, ac) {
            var el = h('input', { id: id, class: 'gs-input', type: 'password', autocomplete: ac, required: true, maxlength: '72' });
            return { el: el, no: h('div', { class: 'adm-campo' }, [h('label', { for: id, text: rotulo }), el]) };
        }
        var atual = campo('admSenhaAtual', 'Senha atual', 'current-password');
        var nova = campo('admSenhaNova', 'Nova senha (mínimo 12 caracteres)', 'new-password');
        var conf = campo('admSenhaConf', 'Confirme a nova senha', 'new-password');
        var form = h('form', { novalidate: true }, [atual.no, nova.no, conf.no, erro]);
        var ok = h('button', { type: 'submit', class: 'gs-btn gs-btn-primary', text: 'Trocar senha' });
        ok.setAttribute('form', 'admFormSenha');
        form.id = 'admFormSenha';
        var m = Adm.modal({ titulo: 'Trocar minha senha', corpo: form, rodape: h('div', { class: 'adm-acoes' }, [
            h('button', { type: 'button', class: 'gs-btn gs-btn-secondary', text: 'Cancelar', onClick: function() { if (!m.ocupado) m.fechar(); } }), ok]) });
        form.addEventListener('submit', async function(e) {
            e.preventDefault();
            if (m.ocupado) return;
            function falha(msg) { erro.textContent = msg; erro.hidden = false; }
            if (!atual.el.value) return falha('Informe a senha atual.');
            if (nova.el.value.length < 12) return falha('A nova senha precisa ter pelo menos 12 caracteres.');
            if (nova.el.value !== conf.el.value) return falha('A confirmação não confere.');
            erro.hidden = true;
            m.ocupado = true; ok.disabled = true; ok.classList.add('loading');
            try {
                await Adm.post('/auth/senha', { senha_atual: atual.el.value, nova_senha: nova.el.value });
                m.ocupado = false;
                m.fechar();
                Adm.toast('Senha alterada. As outras sessões foram encerradas.', 'success');
            } catch (err) {
                m.ocupado = false; ok.disabled = false; ok.classList.remove('loading');
                if (err.status !== 401) falha(err.message);
            }
        });
    };
})();
