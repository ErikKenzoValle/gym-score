package services

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"

	"gorm.io/gorm"
)

// Dinheiro do pódio na moderação (parte 2 — spec §2.6/§2.7, Q3, Q5, Q6):
//   - o pódio cria pagamentos_pendentes (desafio_service.LancarPodio); o job liberar-pagamentos credita
//     os a_liberar com a janela vencida (retidos nunca);
//   - a anulação do resultado (superadmin) cancela os pendentes não liberados, estorna do saldo os já
//     liberados (o que faltar vira usuario_debitos), estorna a taxa liberada (Q6), desfaz os pontos e,
//     conforme o modo, reabre o desafio para relançamento (Q3) ou devolve as inscrições INTEGRAIS;
//   - ordem de travas em liberação e anulação: desafio -> pendentes -> usuários (ordem de id) ->
//     usuario_debitos. Liberação × anulação ficam serializadas pela trava do desafio; o saque trava o
//     usuário antes de olhar os débitos.

// AcaoDesafioAnularResultado é a ação do admin na auditoria.
const AcaoDesafioAnularResultado = "desafio_anular_resultado"

// tentativasDeadlock: a transação desfeita por deadlock (1213) é repetida do zero (ver EhDeadlock).
const tentativasDeadlock = 3

func comRetentativaDeadlock(fn func() error) error {
	var err error
	for i := 0; i < tentativasDeadlock; i++ {
		if err = fn(); err == nil || !repositories.EhDeadlock(err) {
			return err
		}
		time.Sleep(time.Duration(10*(i+1)) * time.Millisecond)
	}
	return err
}

// tipoLedgerDoPendente mapeia o tipo do pendente para o tipo do ledger gravado na liberação.
func tipoLedgerDoPendente(tipo string) (models.TipoTransacao, string, bool) {
	switch tipo {
	case models.PendentePremio:
		return models.TipoPremioDesafio, "Prêmio do desafio", true
	case models.PendentePagamentoInstrutor:
		return models.TipoPagamentoInstrutor, "Pagamento de instrutor", true
	case models.PendenteReceitaAcademia:
		return models.TipoReceitaAcademia, "Parte da academia no desafio", true
	}
	return "", "", false
}

// ─── Job: liberar-pagamentos ─────────────────────────────────────────────────

// LiberarPagamentos credita, desafio a desafio (uma transação cada), os pendentes a_liberar com
// liberar_em vencido. Idempotente e seguro com N instâncias (trava do desafio + CAS a_liberar ->
// liberado). Se aparecer denúncia retentora aberta, retém em vez de liberar. Devolve quantos liberou.
func (s *moderacaoService) LiberarPagamentos(limite int) (int, error) {
	agora := s.now()
	ids, err := s.pendentes.DesafiosProntos(agora, limite)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, id := range ids {
		var n int
		pc := &posCommit{}
		err := comRetentativaDeadlock(func() error {
			n, pc = 0, &posCommit{}
			return s.db.Transaction(func(tx *gorm.DB) error {
				var err error
				n, err = s.liberarDesafioTx(tx, id, agora, pc)
				return err
			})
		})
		if err != nil {
			return total, fmt.Errorf("liberar pagamentos do desafio %d: %w", id, err)
		}
		pc.disparar()
		total += n
	}
	return total, nil
}

func (s *moderacaoService) liberarDesafioTx(tx *gorm.DB, idDesafio uint, agora time.Time, pc *posCommit) (int, error) {
	d, err := s.desafios.ComTx(tx).TravarPorID(idDesafio)
	if err != nil || d == nil {
		return 0, err
	}
	if d.Status != models.StatusEncerrado || d.ResultadoAnuladoEm != nil {
		return 0, nil // a anulação cancela os pendentes; nada a liberar
	}
	ps, err := s.pendentes.TravarLiberaveis(tx, idDesafio, agora)
	if err != nil || len(ps) == 0 {
		return 0, err
	}
	// Reconferência sob a trava: denúncia de desafio aberta depois do pódio (ou que correu com ele) retém.
	retentora, err := s.denuncias.RetentoraAberta(tx, idDesafio, 0)
	if err != nil {
		return 0, err
	}
	if retentora != nil {
		n, err := s.pendentes.Reter(tx, idDesafio, *retentora)
		if err != nil || n == 0 {
			return 0, err
		}
		return 0, s.registrar(tx, atorSistema, EventoPagamentoRetido, models.EntidadeDesafio, idDesafio, &idDesafio, "",
			map[string]any{"status": models.PendenteALiberar}, map[string]any{"status": models.PendenteRetido, "id_denuncia": *retentora, "pagamentos": n})
	}
	var usuarios []uint
	for _, p := range ps {
		if p.IDUsuario != nil {
			usuarios = append(usuarios, *p.IDUsuario)
		}
	}
	if err := s.carteira.TravarUsuarios(tx, usuarios); err != nil {
		return 0, err
	}
	titulo := d.Titulo
	for _, p := range ps {
		var idTransacao *uint
		if p.IDUsuario == nil {
			if err := s.receitas.RegistrarTaxaInscricao(tx, idDesafio, p.ValorCentavos); err != nil {
				return 0, err
			}
		} else {
			tipo, rotulo, ok := tipoLedgerDoPendente(p.Tipo)
			if !ok {
				return 0, fmt.Errorf("pendente %d com tipo %q inválido para usuário", p.ID, p.Tipo)
			}
			t, err := s.carteira.Creditar(tx, repositories.Lancamento{IDUsuario: *p.IDUsuario, Tipo: tipo,
				ValorCentavos: p.ValorCentavos, IDDesafio: &idDesafio, Descricao: limitar(rotulo+": "+titulo, 255)})
			if err != nil {
				return 0, err
			}
			idTransacao = &t.ID
		}
		ok, err := s.pendentes.MarcarLiberado(tx, p.ID, idTransacao, agora)
		if err != nil {
			return 0, err
		}
		if !ok {
			return 0, fmt.Errorf("pendente %d mudou de estado durante a liberação", p.ID)
		}
		if err := s.registrar(tx, atorSistema, EventoPagamentoLiberado, models.EntidadePagamento, p.ID, &idDesafio, "",
			map[string]any{"status": models.PendenteALiberar},
			map[string]any{"status": models.PendenteLiberado, "valor_centavos": p.ValorCentavos, "id_transacao": idTransacao}); err != nil {
			return 0, err
		}
		if p.IDUsuario != nil {
			uid, valor := *p.IDUsuario, p.ValorCentavos
			pc.add(func() {
				s.notif.Notificar(uid, models.NotifPagamentoLiberado, "Pagamento liberado",
					fmt.Sprintf("%s do desafio \"%s\" entraram no seu saldo.", reaisBR(valor), titulo), linkDesafio(idDesafio))
			})
		}
	}
	return len(ps), nil
}

// ─── Admin: anulação do resultado ────────────────────────────────────────────

// AnularResultado anula o resultado de um desafio encerrado (punição anulacao_resultado), numa transação.
func (s *moderacaoService) AnularResultado(ator AtorAdmin, idDesafio uint, req *models.AnularResultadoRequest) (*models.AnulacaoResponse, error) {
	motivo, err := motivoValido(req.Motivo)
	if err != nil {
		return nil, err
	}
	just, err := textoEntre(req.Justificativa, decisaoTextoMin, decisaoTextoMax, "A justificativa")
	if err != nil {
		return nil, err
	}
	if req.Modo != models.AnulacaoRelancarPodio && req.Modo != models.AnulacaoDevolverInscricoes {
		return nil, erroRequisicao("Modo inválido (use relancar_podio ou devolver_inscricoes)")
	}
	alvo, err := s.usuarios.BuscarPorID(req.IDUsuarioPunido)
	if err != nil {
		return nil, err
	}
	if alvo == nil {
		return nil, erroNaoEncontrado("Usuário punido não encontrado")
	}
	var resp *models.AnulacaoResponse
	var pc *posCommit
	err = comRetentativaDeadlock(func() error {
		resp, pc = &models.AnulacaoResponse{IDDesafio: idDesafio, Modo: req.Modo, Debitos: []models.DebitoResponse{}}, &posCommit{}
		return s.db.Transaction(func(tx *gorm.DB) error {
			return s.anularTx(tx, ator, idDesafio, req, just, motivo, resp, pc)
		})
	})
	if err != nil {
		return nil, err
	}
	pc.disparar()
	return resp, nil
}

func (s *moderacaoService) anularTx(tx *gorm.DB, ator AtorAdmin, idDesafio uint, req *models.AnularResultadoRequest,
	just, motivo string, resp *models.AnulacaoResponse, pc *posCommit) error {
	agora := s.now()
	repo := s.desafios.ComTx(tx)
	d, err := repo.TravarPorID(idDesafio)
	if err != nil {
		return err
	}
	if d == nil {
		return erroNaoEncontrado("Desafio não encontrado")
	}
	if d.ResultadoAnuladoEm != nil {
		return erroConflito("O resultado deste desafio já foi anulado")
	}
	if d.Status != models.StatusEncerrado {
		return erroConflito("Só o resultado de um desafio encerrado pode ser anulado")
	}
	pontos, temPodio, err := pontosDoPodioLancado(tx, d.ID)
	if err != nil {
		return err
	}
	if d.PodioLancadoEm == nil || !temPodio {
		return erroConflito("Este desafio foi encerrado antes da janela de contestação: o resultado não pode ser anulado por aqui")
	}
	// Denúncia de origem: decidida procedente na mesma transação (se ainda aberta e com o admin). Travada
	// ANTES dos pendentes (a decisão de denúncia trava denúncia -> pendentes; mesma ordem, sem deadlock).
	if req.IDDenuncia != nil {
		if err := s.decidirDenunciaDaAnulacao(tx, ator, *req.IDDenuncia, d.ID, req.IDUsuarioPunido, just, agora, pc); err != nil {
			return err
		}
	}
	pendentes, err := s.pendentes.TravarDoDesafio(tx, d.ID)
	if err != nil {
		return err
	}
	var liberados []models.PagamentoPendente
	for _, p := range pendentes {
		if p.Status == models.PendenteLiberado {
			liberados = append(liberados, p)
		}
	}
	if req.Modo == models.AnulacaoRelancarPodio && len(liberados) > 0 {
		return erroConflito("Já há pagamento deste desafio liberado: o pódio não pode ser relançado (use devolver_inscricoes)")
	}
	ok, err := s.desafioMod.AnularResultado(tx, d.ID, req.Modo, agora)
	if err != nil {
		return err
	}
	if !ok {
		return erroConflito("O resultado deste desafio já foi anulado")
	}
	participantes, err := repo.ListarParticipantes(d.ID)
	if err != nil {
		return err
	}
	// Trava todos os saldos que esta anulação vai mexer, em ordem de id (mesma ordem da liberação).
	travar := map[uint]bool{}
	for _, p := range liberados {
		if p.IDUsuario != nil {
			travar[*p.IDUsuario] = true
		}
	}
	if req.Modo == models.AnulacaoDevolverInscricoes {
		for _, p := range participantes {
			travar[p.IDUsuario] = true
		}
		travar[d.IDCriador] = true
	}
	if err := s.carteira.TravarUsuarios(tx, chavesOrdenadas(travar)); err != nil {
		return err
	}

	// 1) Pendentes ainda não liberados: cancelados (terminal).
	var cancelados []map[string]any
	for _, p := range pendentes {
		if p.Status == models.PendenteALiberar || p.Status == models.PendenteRetido {
			resp.PendentesCancelados++
			resp.CanceladoCentavos += p.ValorCentavos
			cancelados = append(cancelados, map[string]any{"id": p.ID, "tipo": p.Tipo, "id_usuario": p.IDUsuario, "valor_centavos": p.ValorCentavos, "status": p.Status})
		}
	}
	if n, err := s.pendentes.CancelarDoDesafio(tx, d.ID); err != nil {
		return err
	} else if int(n) != resp.PendentesCancelados {
		return fmt.Errorf("desafio %d: %d pendentes cancelados, esperado %d", d.ID, n, resp.PendentesCancelados)
	}
	if len(cancelados) > 0 {
		if err := s.registrar(tx, atorDoAdmin(ator), EventoPagamentoCancelado, models.EntidadeDesafio, d.ID, &d.ID, just,
			map[string]any{"pagamentos": cancelados}, map[string]any{"status": models.PendenteCancelado}); err != nil {
			return err
		}
	}

	// 2) devolver_inscricoes: entradas INTEGRAIS a todos os inscritos e a reserva inteira à academia
	// (antes dos estornos: o estorno do pódio pode usar o que acabou de voltar ao saldo).
	if req.Modo == models.AnulacaoDevolverInscricoes {
		if err := s.devolverInscricoesTx(tx, d, participantes, resp); err != nil {
			return err
		}
	}

	// 3) Pagamentos já liberados: estorno do saldo (estorno_anulacao, ligado à linha do crédito) e, se o
	// saldo não cobre, débito do restante. Taxa da plataforma liberada: estorno de receita (Q6).
	sort.SliceStable(liberados, func(i, j int) bool { return idOuZero(liberados[i].IDUsuario) < idOuZero(liberados[j].IDUsuario) })
	for _, p := range liberados {
		if err := s.estornarLiberadoTx(tx, ator, d, p, just, resp, pc); err != nil {
			return err
		}
	}

	// 4) Pontos do pódio desfeitos (exatamente os somados no lançamento; piso 0).
	uids := make([]string, 0, len(pontos))
	for k := range pontos {
		uids = append(uids, k)
	}
	sort.Strings(uids)
	for _, k := range uids {
		uid, err := strconv.ParseUint(k, 10, 64)
		if err != nil {
			return err
		}
		if err := s.desafioMod.DecrementarPontos(tx, uint(uid), pontos[k]); err != nil {
			return err
		}
	}

	// 5) relancar_podio (Q3): pódio removido, desafio volta a em_andamento (a academia dona relança).
	if req.Modo == models.AnulacaoRelancarPodio {
		if err := s.desafioMod.RemoverPodio(tx, d.ID); err != nil {
			return err
		}
		ok, err := s.desafioMod.ReabrirParaRelancamento(tx, d.ID)
		if err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("desafio %d: reabertura para relançamento não aplicada", d.ID)
		}
		resp.Status = string(models.StatusEmAndamento)
	} else {
		resp.Status = string(models.StatusEncerrado)
	}

	// 6) Punição anulacao_resultado (global, com o desafio), linha do tempo e auditoria.
	idAdm, idD := ator.ID, d.ID
	pun := &models.Punicao{IDDenuncia: req.IDDenuncia, IDUsuario: req.IDUsuarioPunido, Tipo: models.PunicaoAnulacaoResultado,
		Escopo: models.EscopoGlobal, IDDesafio: &idD, Inicio: agora, AplicadaPorAdmin: &idAdm, Justificativa: just}
	if err := s.aplicarPunicaoTx(tx, atorDoAdmin(ator), pun, agora); err != nil {
		return err
	}
	resp.IDPunicao = pun.ID
	s.avisarPunicao(pc, pun)
	if err := s.registrar(tx, atorDoAdmin(ator), EventoResultadoAnulado, models.EntidadeDesafio, d.ID, &d.ID, just,
		map[string]any{"status": d.Status, "podio_lancado_em": d.PodioLancadoEm},
		map[string]any{"status": resp.Status, "modo": req.Modo, "id_punicao": pun.ID, "pendentes_cancelados": resp.PendentesCancelados,
			"cancelado_centavos": resp.CanceladoCentavos, "estornado_centavos": resp.EstornadoCentavos,
			"taxa_estornada_centavos": resp.TaxaEstornadaCentavos, "inscricoes_devolvidas_centavos": resp.InscricoesDevolvidasCentavos,
			"reserva_devolvida_centavos": resp.ReservaDevolvidaCentavos, "debitos": len(resp.Debitos), "pontos_desfeitos": pontos}); err != nil {
		return err
	}
	if err := s.auditar(tx, ator, AcaoDesafioAnularResultado, AlvoDesafio, d.ID, motivo, map[string]any{
		"modo": req.Modo, "id_punicao": pun.ID, "id_usuario_punido": req.IDUsuarioPunido, "id_denuncia": req.IDDenuncia,
		"cancelado_centavos": resp.CanceladoCentavos, "estornado_centavos": resp.EstornadoCentavos,
		"taxa_estornada_centavos": resp.TaxaEstornadaCentavos, "inscricoes_devolvidas_centavos": resp.InscricoesDevolvidasCentavos,
		"reserva_devolvida_centavos": resp.ReservaDevolvidaCentavos, "debitos": len(resp.Debitos),
	}); err != nil {
		return err
	}
	s.avisarAnulacao(pc, d, participantes, req.Modo)
	return nil
}

func idOuZero(p *uint) uint {
	if p == nil {
		return 0
	}
	return *p
}

func chavesOrdenadas(m map[uint]bool) []uint {
	out := make([]uint, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// decidirDenunciaDaAnulacao liga a anulação à denúncia: aberta com o admin -> procedente agora; já
// procedente -> só referência. Precisa ser do mesmo desafio e contra o usuário punido.
func (s *moderacaoService) decidirDenunciaDaAnulacao(tx *gorm.DB, ator AtorAdmin, idDenuncia, idDesafio, idPunido uint,
	just string, agora time.Time, pc *posCommit) error {
	den, err := s.denuncias.Buscar(tx, idDenuncia, true)
	if err != nil {
		return err
	}
	if den == nil {
		return erroNaoEncontrado("Denúncia não encontrada")
	}
	if den.IDDesafio == nil || *den.IDDesafio != idDesafio || den.IDDenunciado != idPunido {
		return erroConflito("A denúncia precisa ser deste desafio e contra o usuário punido")
	}
	switch den.Status {
	case models.DenunciaProcedente:
		return nil
	case models.DenunciaAberta, models.DenunciaEmAnalise:
		if den.Jurisdicao != models.JurisdicaoAdmin {
			return erroConflito("A denúncia está com a academia julgadora até o prazo (ou até ela encaminhar)")
		}
		ok, err := s.denuncias.Decidir(tx, den.ID, models.DenunciaProcedente, just, agora, models.JurisdicaoAdmin, nil)
		if err != nil {
			return err
		}
		if !ok {
			return erroConflito("A denúncia já foi decidida")
		}
		if err := s.registrar(tx, atorDoAdmin(ator), EventoDenunciaDecidida, models.EntidadeDenuncia, den.ID, den.IDDesafio, just,
			map[string]any{"status": den.Status}, map[string]any{"status": models.DenunciaProcedente, "anulacao_resultado": true}); err != nil {
			return err
		}
		s.avisarDecisao(pc, den, models.DenunciaProcedente)
		return nil
	default:
		return erroConflito("A denúncia já foi julgada improcedente ou arquivada")
	}
}

// devolverInscricoesTx (modo devolver_inscricoes, Q6): entrada INTEGRAL a cada inscrito (estorno_desafio)
// e a reserva inteira à academia (estorno_reserva). Os créditos abatem débitos abertos (Q5).
func (s *moderacaoService) devolverInscricoesTx(tx *gorm.DB, d *models.Desafio, participantes []models.DesafioParticipante,
	resp *models.AnulacaoResponse) error {
	ordenados := append([]models.DesafioParticipante(nil), participantes...)
	sort.SliceStable(ordenados, func(i, j int) bool { return ordenados[i].IDUsuario < ordenados[j].IDUsuario })
	if d.ValorEntradaCentavos > 0 {
		for _, p := range ordenados {
			if _, err := s.carteira.Creditar(tx, repositories.Lancamento{IDUsuario: p.IDUsuario, Tipo: models.TipoEstornoDesafio,
				ValorCentavos: d.ValorEntradaCentavos, IDDesafio: &d.ID,
				Descricao: limitar("Reembolso (resultado anulado): "+d.Titulo, 255)}); err != nil {
				return err
			}
			resp.InscricoesDevolvidasCentavos += d.ValorEntradaCentavos
		}
	}
	if d.ReservaAcademiaCentavos > 0 {
		if _, err := s.carteira.Creditar(tx, repositories.Lancamento{IDUsuario: d.IDCriador, Tipo: models.TipoEstornoReserva,
			ValorCentavos: d.ReservaAcademiaCentavos, IDDesafio: &d.ID,
			Descricao: limitar("Reserva devolvida (resultado anulado): "+d.Titulo, 255)}); err != nil {
			return err
		}
		resp.ReservaDevolvidaCentavos = d.ReservaAcademiaCentavos
	}
	return s.registrar(tx, atorSistema, EventoInscricoesDevolvidas, models.EntidadeDesafio, d.ID, &d.ID, "", nil,
		map[string]any{"inscritos": len(participantes), "entrada_centavos": d.ValorEntradaCentavos,
			"inscricoes_devolvidas_centavos": resp.InscricoesDevolvidasCentavos, "reserva_devolvida_centavos": resp.ReservaDevolvidaCentavos})
}

// estornarLiberadoTx estorna um pagamento já liberado: usuário -> tira do saldo o que der (estorno_anulacao
// com id_transacao_origem = o crédito liberado; UNIQUE impede estornar duas vezes) e o restante vira
// débito; plataforma -> estorno_taxa (receita negativa, Q6). A linha do pendente continua `liberado`.
func (s *moderacaoService) estornarLiberadoTx(tx *gorm.DB, ator AtorAdmin, d *models.Desafio, p models.PagamentoPendente,
	just string, resp *models.AnulacaoResponse, pc *posCommit) error {
	if p.IDUsuario == nil {
		if err := s.desafioMod.RegistrarEstornoTaxa(tx, d.ID, p.ValorCentavos); err != nil {
			return err
		}
		resp.TaxaEstornadaCentavos += p.ValorCentavos
		return s.registrar(tx, atorDoAdmin(ator), EventoPagamentoEstornado, models.EntidadePagamento, p.ID, &d.ID, just,
			map[string]any{"status": models.PendenteLiberado, "tipo": p.Tipo},
			map[string]any{"estorno_taxa_centavos": p.ValorCentavos})
	}
	uid := *p.IDUsuario
	saldo, err := s.carteira.SaldoTravado(tx, uid)
	if err != nil {
		return err
	}
	estorno := min(max(saldo, 0), p.ValorCentavos)
	var idEstorno *uint
	if estorno > 0 {
		t, err := s.carteira.Debitar(tx, repositories.Lancamento{IDUsuario: uid, Tipo: models.TipoEstornoAnulacao,
			ValorCentavos: estorno, IDDesafio: &d.ID, IDTransacaoOrigem: p.IDTransacao,
			Descricao: limitar("Estorno (resultado anulado): "+d.Titulo, 255)})
		if err != nil {
			return err
		}
		idEstorno = &t.ID
		resp.EstornadoCentavos += estorno
	}
	var idDebito *uint
	if falta := p.ValorCentavos - estorno; falta > 0 {
		deb, err := s.debitos.Criar(tx, uid, d.ID, falta, limitar(fmt.Sprintf("Anulação do resultado do desafio #%d", d.ID), 255))
		if err != nil {
			return err
		}
		idDebito = &deb.ID
		resp.Debitos = append(resp.Debitos, debitoResponse(*deb, d.Titulo, nil))
		if err := s.registrar(tx, atorDoAdmin(ator), EventoDebitoCriado, models.EntidadeDebito, deb.ID, &d.ID, just, nil,
			map[string]any{"id_usuario": uid, "valor_centavos": falta, "id_pagamento": p.ID}); err != nil {
			return err
		}
		titulo := d.Titulo
		pc.add(func() {
			s.notif.Notificar(uid, models.NotifDebitoCriado, "Débito na sua carteira",
				fmt.Sprintf("O resultado do desafio \"%s\" foi anulado e %s não puderam ser estornados do seu saldo. "+
					"O valor fica como débito: os próximos créditos o abatem e o saque fica bloqueado até a quitação.", titulo, reaisBR(falta)),
				"/carteira")
		})
	}
	return s.registrar(tx, atorDoAdmin(ator), EventoPagamentoEstornado, models.EntidadePagamento, p.ID, &d.ID, just,
		map[string]any{"status": models.PendenteLiberado, "tipo": p.Tipo, "valor_centavos": p.ValorCentavos},
		map[string]any{"estornado_centavos": estorno, "id_transacao_estorno": idEstorno, "id_debito": idDebito})
}

// avisarAnulacao notifica inscritos, instrutor e academia depois do commit.
func (s *moderacaoService) avisarAnulacao(pc *posCommit, d *models.Desafio, participantes []models.DesafioParticipante, modo string) {
	ids := make([]uint, 0, len(participantes))
	for _, p := range participantes {
		ids = append(ids, p.IDUsuario)
	}
	titulo, idDesafio, academia := d.Titulo, d.ID, d.IDCriador
	var instrutor uint
	if d.IDInstrutor != nil {
		if i, err := s.instrutores.BuscarPorID(*d.IDInstrutor); err == nil && i != nil {
			instrutor = i.IDUsuario
		}
	}
	pc.add(func() {
		msg := fmt.Sprintf("O resultado do desafio \"%s\" foi anulado pela equipe GymScore. O pódio será relançado pela academia.", titulo)
		if modo == models.AnulacaoDevolverInscricoes {
			msg = fmt.Sprintf("O resultado do desafio \"%s\" foi anulado pela equipe GymScore e as inscrições foram devolvidas.", titulo)
		}
		s.notif.NotificarVarios(ids, models.NotifResultadoAnulado, "Resultado anulado", msg, linkDesafio(idDesafio))
		if instrutor != 0 {
			s.notif.Notificar(instrutor, models.NotifResultadoAnulado, "Resultado anulado", msg, linkDesafio(idDesafio))
		}
		msgAcad := msg
		if modo == models.AnulacaoRelancarPodio {
			msgAcad = fmt.Sprintf("O resultado do desafio \"%s\" foi anulado pela equipe GymScore. Relance o pódio no detalhe do desafio.", titulo)
		}
		s.notif.Notificar(academia, models.NotifResultadoAnulado, "Resultado anulado", msgAcad, linkDesafio(idDesafio))
	})
}

// ─── Admin: filas de pagamentos pendentes e débitos ──────────────────────────

func debitoResponse(d models.UsuarioDebito, titulo string, u *models.UsuarioRefModeracao) models.DebitoResponse {
	return models.DebitoResponse{ID: d.ID, IDUsuario: d.IDUsuario, Usuario: u, IDDesafio: d.IDDesafio, TituloDesafio: titulo,
		ValorCentavos: d.ValorCentavos, SaldoRestanteCentavos: d.SaldoRestanteCentavos, Motivo: d.Motivo,
		Quitado: d.QuitadoEm != nil, CriadoEm: d.CriadoEm, QuitadoEm: d.QuitadoEm}
}

// PendenteParaResponse monta o DTO do pendente; comAdmin inclui retenção, transação e usuário.
func PendenteParaResponse(p models.PagamentoPendente, titulo string, u *models.UsuarioRefModeracao, comAdmin bool) models.PagamentoPendenteResponse {
	r := models.PagamentoPendenteResponse{ID: p.ID, IDDesafio: p.IDDesafio, TituloDesafio: titulo, IDUsuario: p.IDUsuario,
		Tipo: p.Tipo, ValorCentavos: p.ValorCentavos, Status: p.Status, LiberarEm: p.LiberarEm, LiberadoEm: p.LiberadoEm, CriadoEm: p.CriadoEm}
	if comAdmin {
		r.Usuario, r.IDDenunciaRetencao, r.IDTransacao = u, p.IDDenunciaRetencao, p.IDTransacao
	}
	return r
}

// TitulosDesafios devolve id -> título dos desafios pedidos.
func TitulosDesafios(db *gorm.DB, ids []uint) (map[uint]string, error) {
	out := map[uint]string{}
	if len(ids) == 0 {
		return out, nil
	}
	var linhas []struct {
		ID     uint
		Titulo string
	}
	err := db.Model(&models.Desafio{}).Select("id, titulo").Where("id IN ?", ids).Scan(&linhas).Error
	for _, l := range linhas {
		out[l.ID] = l.Titulo
	}
	return out, err
}

func (s *moderacaoService) PagamentosPendentes(f repositories.FiltroPendentes) (*models.PaginaAdmin[models.PagamentoPendenteResponse], error) {
	ps, total, err := s.pendentes.Listar(f)
	if err != nil {
		return nil, err
	}
	var idsD, idsU []uint
	for _, p := range ps {
		idsD = append(idsD, p.IDDesafio)
		if p.IDUsuario != nil {
			idsU = append(idsU, *p.IDUsuario)
		}
	}
	titulos, err := TitulosDesafios(s.db, idsD)
	if err != nil {
		return nil, err
	}
	us, err := s.consultas.UsuariosPorID(idsU)
	if err != nil {
		return nil, err
	}
	itens := make([]models.PagamentoPendenteResponse, 0, len(ps))
	for _, p := range ps {
		var u *models.UsuarioRefModeracao
		if p.IDUsuario != nil {
			r := refUsuario(us[*p.IDUsuario], true)
			r.ID = *p.IDUsuario
			u = &r
		}
		itens = append(itens, PendenteParaResponse(p, titulos[p.IDDesafio], u, true))
	}
	return paginaDe(itens, total, f.Pagina, f.Limite), nil
}

func (s *moderacaoService) Debitos(abertos bool, idUsuario *uint, pagina, limite int) (*models.PaginaAdmin[models.DebitoResponse], error) {
	ds, total, err := s.debitos.Listar(abertos, idUsuario, pagina, limite)
	if err != nil {
		return nil, err
	}
	var idsD, idsU []uint
	for _, d := range ds {
		idsD, idsU = append(idsD, d.IDDesafio), append(idsU, d.IDUsuario)
	}
	titulos, err := TitulosDesafios(s.db, idsD)
	if err != nil {
		return nil, err
	}
	us, err := s.consultas.UsuariosPorID(idsU)
	if err != nil {
		return nil, err
	}
	itens := make([]models.DebitoResponse, 0, len(ds))
	for _, d := range ds {
		r := refUsuario(us[d.IDUsuario], true)
		r.ID = d.IDUsuario
		itens = append(itens, debitoResponse(d, titulos[d.IDDesafio], &r))
	}
	return paginaDe(itens, total, pagina, limite), nil
}
