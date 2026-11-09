// GymScore — endereço estruturado da academia (rodada 4): CEP com máscara e busca automática
// (GET /api/cep/:cep, proxy do ViaCEP no backend), logradouro, número, complemento, bairro, cidade e UF.
// Depende de js/api.js (h, limpar, Mask, API, erroCampo). Tudo via h()/textContent.
//
// var end = Endereco.montar(container, { valores: {...} });
//   end.valores()  -> { cep (só dígitos), logradouro, numero, complemento, bairro, cidade, uf }
//   end.validar()  -> true | false (marca os campos com erroCampo e foca o primeiro inválido)
//   end.preencher({...})
// A busca do CEP é só ajuda: todos os campos continuam editáveis e o envio nunca depende dela.
var Endereco = {
    UFS: ['AC', 'AL', 'AP', 'AM', 'BA', 'CE', 'DF', 'ES', 'GO', 'MA', 'MT', 'MS', 'MG', 'PA', 'PB', 'PR', 'PE', 'PI',
          'RJ', 'RN', 'RS', 'RO', 'RR', 'SC', 'SP', 'SE', 'TO'],

    montar: function(container, opts) {
        opts = opts || {};
        var pre = opts.prefixo || 'end';
        var campos = {};
        function campo(chave, rotulo, attrs, extraClasse) {
            var id = pre + chave.charAt(0).toUpperCase() + chave.slice(1);
            var input = h('input', Object.assign({ type: 'text', id: id, class: 'gs-input' }, attrs));
            campos[chave] = input;
            return h('div', { class: 'gs-field' + (extraClasse ? ' ' + extraClasse : '') }, [h('label', { class: 'gs-label', for: id, text: rotulo }), input]);
        }
        var uf = h('select', { id: pre + 'Uf', class: 'gs-input gs-input-select', required: true, 'aria-required': 'true', autocomplete: 'address-level1' },
            [h('option', { value: '', text: 'UF' })].concat(Endereco.UFS.map(function(s) { return h('option', { value: s, text: s }); })));
        campos.uf = uf;
        var status = h('p', { class: 'gs-end-status', id: pre + 'CepStatus', 'aria-live': 'polite' });

        limpar(container);
        container.appendChild(h('div', { class: 'gs-end-linha' }, [
            campo('cep', 'CEP', { placeholder: '00000-000', required: true, 'aria-required': 'true' }),
            h('div', { class: 'gs-field gs-end-curto' }, [h('label', { class: 'gs-label', for: pre + 'Uf', text: 'UF' }), uf])
        ]));
        container.appendChild(status);
        container.appendChild(campo('logradouro', 'Logradouro', { placeholder: 'Rua, avenida...', maxlength: '150', required: true, 'aria-required': 'true', autocomplete: 'address-line1' }));
        container.appendChild(h('div', { class: 'gs-end-linha' }, [
            campo('numero', 'Número', { placeholder: 'Ex.: 123 ou S/N', maxlength: '20', required: true, 'aria-required': 'true' }, 'gs-end-curto'),
            campo('complemento', 'Complemento', { placeholder: 'Opcional: sala, bloco...', maxlength: '100', autocomplete: 'address-line2' })
        ]));
        container.appendChild(campo('bairro', 'Bairro', { maxlength: '100', required: true, 'aria-required': 'true', autocomplete: 'address-level3' }));
        container.appendChild(campo('cidade', 'Cidade', { maxlength: '100', required: true, 'aria-required': 'true', autocomplete: 'address-level2' }));
        Mask.aplicar(campos.cep, 'cep');

        function avisar(texto, tipo) {
            status.textContent = texto || '';
            status.className = 'gs-end-status' + (tipo ? ' ' + tipo : '');
        }

        // Busca ao completar 8 dígitos; resposta velha (CEP já mudou) é descartada
        var seq = 0, ultimo = '';
        async function buscar(cep) {
            var minha = ++seq;
            ultimo = cep;
            avisar('Buscando endereço...');
            campos.cep.setAttribute('aria-busy', 'true');
            try {
                var d = (await API.buscarCEP(cep)).data || {};
                if (minha !== seq) return;
                // Os campos do CEP anterior não valem mais: o que o CEP não traz (CEP geral de cidade) fica vazio
                ['logradouro', 'bairro', 'cidade'].forEach(function(k) { campos[k].value = d[k] || ''; if (d[k]) erroCampo(campos[k], ''); });
                if (d.uf && Endereco.UFS.indexOf(String(d.uf).toUpperCase()) >= 0) { uf.value = String(d.uf).toUpperCase(); erroCampo(uf, ''); }
                avisar(d.logradouro ? 'Endereço encontrado. Confira e informe o número.' : 'CEP da cidade encontrado. Complete o logradouro, o bairro e o número.', 'ok');
                (d.logradouro ? campos.numero : campos.logradouro).focus();
            } catch (e) {
                if (minha !== seq) return;
                ultimo = ''; // permite tentar de novo o mesmo CEP
                avisar(e.status === 404 ? 'CEP não encontrado. Confira o número ou preencha o endereço à mão.'
                    : e.status === 400 ? 'CEP inválido. Confira os 8 dígitos.'
                    : e.status === 429 ? 'Muitas buscas de CEP seguidas. Preencha o endereço à mão ou tente em instantes.'
                    : 'A busca de CEP está indisponível agora. Preencha o endereço à mão; o cadastro funciona normalmente.', 'erro');
            } finally {
                if (minha === seq) campos.cep.removeAttribute('aria-busy');
            }
        }
        campos.cep.addEventListener('input', function() {
            var d = Mask.digitos(campos.cep.value);
            erroCampo(campos.cep, '');
            if (d.length === 8 && d !== ultimo) buscar(d);
            else if (d.length < 8) { seq++; ultimo = ''; avisar(''); campos.cep.removeAttribute('aria-busy'); }
        });
        Object.keys(campos).forEach(function(k) {
            if (k !== 'cep') campos[k].addEventListener(k === 'uf' ? 'change' : 'input', function() { erroCampo(campos[k], ''); });
        });

        var api = {
            campos: campos,
            preencher: function(v) {
                v = v || {};
                if (v.cep) campos.cep.value = Mask.cep(v.cep);
                ['logradouro', 'numero', 'complemento', 'bairro', 'cidade'].forEach(function(k) { if (v[k] != null) campos[k].value = v[k]; });
                if (v.uf) uf.value = String(v.uf).toUpperCase();
                ultimo = Mask.digitos(campos.cep.value);
            },
            valores: function() {
                return {
                    cep: Mask.digitos(campos.cep.value),
                    logradouro: campos.logradouro.value.trim(),
                    numero: campos.numero.value.trim(),
                    complemento: campos.complemento.value.trim(),
                    bairro: campos.bairro.value.trim(),
                    cidade: campos.cidade.value.trim(),
                    uf: uf.value
                };
            },
            validar: function() {
                var v = api.valores(), primeiro = null;
                function checar(k, ok, msg) {
                    erroCampo(campos[k], ok ? '' : msg);
                    if (!ok && !primeiro) primeiro = campos[k];
                }
                checar('cep', v.cep.length === 8, 'Informe o CEP com 8 dígitos.');
                checar('uf', Endereco.UFS.indexOf(v.uf) >= 0, 'Escolha a UF.');
                checar('logradouro', !!v.logradouro, 'Informe o logradouro.');
                checar('numero', !!v.numero, 'Informe o número (ou S/N).');
                checar('bairro', !!v.bairro, 'Informe o bairro.');
                checar('cidade', !!v.cidade, 'Informe a cidade.');
                if (primeiro) primeiro.focus();
                return !primeiro;
            }
        };
        if (opts.valores) api.preencher(opts.valores);
        return api;
    }
};
