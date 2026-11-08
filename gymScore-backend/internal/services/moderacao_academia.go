package services

import (
	"fmt"
	"time"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"

	"gorm.io/gorm"
)

// ─── Peças comuns às decisões (academia e admin) ─────────────────────────────

// aplicarPunicaoTx insere a punição, recalcula o estado da conta quando ela mexe no acesso (suspensão,
// banimento — token_versao+1 quando endurece, derrubando as sessões) e grava o evento.
func (s *moderacaoService) aplicarPunicaoTx(tx *gorm.DB, ator atorEvento, p *models.Punicao, agora time.Time) error {
	if err := s.punicoes.Aplicar(tx, p); err != nil {
		return err
	}
	depois := map[string]any{"tipo": p.Tipo, "escopo": p.Escopo, "inicio": p.Inicio, "fim": p.Fim, "id_usuario": p.IDUsuario}
	if p.IDAcademia != nil {
		depois["id_academia"] = *p.IDAcademia
	}
	if p.IDDenuncia != nil {
		depois["id_denuncia"] = *p.IDDenuncia
	}
	if p.Tipo == models.PunicaoSuspensao || p.Tipo == models.PunicaoBanimento {
		estado, endureceu, err := s.punicoes.RecalcularEstadoConta(tx, p.IDUsuario, agora)
		if err != nil {
			return err
		}
		depois["conta"] = map[string]any{"banido_em": estado.BanidoEm, "suspenso_ate": estado.SuspensoAte, "sessoes_derrubadas": endureceu}
	}
	return s.registrar(tx, ator, EventoPunicaoAplicada, models.EntidadePunicao, p.ID, p.IDDesafio, p.Justificativa, nil, depois)
}

// revogarPunicaoTx revoga (CAS) e recalcula o estado da conta. false = já estava revogada.
func (s *moderacaoService) revogarPunicaoTx(tx *gorm.DB, ator AtorAdmin, p *models.Punicao, motivo string, agora time.Time) (bool, error) {
	ok, err := s.punicoes.Revogar(tx, p.ID, ator.ID, motivo, agora)
	if err != nil || !ok {
		return false, err
	}
	depois := map[string]any{"revogada_em": agora}
	if p.Tipo == models.PunicaoSuspensao || p.Tipo == models.PunicaoBanimento {
		estado, _, err := s.punicoes.RecalcularEstadoConta(tx, p.IDUsuario, agora)
		if err != nil {
			return false, err
		}
		depois["conta"] = map[string]any{"banido_em": estado.BanidoEm, "suspenso_ate": estado.SuspensoAte}
	}
	return true, s.registrar(tx, atorDoAdmin(ator), EventoPunicaoRevogada, models.EntidadePunicao, p.ID, p.IDDesafio, motivo,
		map[string]any{"revogada_em": nil}, depois)
}

// liberarRetencaoTx devolve a a_liberar os pagamentos retidos por esta denúncia (ou repassa a retenção
// para outra denúncia retentora ainda aberta no mesmo desafio) — spec §2.3 "efeito na retenção".
func (s *moderacaoService) liberarRetencaoTx(tx *gorm.DB, d *models.Denuncia) error {
	if d.IDDesafio == nil || !models.CategoriaDeDesafio(d.Categoria) {
		return nil
	}
	outra, err := s.denuncias.RetentoraAberta(tx, *d.IDDesafio, d.ID)
	if err != nil {
		return err
	}
	if outra != nil {
		n, err := s.pendentes.TrocarRetencao(tx, d.ID, *outra)
		if err != nil || n == 0 {
			return err
		}
		return s.registrar(tx, atorSistema, EventoPagamentoRetido, models.EntidadeDesafio, *d.IDDesafio, d.IDDesafio, "",
			map[string]any{"id_denuncia": d.ID}, map[string]any{"id_denuncia": *outra, "pagamentos": n})
	}
	n, err := s.pendentes.Reabrir(tx, d.ID)
	if err != nil || n == 0 {
		return err
	}
	return s.registrar(tx, atorSistema, EventoPagamentoReaberto, models.EntidadeDesafio, *d.IDDesafio, d.IDDesafio, "",
		map[string]any{"status": models.PendenteRetido, "id_denuncia": d.ID}, map[string]any{"status": models.PendenteALiberar, "pagamentos": n})
}

// avisarDecisao notifica o denunciante (sem detalhes da punição de terceiro).
func (s *moderacaoService) avisarDecisao(pc *posCommit, d *models.Denuncia, status string) {
	idDen, id := d.IDDenunciante, d.ID
	pc.add(func() {
		s.notif.Notificar(idDen, models.NotifDenunciaDecidida, "Sua denúncia foi analisada",
			fmt.Sprintf("A denúncia #%d foi concluída como %s.", id, rotuloStatusDenuncia(status)), "/minhas-denuncias")
	})
}

func rotuloStatusDenuncia(st string) string {
	switch st {
	case models.DenunciaProcedente:
		return "procedente"
	case models.DenunciaImprocedente:
		return "improcedente"
	case models.DenunciaArquivada:
		return "arquivada"
	}
	return st
}

func rotuloPunicao(tipo string) string {
	switch tipo {
	case models.PunicaoAdvertencia:
		return "advertência"
	case models.PunicaoSuspensao:
		return "suspensão"
	case models.PunicaoBanimento:
		return "banimento"
	case models.PunicaoBloqueioAcademia:
		return "bloqueio na academia"
	case models.PunicaoAnulacaoResultado:
		return "anulação de resultado"
	}
	return tipo
}

// avisarPunicao notifica o punido (tipo, fim e justificativa). O banido não lê notificações in-app,
// mas o registro fica (e o motivo aparece no 403 do login).
func (s *moderacaoService) avisarPunicao(pc *posCommit, p *models.Punicao) {
	id, tipo, fim, just := p.IDUsuario, p.Tipo, p.Fim, p.Justificativa
	pc.add(func() {
		msg := fmt.Sprintf("Você recebeu uma %s. Motivo: %s", rotuloPunicao(tipo), just)
		if fim != nil {
			msg = fmt.Sprintf("Você recebeu uma %s até %s. Motivo: %s", rotuloPunicao(tipo), dataBR(*fim), just)
		}
		s.notif.Notificar(id, models.NotifPunicaoAplicada, "Punição aplicada", limitar(msg, 500), "/minhas-punicoes")
	})
}

// ─── Academia ────────────────────────────────────────────────────────────────

// academiaDoUsuario devolve a academia da conta logada (403 se não houver).
func (s *moderacaoService) academiaDoUsuario(userID uint) (*models.Academia, error) {
	a, err := s.academias.BuscarPorUsuario(userID)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, erroProibido("Conta sem academia cadastrada")
	}
	return a, nil
}

// denunciaDaAcademia carrega a denúncia só se estiver na jurisdição da academia (404 caso contrário:
// encaminhada/escalonada sai da vista dela).
func (s *moderacaoService) denunciaDaAcademia(tx *gorm.DB, idAcademia, id uint, travar bool) (*models.Denuncia, error) {
	d, err := s.denuncias.Buscar(tx, id, travar)
	if err != nil {
		return nil, err
	}
	if d == nil || d.Jurisdicao != models.JurisdicaoAcademia || d.IDAcademiaJulgadora == nil || *d.IDAcademiaJulgadora != idAcademia {
		return nil, erroNaoEncontrado("Denúncia não encontrada (ou não está mais com a sua academia)")
	}
	return d, nil
}

func (s *moderacaoService) FilaAcademia(userID uint, status string, pagina, limite int) (*models.PaginaAdmin[models.DenunciaResponse], error) {
	a, err := s.academiaDoUsuario(userID)
	if err != nil {
		return nil, err
	}
	f := repositories.FiltroDenuncias{IDAcademia: &a.ID, Jurisdicao: models.JurisdicaoAcademia, Pagina: pagina, Limite: limite}
	if status != "" {
		f.Status = []string{status}
	}
	ds, total, err := s.denuncias.Listar(f)
	if err != nil {
		return nil, err
	}
	itens, err := s.denunciasResponse(ds, paraAcademia, userID)
	if err != nil {
		return nil, err
	}
	return paginaDe(itens, total, pagina, limite), nil
}

func (s *moderacaoService) DenunciaAcademia(userID, id uint) (*models.DenunciaAcademiaDetalhe, error) {
	a, err := s.academiaDoUsuario(userID)
	if err != nil {
		return nil, err
	}
	d, err := s.denunciaDaAcademia(nil, a.ID, id, false)
	if err != nil {
		return nil, err
	}
	r, err := s.denunciaResponse(d, paraAcademia, userID)
	if err != nil {
		return nil, err
	}
	ps, _, err := s.punicoes.Listar(repositories.FiltroPunicoes{IDUsuario: &d.IDDenunciado, IDAcademia: &a.ID, Pagina: 1, Limite: 100})
	if err != nil {
		return nil, err
	}
	hist, err := s.punicoesResponse(ps, false)
	if err != nil {
		return nil, err
	}
	return &models.DenunciaAcademiaDetalhe{Denuncia: *r, HistoricoNaAcademia: hist}, nil
}

func (s *moderacaoService) AssumirAcademia(userID, id uint) (*models.DenunciaResponse, error) {
	a, err := s.academiaDoUsuario(userID)
	if err != nil {
		return nil, err
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		d, err := s.denunciaDaAcademia(tx, a.ID, id, true)
		if err != nil {
			return err
		}
		ok, err := s.denuncias.AssumirComoAcademia(tx, id, a.ID, userID)
		if err != nil {
			return err
		}
		if !ok {
			return erroConflito("A denúncia não está aberta (já foi assumida ou decidida)")
		}
		return s.registrar(tx, atorAcademia(userID), EventoDenunciaAssumida, models.EntidadeDenuncia, id, d.IDDesafio, "",
			map[string]any{"status": d.Status}, map[string]any{"status": models.DenunciaEmAnalise})
	})
	if err != nil {
		return nil, err
	}
	return s.respostaAcademia(a.ID, userID, id)
}

func (s *moderacaoService) respostaAcademia(idAcademia, userID, id uint) (*models.DenunciaResponse, error) {
	d, err := s.denuncias.Buscar(nil, id, false)
	if err != nil || d == nil {
		return nil, err
	}
	return s.denunciaResponse(d, paraAcademia, userID)
}

func (s *moderacaoService) EncaminharAcademia(userID, id uint, justificativa string) (*models.DenunciaResponse, error) {
	justificativa, err := textoEntre(justificativa, decisaoTextoMin, decisaoTextoMax, "A justificativa")
	if err != nil {
		return nil, err
	}
	a, err := s.academiaDoUsuario(userID)
	if err != nil {
		return nil, err
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		d, err := s.denunciaDaAcademia(tx, a.ID, id, true)
		if err != nil {
			return err
		}
		ok, err := s.denuncias.Encaminhar(tx, id, &a.ID, s.now())
		if err != nil {
			return err
		}
		if !ok {
			return erroConflito("A denúncia já foi decidida")
		}
		return s.registrar(tx, atorAcademia(userID), EventoDenunciaEncaminhada, models.EntidadeDenuncia, id, d.IDDesafio, justificativa,
			map[string]any{"jurisdicao": d.Jurisdicao, "status": d.Status},
			map[string]any{"jurisdicao": models.JurisdicaoAdmin, "status": models.DenunciaAberta})
	})
	if err != nil {
		return nil, err
	}
	d, err := s.denuncias.Buscar(nil, id, false)
	if err != nil || d == nil {
		return nil, err
	}
	// A academia continua vendo a resposta desta ação (sem denunciante), mas a denúncia sai da fila dela.
	return s.denunciaResponse(d, paraAcademia, userID)
}

// validarPunicaoAcademia: a academia só aplica advertência ou bloqueio na própria academia.
func validarPunicaoAcademia(p *models.PunicaoAcademiaRequest, agora time.Time) error {
	switch p.Tipo {
	case models.PunicaoAdvertencia:
		if p.Fim != nil {
			return erroRequisicao("Advertência não tem data de fim")
		}
	case models.PunicaoBloqueioAcademia:
		if p.Fim != nil && !p.Fim.After(agora) {
			return erroRequisicao("O fim do bloqueio deve estar no futuro")
		}
	case models.PunicaoSuspensao, models.PunicaoBanimento, models.PunicaoAnulacaoResultado:
		return erroProibido("A academia só aplica advertência ou bloqueio na academia; para suspensão, banimento ou anulação, encaminhe ao admin")
	default:
		return erroRequisicao("Tipo de punição inválido")
	}
	return nil
}

func (s *moderacaoService) DecidirAcademia(userID, id uint, req *models.DecidirDenunciaAcademiaRequest) (*models.DenunciaResponse, error) {
	if !statusDecisaoValido(req.Status) {
		return nil, erroRequisicao("Status inválido (use procedente, improcedente ou arquivada)")
	}
	texto, err := textoEntre(req.DecisaoTexto, decisaoTextoMin, decisaoTextoMax, "O texto da decisão")
	if err != nil {
		return nil, err
	}
	agora := s.now()
	if req.Punicao != nil {
		if req.Status != models.DenunciaProcedente {
			return nil, erroRequisicao("Só denúncia procedente gera punição")
		}
		if err := validarPunicaoAcademia(req.Punicao, agora); err != nil {
			return nil, err
		}
	}
	a, err := s.academiaDoUsuario(userID)
	if err != nil {
		return nil, err
	}
	pc := &posCommit{}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		d, err := s.denunciaDaAcademia(tx, a.ID, id, true)
		if err != nil {
			return err
		}
		ok, err := s.denuncias.Decidir(tx, id, req.Status, texto, agora, models.JurisdicaoAcademia, &a.ID)
		if err != nil {
			return err
		}
		if !ok {
			return erroConflito("A denúncia já foi decidida")
		}
		depois := map[string]any{"status": req.Status}
		if req.Punicao != nil {
			idAcad, aplicador := a.ID, userID
			p := &models.Punicao{IDDenuncia: &d.ID, IDUsuario: d.IDDenunciado, Tipo: req.Punicao.Tipo, Escopo: models.EscopoAcademia,
				IDAcademia: &idAcad, IDDesafio: d.IDDesafio, Inicio: agora, Fim: req.Punicao.Fim, AplicadaPor: &aplicador, Justificativa: texto}
			if err := s.aplicarPunicaoTx(tx, atorAcademia(userID), p, agora); err != nil {
				return err
			}
			depois["punicoes"] = []uint{p.ID}
			s.avisarPunicao(pc, p)
		}
		if err := s.registrar(tx, atorAcademia(userID), EventoDenunciaDecidida, models.EntidadeDenuncia, id, d.IDDesafio, texto,
			map[string]any{"status": d.Status}, depois); err != nil {
			return err
		}
		if err := s.liberarRetencaoTx(tx, d); err != nil {
			return err
		}
		s.avisarDecisao(pc, d, req.Status)
		return nil
	})
	if err != nil {
		return nil, err
	}
	pc.disparar()
	return s.respostaAcademia(a.ID, userID, id)
}

func (s *moderacaoService) PunicoesAcademia(userID uint, tipo string, vigentes bool, pagina, limite int) (*models.PaginaAdmin[models.PunicaoResponse], error) {
	a, err := s.academiaDoUsuario(userID)
	if err != nil {
		return nil, err
	}
	switch tipo {
	case "", models.PunicaoAdvertencia, models.PunicaoBloqueioAcademia:
	default:
		return nil, erroRequisicao("Tipo inválido (advertencia ou bloqueio_academia)")
	}
	ps, total, err := s.punicoes.Listar(repositories.FiltroPunicoes{IDAcademia: &a.ID, Tipo: tipo, Vigentes: vigentes, Agora: s.now(),
		Pagina: pagina, Limite: limite})
	if err != nil {
		return nil, err
	}
	itens, err := s.punicoesResponse(ps, false)
	if err != nil {
		return nil, err
	}
	return paginaDe(itens, total, pagina, limite), nil
}
