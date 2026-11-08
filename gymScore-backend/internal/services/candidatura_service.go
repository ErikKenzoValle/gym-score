package services

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/pkg/utils"

	"gorm.io/gorm"
)

// Vaga de instrutor ("leilão" com teto): a academia abre a vaga informando o teto; qualquer instrutor
// (vinculado ou autônomo) se candidata com um lance <= teto; a academia escolhe um. O escolhido vira
// o instrutor designado DAQUELE desafio (sem criar vínculo com a academia): o lance passa a ser a
// parte fixa dele e a diferença teto - lance volta para a academia (estorno_reserva). Todas as
// mudanças de estado rodam com o desafio travado (FOR UPDATE), o que serializa candidatar, retirar,
// escolher, editar e cancelar.

func paraCandidaturaResponse(c *models.CandidaturaInstrutor, idAcademia *uint) models.CandidaturaResponse {
	r := models.CandidaturaResponse{
		ID: c.ID, IDDesafio: c.IDDesafio, LanceCentavos: c.LanceCentavos, Lance: utils.CentavosParaReais(c.LanceCentavos),
		Status: c.Status, CriadoEm: c.CriadoEm, AtualizadoEm: c.AtualizadoEm,
	}
	if c.Mensagem != nil {
		r.Mensagem = *c.Mensagem
	}
	if idAcademia != nil && c.Instrutor != nil { // visão da academia dona
		nome := ""
		if c.Instrutor.Usuario != nil {
			nome = strings.TrimSpace(c.Instrutor.Usuario.Nome + " " + c.Instrutor.Usuario.Sobrenome)
		}
		r.Instrutor = &models.CandidatoResumo{
			ID: c.Instrutor.ID, Nome: nome, CREF: c.Instrutor.CREF, Especialidade: c.Instrutor.Especialidade,
			Vinculado: c.Instrutor.IDAcademia != nil && *c.Instrutor.IDAcademia == *idAcademia,
		}
	}
	return r
}

// enriquecerVagas preenche, em lote, os dados de vaga que dependem de quem consulta: as candidaturas
// pendentes (academia dona) e a candidatura do próprio instrutor.
func (s *desafioService) enriquecerVagas(ctx *contextoUsuario, ds []*models.Desafio) error {
	var idsPendentes, idsVaga []uint
	for _, d := range ds {
		if d.InstrutorSelecao != models.SelecaoVagaAberta {
			continue
		}
		idsVaga = append(idsVaga, d.ID)
		if d.IDInstrutor == nil && ctx.dono(d) {
			idsPendentes = append(idsPendentes, d.ID)
		}
	}
	if len(idsPendentes) > 0 {
		cont, err := s.candidaturas.ContarPendentes(idsPendentes)
		if err != nil {
			return err
		}
		for _, d := range ds {
			d.QtdCandidaturas = cont[d.ID]
		}
	}
	if ctx.InstrutorID != 0 && len(idsVaga) > 0 {
		minhas, err := s.candidaturas.DoInstrutor(ctx.InstrutorID, idsVaga)
		if err != nil {
			return err
		}
		for _, d := range ds {
			if c, ok := minhas[d.ID]; ok {
				c := c
				d.MinhaCandidatura = &c
			}
		}
	}
	return nil
}

// Candidatar (instrutor) dá um lance <= teto na vaga aberta. Repetir atualiza o lance e a mensagem
// enquanto estiver pendente; depois de retirar, volta a pendente. O segundo retorno é true quando a
// candidatura foi criada (ou reaberta). Notifica a academia na criação/reabertura.
func (s *desafioService) Candidatar(userID, id uint, req *models.CandidatarRequest) (*models.CandidaturaResponse, bool, error) {
	ctx, err := s.contextoAtivo(userID)
	if err != nil {
		return nil, false, err
	}
	if ctx.InstrutorID == 0 {
		return nil, false, erroProibido("Apenas instrutores podem se candidatar a uma vaga")
	}
	if !ctx.InstrutorAprovado {
		return nil, false, ErroCadastroNaoAprovado
	}
	mensagem := strings.TrimSpace(req.Mensagem)
	if utf8.RuneCountInString(mensagem) > models.MensagemCandidaturaMax {
		return nil, false, erroRequisicao(fmt.Sprintf("A mensagem deve ter no máximo %d caracteres", models.MensagemCandidaturaMax))
	}
	if req.LanceCentavos < 0 {
		return nil, false, erroRequisicao("O lance não pode ser negativo")
	}
	var msg *string
	if mensagem != "" {
		msg = &mensagem
	}
	var criada bool
	err = s.comDesafioTravado(id, func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if d.InstrutorSelecao != models.SelecaoVagaAberta {
			return erroConflito("Este desafio não tem vaga de instrutor aberta")
		}
		if d.Status != models.StatusAberto {
			return erroConflito("O desafio não está aberto para candidaturas")
		}
		if d.IDInstrutor != nil {
			return erroConflito("A vaga de instrutor já foi preenchida")
		}
		if d.DataEvento != nil && !d.DataEvento.After(s.agora()) {
			return erroConflito("O evento já começou: as candidaturas estão encerradas")
		}
		if err := exigirSemBloqueioAcademia(repo, userID, d, s.agora()); err != nil {
			return err
		}
		teto := int64(0)
		if d.InstrutorTetoCentavos != nil {
			teto = *d.InstrutorTetoCentavos
		}
		if req.LanceCentavos > teto {
			return erroRequisicao(fmt.Sprintf("O lance (%s) não pode passar do teto de %s", reaisBR(req.LanceCentavos), reaisBR(teto)))
		}
		cr := s.candidaturas.ComTx(tx)
		atual, err := cr.BuscarDoInstrutor(d.ID, ctx.InstrutorID)
		if err != nil {
			return err
		}
		switch {
		case atual == nil:
			criada = true
			if err := cr.Criar(&models.CandidaturaInstrutor{
				IDDesafio: d.ID, IDInstrutor: ctx.InstrutorID, LanceCentavos: req.LanceCentavos,
				Mensagem: msg, Status: models.CandidaturaPendente,
			}); err != nil {
				if repositories.EhDuplicata(err) {
					return erroConflito("Você já tem uma candidatura neste desafio")
				}
				return err
			}
		case atual.Status == models.CandidaturaPendente:
			if err := cr.Reabrir(atual.ID, req.LanceCentavos, msg); err != nil {
				return err
			}
		case atual.Status == models.CandidaturaRetirada:
			criada = true
			if err := cr.Reabrir(atual.ID, req.LanceCentavos, msg); err != nil {
				return err
			}
		default:
			return erroConflito("Sua candidatura neste desafio já foi decidida")
		}
		if criada {
			academia, titulo, idDesafio, lance := d.IDCriador, d.Titulo, d.ID, req.LanceCentavos
			nome := "Um instrutor"
			if u, err := s.usuarios.BuscarPorID(userID); err == nil && u != nil {
				nome = strings.TrimSpace(u.Nome + " " + u.Sobrenome)
			}
			ev.add(func() {
				s.notif.Notificar(academia, models.NotifCandidaturaRecebida, "Nova candidatura de instrutor",
					fmt.Sprintf("%s se candidatou à vaga de instrutor de \"%s\" com lance de %s.", nome, titulo, reaisBR(lance)), linkDesafio(idDesafio))
			})
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	c, err := s.candidaturas.BuscarDoInstrutor(id, ctx.InstrutorID)
	if err != nil || c == nil {
		return nil, false, err
	}
	r := paraCandidaturaResponse(c, nil)
	return &r, criada, nil
}

// RetirarCandidatura (instrutor) retira a própria candidatura pendente. Candidatura já escolhida ou
// recusada não pode ser retirada.
func (s *desafioService) RetirarCandidatura(userID, id uint) (*models.CandidaturaResponse, error) {
	ctx, err := s.contexto(userID)
	if err != nil {
		return nil, err
	}
	if ctx.InstrutorID == 0 {
		return nil, erroProibido("Apenas instrutores têm candidaturas")
	}
	err = s.comDesafioTravado(id, func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		cr := s.candidaturas.ComTx(tx)
		c, err := cr.BuscarDoInstrutor(d.ID, ctx.InstrutorID)
		if err != nil {
			return err
		}
		if c == nil {
			return erroNaoEncontrado("Você não tem candidatura neste desafio")
		}
		switch c.Status {
		case models.CandidaturaPendente:
		case models.CandidaturaEscolhida:
			return erroConflito("Sua candidatura foi escolhida e não pode mais ser retirada")
		case models.CandidaturaRetirada:
			return erroConflito("Sua candidatura já foi retirada")
		default:
			return erroConflito("Sua candidatura já foi decidida")
		}
		ok, err := cr.MudarStatus(c.ID, models.CandidaturaPendente, models.CandidaturaRetirada)
		if err != nil {
			return err
		}
		if !ok {
			return erroConflito("A candidatura já mudou de estado")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	c, err := s.candidaturas.BuscarDoInstrutor(id, ctx.InstrutorID)
	if err != nil || c == nil {
		return nil, err
	}
	r := paraCandidaturaResponse(c, nil)
	return &r, nil
}

// ListarCandidaturas (só a academia dona) lista os candidatos, do menor para o maior lance. As
// retiradas ficam de fora.
func (s *desafioService) ListarCandidaturas(userID, id uint) ([]models.CandidaturaResponse, error) {
	ctx, err := s.contexto(userID)
	if err != nil {
		return nil, err
	}
	d, err := s.desafios.BuscarDetalhe(id)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, erroNaoEncontrado("Desafio não encontrado")
	}
	if err := exigirDono(ctx, d); err != nil {
		return nil, err
	}
	cs, err := s.candidaturas.ListarDoDesafio(id)
	if err != nil {
		return nil, err
	}
	idAcademia := d.IDAcademia
	if idAcademia == nil {
		idAcademia = new(uint)
	}
	out := make([]models.CandidaturaResponse, 0, len(cs))
	for i := range cs {
		out = append(out, paraCandidaturaResponse(&cs[i], idAcademia))
	}
	return out, nil
}

// EscolherCandidatura (só a academia dona, desafio aberto, vaga sem escolhido) designa o candidato:
// o lance vira o valor fixo do instrutor, a reserva passa a ser prêmio fixo + lance e a diferença
// teto - lance volta para a academia (estorno_reserva). As demais candidaturas pendentes são
// recusadas e todos os candidatos são notificados. Duas escolhas simultâneas: uma vence, a outra
// recebe 409.
func (s *desafioService) EscolherCandidatura(userID, id, idCandidatura uint) (*models.DesafioResponse, error) {
	return s.acao(userID, id, func(ctx *contextoUsuario, tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if err := exigirDono(ctx, d); err != nil {
			return err
		}
		if d.InstrutorSelecao != models.SelecaoVagaAberta {
			return erroConflito("Este desafio não tem vaga de instrutor aberta")
		}
		if d.Status != models.StatusAberto {
			return erroConflito("Só é possível escolher o instrutor em desafios abertos")
		}
		if d.IDInstrutor != nil {
			return erroConflito("A vaga de instrutor já foi preenchida")
		}
		cr := s.candidaturas.ComTx(tx)
		c, err := cr.Buscar(idCandidatura)
		if err != nil {
			return err
		}
		if c == nil || c.IDDesafio != d.ID {
			return erroNaoEncontrado("Candidatura não encontrada neste desafio")
		}
		if c.Status != models.CandidaturaPendente {
			return erroConflito("Só é possível escolher uma candidatura pendente")
		}
		if d.InstrutorTetoCentavos == nil || c.LanceCentavos > *d.InstrutorTetoCentavos {
			return erroConflito("O lance desta candidatura passa do teto da vaga")
		}
		if c.Instrutor == nil || c.Instrutor.Usuario == nil {
			return fmt.Errorf("candidatura %d sem instrutor carregado", c.ID)
		}

		reserva := ReservaNecessaria(d.PremioFixoCentavos, c.LanceCentavos, nil)
		if err := repo.Atualizar(d.ID, map[string]any{
			"id_instrutor": c.IDInstrutor, "instrutor_fixo_centavos": c.LanceCentavos, "reserva_academia_centavos": reserva,
		}); err != nil {
			return err
		}
		switch diff := d.ReservaAcademiaCentavos - reserva; {
		case diff > 0:
			if _, err := s.carteira.Creditar(tx, repositories.Lancamento{
				IDUsuario: d.IDCriador, Tipo: models.TipoEstornoReserva, ValorCentavos: diff,
				IDDesafio: &d.ID, Descricao: "Reserva devolvida (teto - lance do instrutor): " + d.Titulo,
			}); err != nil {
				return err
			}
		case diff < 0: // reserva anterior abaixo do necessário (dados inconsistentes): completa
			if _, err := s.carteira.Debitar(tx, repositories.Lancamento{
				IDUsuario: d.IDCriador, Tipo: models.TipoReservaDesafio, ValorCentavos: -diff,
				IDDesafio: &d.ID, Descricao: "Reserva adicional (instrutor escolhido): " + d.Titulo,
			}); err != nil {
				return err
			}
		}
		if ok, err := cr.MudarStatus(c.ID, models.CandidaturaPendente, models.CandidaturaEscolhida); err != nil {
			return err
		} else if !ok {
			return erroConflito("A candidatura já mudou de estado")
		}
		recusados, err := cr.RecusarPendentes(d.ID, c.ID)
		if err != nil {
			return err
		}
		escolhido, titulo, idDesafio, lance := c.Instrutor.IDUsuario, d.Titulo, d.ID, c.LanceCentavos
		ev.add(func() {
			s.notif.Notificar(escolhido, models.NotifCandidaturaEscolhida, "Você foi escolhido como instrutor",
				fmt.Sprintf("Sua candidatura ao desafio \"%s\" foi escolhida (lance de %s).", titulo, reaisBR(lance)), linkDesafio(idDesafio))
			s.notif.NotificarVarios(recusados, models.NotifCandidaturaRecusada, "Candidatura não escolhida",
				fmt.Sprintf("A academia escolheu outro instrutor para o desafio \"%s\".", titulo), linkDesafio(idDesafio))
		})
		return nil
	})
}

// ListarVagas (instrutor) lista as vagas de instrutor abertas: desafios abertos, com data futura,
// vaga aberta e sem escolhido, do evento mais próximo ao mais distante. Cada item traz `vaga`
// (teto, modo de pagamento, % e a candidatura do próprio instrutor).
func (s *desafioService) ListarVagas(userID uint, f FiltroListagem) (*models.ListaDesafiosResponse, error) {
	ctx, err := s.contexto(userID)
	if err != nil {
		return nil, err
	}
	if ctx.InstrutorID == 0 {
		return nil, erroProibido("Apenas instrutores veem as vagas abertas")
	}
	pagina, limite := normalizarPagina(f.Pagina, f.Limite)
	desafios, total, err := s.desafios.Listar(repositories.FiltroDesafios{
		IDUsuario: userID, Status: models.StatusAberto, Todos: true, VagaAberta: true, Futuros: true, Agora: s.agora(),
		Categoria: f.Categoria, Busca: f.Busca, IDAcademia: f.IDAcademia, Ordem: repositories.OrdemEventoProx,
		Pagina: pagina, Limite: limite,
	})
	if err != nil {
		return nil, err
	}
	ptrs := make([]*models.Desafio, len(desafios))
	for i := range desafios {
		ptrs[i] = &desafios[i]
	}
	if err := s.enriquecerVagas(ctx, ptrs); err != nil {
		return nil, err
	}
	itens := make([]models.DesafioResponse, 0, len(desafios))
	for i := range desafios {
		itens = append(itens, *s.montarResposta(&desafios[i], ctx, false))
	}
	return &models.ListaDesafiosResponse{Itens: itens, Total: total, Pagina: pagina, Limite: limite}, nil
}
