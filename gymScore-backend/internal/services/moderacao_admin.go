package services

import (
	"fmt"
	"time"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"

	"gorm.io/gorm"
)

// ─── Admin: denúncias ────────────────────────────────────────────────────────

func (s *moderacaoService) Denuncias(f repositories.FiltroDenuncias) (*models.PaginaAdmin[models.DenunciaResponse], error) {
	ds, total, err := s.denuncias.Listar(f)
	if err != nil {
		return nil, err
	}
	itens, err := s.denunciasResponse(ds, paraAdmin, 0)
	if err != nil {
		return nil, err
	}
	return paginaDe(itens, total, f.Pagina, f.Limite), nil
}

func eventosResponse(es []models.ModeracaoEvento) []models.EventoResponse {
	out := make([]models.EventoResponse, 0, len(es))
	for _, e := range es {
		out = append(out, models.EventoResponse{ID: e.ID, AtorPapel: e.AtorPapel, IDAtor: e.IDAtor, IDAdmin: e.IDAdmin, Acao: e.Acao,
			Entidade: e.Entidade, IDEntidade: e.IDEntidade, IDDesafio: e.IDDesafio, Justificativa: e.Justificativa, Dados: e.Dados,
			CriadoEm: e.CriadoEm})
	}
	return out
}

func (s *moderacaoService) DenunciaAdmin(id uint) (*models.DenunciaAdminDetalhe, error) {
	d, err := s.denuncias.Buscar(nil, id, false)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, erroNaoEncontrado("Denúncia não encontrada")
	}
	r, err := s.denunciaResponse(d, paraAdmin, 0)
	if err != nil {
		return nil, err
	}
	ps, _, err := s.punicoes.Listar(repositories.FiltroPunicoes{IDDenuncia: &d.ID, Pagina: 1, Limite: 100})
	if err != nil {
		return nil, err
	}
	punicoes, err := s.punicoesResponse(ps, true)
	if err != nil {
		return nil, err
	}
	evs, _, err := s.eventos.PorEntidade(models.EntidadeDenuncia, d.ID, 1, 100)
	if err != nil {
		return nil, err
	}
	outras, err := s.consultas.DenunciasDoDenunciado(d.IDDenunciado, d.ID, 20)
	if err != nil {
		return nil, err
	}
	outrasR, err := s.denunciasResponse(outras, paraAdmin, 0)
	if err != nil {
		return nil, err
	}
	reinc, err := s.punicoes.ContarDoUsuarioDesde(d.IDDenunciado, models.PunicaoAdvertencia, s.now().Add(-reincidenciaJanela))
	if err != nil {
		return nil, err
	}
	return &models.DenunciaAdminDetalhe{Denuncia: *r, Punicoes: punicoes, Eventos: eventosResponse(evs),
		OutrasDoDenunciado: outrasR, ReincidenciaAdvertencias90d: reinc}, nil
}

func (s *moderacaoService) AssumirAdmin(ator AtorAdmin, id uint, motivo string) (*models.DenunciaResponse, error) {
	motivo, err := motivoValido(motivo)
	if err != nil {
		return nil, err
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		d, err := s.denuncias.Buscar(tx, id, true)
		if err != nil {
			return err
		}
		if d == nil {
			return erroNaoEncontrado("Denúncia não encontrada")
		}
		if d.Jurisdicao != models.JurisdicaoAdmin {
			return erroConflito("A denúncia está com a academia julgadora até o prazo (ou até ela encaminhar)")
		}
		ok, err := s.denuncias.AssumirComoAdmin(tx, id, ator.ID)
		if err != nil {
			return err
		}
		if !ok {
			return erroConflito("A denúncia já foi decidida")
		}
		if err := s.registrar(tx, atorDoAdmin(ator), EventoDenunciaAssumida, models.EntidadeDenuncia, id, d.IDDesafio, motivo,
			map[string]any{"status": d.Status, "id_responsavel_admin": d.IDResponsavelAdmin},
			map[string]any{"status": models.DenunciaEmAnalise, "id_responsavel_admin": ator.ID}); err != nil {
			return err
		}
		return s.auditar(tx, ator, AcaoDenunciaAssumir, AlvoDenuncia, id, motivo, map[string]any{"status_anterior": d.Status})
	})
	if err != nil {
		return nil, err
	}
	d, err := s.denuncias.Buscar(nil, id, false)
	if err != nil || d == nil {
		return nil, err
	}
	return s.denunciaResponse(d, paraAdmin, 0)
}

// validarPunicaoAdmin confere tipo/escopo/fim de uma punição aplicada pelo admin e monta o model
// (sem usuário, denúncia e início). Anulação de resultado tem rota própria (parte 2).
func (s *moderacaoService) validarPunicaoAdmin(tipo string, idAcademia *uint, fim *time.Time, agora time.Time) (*models.Punicao, error) {
	p := &models.Punicao{Tipo: tipo, Escopo: models.EscopoGlobal, Fim: fim}
	switch tipo {
	case models.PunicaoAdvertencia:
		if fim != nil {
			return nil, erroRequisicao("Advertência não tem data de fim")
		}
		if idAcademia != nil {
			p.Escopo, p.IDAcademia = models.EscopoAcademia, idAcademia
		}
	case models.PunicaoSuspensao:
		if fim == nil || !fim.After(agora) {
			return nil, erroRequisicao("Suspensão exige uma data de fim no futuro")
		}
		if idAcademia != nil {
			return nil, erroRequisicao("Suspensão é sempre global (sem academia)")
		}
	case models.PunicaoBanimento:
		if fim != nil {
			return nil, erroRequisicao("Banimento não tem data de fim (com prazo, use suspensão)")
		}
		if idAcademia != nil {
			return nil, erroRequisicao("Banimento é sempre global (sem academia)")
		}
	case models.PunicaoBloqueioAcademia:
		if idAcademia == nil {
			return nil, erroRequisicao("Informe a academia do bloqueio (id_academia)")
		}
		if fim != nil && !fim.After(agora) {
			return nil, erroRequisicao("O fim do bloqueio deve estar no futuro")
		}
		p.Escopo, p.IDAcademia = models.EscopoAcademia, idAcademia
	case models.PunicaoAnulacaoResultado:
		return nil, erroRequisicao("A anulação de resultado tem rota própria (POST /api/admin/desafios/:id/anular-resultado)")
	default:
		return nil, erroRequisicao("Tipo de punição inválido")
	}
	if p.IDAcademia != nil {
		dono, err := s.consultas.UsuarioDaAcademia(*p.IDAcademia)
		if err != nil {
			return nil, err
		}
		if dono == nil {
			return nil, erroNaoEncontrado("Academia não encontrada")
		}
	}
	return p, nil
}

func (s *moderacaoService) DecidirAdmin(ator AtorAdmin, id uint, req *models.DecidirDenunciaAdminRequest) (*models.DenunciaAdminDetalhe, error) {
	motivo, err := motivoValido(req.Motivo)
	if err != nil {
		return nil, err
	}
	if !statusDecisaoValido(req.Status) {
		return nil, erroRequisicao("Status inválido (use procedente, improcedente ou arquivada)")
	}
	texto, err := textoEntre(req.DecisaoTexto, decisaoTextoMin, decisaoTextoMax, "O texto da decisão")
	if err != nil {
		return nil, err
	}
	if len(req.Punicoes) > 0 && req.Status != models.DenunciaProcedente {
		return nil, erroRequisicao("Só denúncia procedente gera punição")
	}
	if len(req.Punicoes) > punicoesPorDecisaoMax {
		return nil, erroRequisicao(fmt.Sprintf("No máximo %d punições por decisão", punicoesPorDecisaoMax))
	}
	agora := s.now()
	modelos := make([]*models.Punicao, 0, len(req.Punicoes))
	for _, pr := range req.Punicoes {
		p, err := s.validarPunicaoAdmin(pr.Tipo, pr.IDAcademia, pr.Fim, agora)
		if err != nil {
			return nil, err
		}
		modelos = append(modelos, p)
	}
	pc := &posCommit{}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		d, err := s.denuncias.Buscar(tx, id, true)
		if err != nil {
			return err
		}
		if d == nil {
			return erroNaoEncontrado("Denúncia não encontrada")
		}
		if d.Jurisdicao != models.JurisdicaoAdmin {
			return erroConflito("A denúncia está com a academia julgadora até o prazo (ou até ela encaminhar)")
		}
		ok, err := s.denuncias.Decidir(tx, id, req.Status, texto, agora, models.JurisdicaoAdmin, nil)
		if err != nil {
			return err
		}
		if !ok {
			return erroConflito("A denúncia já foi decidida")
		}
		idsPunicoes := make([]uint, 0, len(modelos))
		idAdm := ator.ID
		for _, p := range modelos {
			p.IDDenuncia, p.IDUsuario, p.IDDesafio, p.Inicio, p.AplicadaPorAdmin, p.Justificativa = &d.ID, d.IDDenunciado, d.IDDesafio, agora, &idAdm, texto
			if err := s.aplicarPunicaoTx(tx, atorDoAdmin(ator), p, agora); err != nil {
				return err
			}
			idsPunicoes = append(idsPunicoes, p.ID)
			s.avisarPunicao(pc, p)
		}
		if err := s.registrar(tx, atorDoAdmin(ator), EventoDenunciaDecidida, models.EntidadeDenuncia, id, d.IDDesafio, texto,
			map[string]any{"status": d.Status}, map[string]any{"status": req.Status, "punicoes": idsPunicoes}); err != nil {
			return err
		}
		if err := s.liberarRetencaoTx(tx, d); err != nil {
			return err
		}
		s.avisarDecisao(pc, d, req.Status)
		return s.auditar(tx, ator, AcaoDenunciaDecidir, AlvoDenuncia, id, motivo,
			map[string]any{"status": req.Status, "punicoes": idsPunicoes, "id_denunciado": d.IDDenunciado})
	})
	if err != nil {
		return nil, err
	}
	pc.disparar()
	return s.DenunciaAdmin(id)
}

// ─── Admin: punições ─────────────────────────────────────────────────────────

func (s *moderacaoService) Punicoes(f repositories.FiltroPunicoes) (*models.PaginaAdmin[models.PunicaoResponse], error) {
	if f.Vigentes {
		f.Agora = s.now()
	}
	ps, total, err := s.punicoes.Listar(f)
	if err != nil {
		return nil, err
	}
	itens, err := s.punicoesResponse(ps, true)
	if err != nil {
		return nil, err
	}
	return paginaDe(itens, total, f.Pagina, f.Limite), nil
}

func (s *moderacaoService) AplicarPunicao(ator AtorAdmin, req *models.AplicarPunicaoRequest) (*models.PunicaoResponse, error) {
	motivo, err := motivoValido(req.Motivo)
	if err != nil {
		return nil, err
	}
	just, err := textoEntre(req.Justificativa, decisaoTextoMin, decisaoTextoMax, "A justificativa")
	if err != nil {
		return nil, err
	}
	agora := s.now()
	p, err := s.validarPunicaoAdmin(req.Tipo, req.IDAcademia, req.Fim, agora)
	if err != nil {
		return nil, err
	}
	alvo, err := s.usuarios.BuscarPorID(req.IDUsuario)
	if err != nil {
		return nil, err
	}
	if alvo == nil {
		return nil, erroNaoEncontrado("Usuário não encontrado")
	}
	idAdm := ator.ID
	p.IDUsuario, p.Inicio, p.AplicadaPorAdmin, p.Justificativa = req.IDUsuario, agora, &idAdm, just
	pc := &posCommit{}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if req.IDDenuncia != nil {
			d, err := s.denuncias.Buscar(tx, *req.IDDenuncia, true)
			if err != nil {
				return err
			}
			if d == nil {
				return erroNaoEncontrado("Denúncia não encontrada")
			}
			if d.Status != models.DenunciaProcedente || d.IDDenunciado != req.IDUsuario {
				return erroConflito("Punição ligada a denúncia exige denúncia procedente contra o mesmo usuário")
			}
			p.IDDenuncia, p.IDDesafio = &d.ID, d.IDDesafio
		}
		if err := s.aplicarPunicaoTx(tx, atorDoAdmin(ator), p, agora); err != nil {
			return err
		}
		s.avisarPunicao(pc, p)
		return s.auditar(tx, ator, AcaoPunicaoAplicar, AlvoUsuario, req.IDUsuario, motivo,
			map[string]any{"id_punicao": p.ID, "tipo": p.Tipo, "fim": p.Fim, "id_academia": p.IDAcademia, "id_denuncia": p.IDDenuncia})
	})
	if err != nil {
		return nil, err
	}
	pc.disparar()
	salva, err := s.punicoes.Buscar(nil, p.ID, false)
	if err != nil || salva == nil {
		return nil, err
	}
	return s.punicaoResponse(salva, true)
}

func (s *moderacaoService) avisarRevogacao(pc *posCommit, p *models.Punicao, motivo string) {
	id, tipo := p.IDUsuario, p.Tipo
	pc.add(func() {
		s.notif.Notificar(id, models.NotifPunicaoRevogada, "Punição revogada",
			limitar(fmt.Sprintf("Sua %s foi revogada. Motivo: %s", rotuloPunicao(tipo), motivo), 500), "/minhas-punicoes")
	})
}

func (s *moderacaoService) RevogarPunicao(ator AtorAdmin, id uint, motivo string) (*models.PunicaoResponse, error) {
	motivo, err := motivoValido(motivo)
	if err != nil {
		return nil, err
	}
	pc := &posCommit{}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		p, err := s.punicoes.Buscar(tx, id, true)
		if err != nil {
			return err
		}
		if p == nil {
			return erroNaoEncontrado("Punição não encontrada")
		}
		if p.Tipo == models.PunicaoAnulacaoResultado {
			// Q4: a anulação é irreversível financeiramente; só o recurso deferido limpa o histórico.
			return erroConflito("Anulação de resultado não pode ser revogada diretamente (só por recurso deferido, sem efeito financeiro)")
		}
		ok, err := s.revogarPunicaoTx(tx, ator, p, motivo, s.now())
		if err != nil {
			return err
		}
		if !ok {
			return erroConflito("A punição já foi revogada")
		}
		s.avisarRevogacao(pc, p, motivo)
		return s.auditar(tx, ator, AcaoPunicaoRevogar, AlvoPunicao, id, motivo, map[string]any{"tipo": p.Tipo, "id_usuario": p.IDUsuario})
	})
	if err != nil {
		return nil, err
	}
	pc.disparar()
	p, err := s.punicoes.Buscar(nil, id, false)
	if err != nil || p == nil {
		return nil, err
	}
	return s.punicaoResponse(p, true)
}

// ─── Admin: recursos ─────────────────────────────────────────────────────────

func (s *moderacaoService) filaRecursos(rs []models.Recurso) ([]models.RecursoFilaItem, error) {
	ids := make([]uint, 0, len(rs))
	for _, rc := range rs {
		ids = append(ids, rc.IDPunicao)
	}
	pm, err := s.consultas.PunicoesPorID(ids)
	if err != nil {
		return nil, err
	}
	ps := make([]models.Punicao, 0, len(rs))
	for _, rc := range rs {
		ps = append(ps, pm[rc.IDPunicao])
	}
	presp, err := s.punicoesResponse(ps, true)
	if err != nil {
		return nil, err
	}
	out := make([]models.RecursoFilaItem, 0, len(rs))
	for i := range rs {
		out = append(out, models.RecursoFilaItem{Recurso: *recursoResponse(&rs[i], true), Punicao: presp[i]})
	}
	return out, nil
}

func (s *moderacaoService) Recursos(status string, pagina, limite int) (*models.PaginaAdmin[models.RecursoFilaItem], error) {
	switch status {
	case "", models.RecursoPendente, models.RecursoDeferido, models.RecursoIndeferido:
	default:
		return nil, erroRequisicao("Status inválido (pendente, deferido ou indeferido)")
	}
	rs, total, err := s.punicoes.ListarRecursos(status, pagina, limite)
	if err != nil {
		return nil, err
	}
	itens, err := s.filaRecursos(rs)
	if err != nil {
		return nil, err
	}
	return paginaDe(itens, total, pagina, limite), nil
}

func (s *moderacaoService) JulgarRecurso(ator AtorAdmin, id uint, req *models.JulgarRecursoRequest) (*models.RecursoFilaItem, error) {
	motivo, err := motivoValido(req.Motivo)
	if err != nil {
		return nil, err
	}
	if req.Status != models.RecursoDeferido && req.Status != models.RecursoIndeferido {
		return nil, erroRequisicao("Status inválido (deferido ou indeferido)")
	}
	texto, err := textoEntre(req.DecisaoTexto, decisaoTextoMin, decisaoTextoMax, "O texto da decisão")
	if err != nil {
		return nil, err
	}
	agora := s.now()
	pc := &posCommit{}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Ordem de travas: recurso -> punição -> usuário (mesma de RevogarPunicao a partir da punição).
		rc, err := s.punicoes.BuscarRecurso(tx, id, true)
		if err != nil {
			return err
		}
		if rc == nil {
			return erroNaoEncontrado("Recurso não encontrado")
		}
		ok, err := s.punicoes.JulgarRecurso(tx, id, req.Status, ator.ID, texto, agora)
		if err != nil {
			return err
		}
		if !ok {
			return erroConflito("O recurso já foi julgado")
		}
		p, err := s.punicoes.Buscar(tx, rc.IDPunicao, true)
		if err != nil {
			return err
		}
		revogou := false
		if req.Status == models.RecursoDeferido && p.RevogadaEm == nil {
			// Deferido = punição revogada. Na anulação de resultado só o histórico muda (Q4).
			if revogou, err = s.revogarPunicaoTx(tx, ator, p, texto, agora); err != nil {
				return err
			}
		}
		if err := s.registrar(tx, atorDoAdmin(ator), EventoRecursoJulgado, models.EntidadeRecurso, id, p.IDDesafio, texto,
			map[string]any{"status": models.RecursoPendente}, map[string]any{"status": req.Status, "punicao_revogada": revogou}); err != nil {
			return err
		}
		idPunido, idRecurso, st, aplicador := p.IDUsuario, id, req.Status, p.AplicadaPor
		pc.add(func() {
			s.notif.Notificar(idPunido, models.NotifRecursoJulgado, "Seu recurso foi julgado",
				limitar(fmt.Sprintf("Recurso #%d %s: %s", idRecurso, map[string]string{models.RecursoDeferido: "deferido (punição revogada)",
					models.RecursoIndeferido: "indeferido"}[st], texto), 500), "/minhas-punicoes")
			if revogou && aplicador != nil { // punição aplicada pela academia: ela fica sabendo da revogação
				s.notif.Notificar(*aplicador, models.NotifPunicaoRevogada, "Punição revogada em recurso",
					fmt.Sprintf("Uma punição aplicada pela sua academia foi revogada pelo recurso #%d.", idRecurso), "/academia/denuncias")
			}
		})
		return s.auditar(tx, ator, AcaoRecursoJulgar, AlvoRecurso, id, motivo,
			map[string]any{"status": req.Status, "id_punicao": p.ID, "punicao_revogada": revogou})
	})
	if err != nil {
		return nil, err
	}
	pc.disparar()
	rc, err := s.punicoes.BuscarRecurso(nil, id, false)
	if err != nil || rc == nil {
		return nil, err
	}
	itens, err := s.filaRecursos([]models.Recurso{*rc})
	if err != nil {
		return nil, err
	}
	return &itens[0], nil
}

// ─── Admin: linha do tempo ───────────────────────────────────────────────────

func (s *moderacaoService) Eventos(entidade string, idEntidade, idDesafio uint, pagina, limite int) (*models.PaginaAdmin[models.EventoResponse], error) {
	var (
		es    []models.ModeracaoEvento
		total int64
		err   error
	)
	switch {
	case idDesafio != 0:
		es, total, err = s.eventos.PorDesafio(idDesafio, pagina, limite)
	case entidade != "" && idEntidade != 0:
		switch entidade {
		case models.EntidadeDenuncia, models.EntidadePunicao, models.EntidadeRecurso, models.EntidadeDesafio,
			models.EntidadePagamento, models.EntidadeDebito, models.EntidadeUsuario:
		default:
			return nil, erroRequisicao("Entidade inválida")
		}
		es, total, err = s.eventos.PorEntidade(entidade, idEntidade, pagina, limite)
	default:
		return nil, erroRequisicao("Informe id_desafio ou entidade + id_entidade")
	}
	if err != nil {
		return nil, err
	}
	return paginaDe(eventosResponse(es), total, pagina, limite), nil
}

// ─── Admin: bloquear/desbloquear do painel (bloqueio unificado — MB3) ────────

func (s *moderacaoService) BloquearUsuario(ator AtorAdmin, id uint, motivo string) (bool, error) {
	motivo, err := motivoValido(motivo)
	if err != nil {
		return false, err
	}
	var mudou bool
	pc := &posCommit{}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Trava o usuário primeiro: dois "bloquear" simultâneos não criam dois banimentos.
		if _, existe, err := s.consultas.TravarUsuario(tx, id); err != nil {
			return err
		} else if !existe {
			return erroNaoEncontrado("Usuário não encontrado")
		}
		// O relógio é lido DEPOIS da trava: quem esperou o outro "bloquear" enxerga o banimento recém-criado
		// como vigente (inicio <= agora). Lido antes, o `agora` podia ser anterior ao `inicio` gravado pelo
		// vencedor e a corrida criava um segundo banimento.
		agora := s.now()
		vigentes, err := s.consultas.PunicoesVigentes(tx, id, models.PunicaoBanimento, agora)
		if err != nil || len(vigentes) > 0 {
			return err // já banido: idempotente, sem auditoria
		}
		idAdm := ator.ID
		p := &models.Punicao{IDUsuario: id, Tipo: models.PunicaoBanimento, Escopo: models.EscopoGlobal, Inicio: agora,
			AplicadaPorAdmin: &idAdm, Justificativa: motivo}
		if err := s.aplicarPunicaoTx(tx, atorDoAdmin(ator), p, agora); err != nil {
			return err
		}
		mudou = true
		s.avisarPunicao(pc, p)
		return s.auditar(tx, ator, AcaoUsuarioBloquear, AlvoUsuario, id, motivo, map[string]any{"id_punicao": p.ID, "tipo": p.Tipo})
	})
	if err != nil {
		return false, err
	}
	pc.disparar()
	return mudou, nil
}

func (s *moderacaoService) DesbloquearUsuario(ator AtorAdmin, id uint, motivo string) (bool, error) {
	motivo, err := motivoValido(motivo)
	if err != nil {
		return false, err
	}
	agora := s.now()
	var mudou bool
	pc := &posCommit{}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if _, existe, err := s.consultas.TravarUsuario(tx, id); err != nil {
			return err
		} else if !existe {
			return erroNaoEncontrado("Usuário não encontrado")
		}
		vigentes, err := s.consultas.PunicoesVigentes(tx, id, models.PunicaoBanimento, agora)
		if err != nil || len(vigentes) == 0 {
			return err
		}
		ids := make([]uint, 0, len(vigentes))
		for i := range vigentes {
			ok, err := s.revogarPunicaoTx(tx, ator, &vigentes[i], motivo, agora)
			if err != nil {
				return err
			}
			if ok {
				ids = append(ids, vigentes[i].ID)
				s.avisarRevogacao(pc, &vigentes[i], motivo)
			}
		}
		mudou = len(ids) > 0
		if !mudou {
			return nil
		}
		return s.auditar(tx, ator, AcaoUsuarioDesbloquear, AlvoUsuario, id, motivo, map[string]any{"punicoes_revogadas": ids})
	})
	if err != nil {
		return false, err
	}
	pc.disparar()
	return mudou, nil
}
