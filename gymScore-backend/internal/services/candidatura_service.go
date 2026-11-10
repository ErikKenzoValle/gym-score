package services

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"

	"gorm.io/gorm"
)

// Equipe de instrutores do desafio (rodada 5, fim do "leilão"). A academia define o tamanho da equipe
// (instrutores_vagas) e as condições POR instrutor (fixo e/ou % das inscrições). Ela pode INDICAR
// instrutores vinculados e aprovados (entram direto) e as vagas restantes ficam abertas: qualquer
// instrutor aprovado (vinculado ou autônomo) se CANDIDATA aceitando as condições (mensagem opcional, sem
// lance) e a academia aceita ou recusa. Ao preencher a última vaga as candidaturas pendentes são
// recusadas. A academia escolhe 1 RESPONSÁVEL (padrão: o primeiro indicado ou, sem indicados, o primeiro
// aceito), o único instrutor que inicia e lança o pódio; todos os membros marcam presença.
//
// Todas as mudanças rodam com o desafio travado (FOR UPDATE), o que serializa candidatar, retirar,
// aceitar, recusar, indicar, remover, trocar responsável, editar, iniciar e cancelar — é essa trava que
// garante o tamanho máximo da equipe e a "última vaga" (duas aceitações simultâneas: uma vence, a outra
// recebe 409). desafios.id_instrutor é o espelho legado do responsável, gravado na mesma transação.

func paraCandidaturaResponse(c *models.CandidaturaInstrutor, idAcademia *uint) models.CandidaturaResponse {
	r := models.CandidaturaResponse{
		ID: c.ID, IDDesafio: c.IDDesafio, Status: c.Status, CriadoEm: c.CriadoEm, AtualizadoEm: c.AtualizadoEm,
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

// enriquecerVagas preenche, em lote, os dados de equipe que dependem de quem consulta: as candidaturas
// pendentes (academia dona) e a candidatura do próprio instrutor.
func (s *desafioService) enriquecerVagas(ctx *contextoUsuario, ds []*models.Desafio) error {
	var idsPendentes, ids []uint
	for _, d := range ds {
		ids = append(ids, d.ID)
		if ctx.dono(d) {
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
	if ctx.InstrutorID != 0 && len(ids) > 0 {
		minhas, err := s.candidaturas.DoInstrutor(ctx.InstrutorID, ids)
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

// nomeUsuario devolve "Nome Sobrenome" do usuário (ou o padrão).
func (s *desafioService) nomeUsuario(id uint, padrao string) string {
	if u, err := s.usuarios.BuscarPorID(id); err == nil && u != nil {
		if n := strings.TrimSpace(u.Nome + " " + u.Sobrenome); n != "" {
			return n
		}
	}
	return padrao
}

// exigirVagaParaCandidatura: desafio aberto, evento futuro e vaga livre na equipe (desafio travado).
func (s *desafioService) exigirVagaParaCandidatura(d *models.Desafio) error {
	if d.Status != models.StatusAberto {
		return erroConflito("O desafio não está aberto para candidaturas")
	}
	if d.DataEvento != nil && !d.DataEvento.After(s.agora()) {
		return erroConflito("O evento já começou: as candidaturas estão encerradas")
	}
	if d.VagasInstrutorLivres() == 0 {
		return erroConflito("A equipe de instrutores deste desafio já está completa")
	}
	return nil
}

// Candidatar (instrutor) se candidata a uma vaga da equipe aceitando as condições da academia (sem
// lance). Repetir atualiza a mensagem enquanto estiver pendente; depois de retirar ou ser recusado, volta
// a pendente. O segundo retorno é true quando a candidatura foi criada (ou reaberta). Notifica a academia
// na criação/reabertura.
func (s *desafioService) Candidatar(userID, id uint, req *models.CandidatarRequest) (*models.CandidaturaResponse, bool, error) {
	ctx, err := s.contextoAtivo(userID)
	if err != nil {
		return nil, false, err
	}
	if err := exigirInstrutor(ctx, "Apenas instrutores podem se candidatar a uma vaga"); err != nil {
		return nil, false, err
	}
	mensagem := strings.TrimSpace(req.Mensagem)
	if utf8.RuneCountInString(mensagem) > models.MensagemCandidaturaMax {
		return nil, false, erroRequisicao(fmt.Sprintf("A mensagem deve ter no máximo %d caracteres", models.MensagemCandidaturaMax))
	}
	var msg *string
	if mensagem != "" {
		msg = &mensagem
	}
	var criada bool
	err = s.comDesafioTravado(id, func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if d.Membro(ctx.InstrutorID) != nil {
			return erroConflito("Você já faz parte da equipe de instrutores deste desafio")
		}
		cr := s.candidaturas.ComTx(tx)
		atual, err := cr.BuscarDoInstrutor(d.ID, ctx.InstrutorID)
		if err != nil {
			return err
		}
		// Atualizar a mensagem de uma candidatura pendente não depende de vaga livre.
		if atual == nil || atual.Status != models.CandidaturaPendente {
			if err := s.exigirVagaParaCandidatura(d); err != nil {
				return err
			}
		} else if d.Status != models.StatusAberto {
			return erroConflito("O desafio não está aberto para candidaturas")
		}
		if err := exigirSemBloqueioAcademia(repo, userID, d, s.agora()); err != nil {
			return err
		}
		// Conflito de interesse (rodada 4): inscrito como atleta não se candidata (desafio travado).
		if err := exigirInstrutorNaoInscrito(repo, userID, d,
			"Você está inscrito como atleta neste desafio e não pode se candidatar à equipe de instrutores"); err != nil {
			return err
		}
		switch {
		case atual == nil:
			criada = true
			if err := cr.Criar(&models.CandidaturaInstrutor{
				IDDesafio: d.ID, IDInstrutor: ctx.InstrutorID, Mensagem: msg, Status: models.CandidaturaPendente,
			}); err != nil {
				if repositories.EhDuplicata(err) {
					return erroConflito("Você já tem uma candidatura neste desafio")
				}
				return err
			}
		case atual.Status == models.CandidaturaPendente:
			if err := cr.Reabrir(atual.ID, msg); err != nil {
				return err
			}
		case atual.Status == models.CandidaturaRetirada || atual.Status == models.CandidaturaRecusada ||
			atual.Status == models.CandidaturaRemovida:
			criada = true
			if err := cr.Reabrir(atual.ID, msg); err != nil {
				return err
			}
		default:
			return erroConflito("Sua candidatura neste desafio já foi aceita")
		}
		if criada {
			academia, titulo, idDesafio := d.IDCriador, d.Titulo, d.ID
			nome := s.nomeUsuario(userID, "Um instrutor")
			ev.add(func() {
				s.notif.Notificar(academia, models.NotifCandidaturaRecebida, "Nova candidatura de instrutor",
					fmt.Sprintf("%s se candidatou à equipe de instrutores de \"%s\".", nome, titulo), linkDesafio(idDesafio))
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

// RetirarCandidatura (instrutor) retira a própria candidatura pendente (livre). Candidatura aceita não é
// retirada por aqui (quem tira membro da equipe é a academia).
func (s *desafioService) RetirarCandidatura(userID, id uint) (*models.CandidaturaResponse, error) {
	ctx, err := s.contexto(userID)
	if err != nil {
		return nil, err
	}
	if err := exigirInstrutor(ctx, "Apenas instrutores têm candidaturas"); err != nil {
		return nil, err
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
			return erroConflito("Sua candidatura foi aceita e não pode mais ser retirada: fale com a academia")
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

// ListarCandidaturas (só a academia dona) lista os candidatos: pendentes primeiro, por ordem de chegada.
// As retiradas ficam de fora.
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

// exigirGestaoEquipe: academia dona e desafio aberto (a equipe congela ao iniciar).
func exigirGestaoEquipe(ctx *contextoUsuario, d *models.Desafio) error {
	if err := exigirDono(ctx, d); err != nil {
		return err
	}
	if d.Status != models.StatusAberto {
		return erroConflito("A equipe de instrutores só pode ser alterada com o desafio aberto (antes de iniciar)")
	}
	return nil
}

// AceitarCandidatura (academia dona, desafio aberto, vaga livre) põe o candidato na equipe (origem
// candidatura). Sem responsável, ele vira o responsável (T5). Ao preencher a última vaga, as demais
// pendentes são recusadas e avisadas. Não mexe em dinheiro: a reserva já cobre todas as vagas.
func (s *desafioService) AceitarCandidatura(userID, id, idCandidatura uint) (*models.DesafioResponse, error) {
	return s.acao(userID, id, func(ctx *contextoUsuario, tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if err := exigirGestaoEquipe(ctx, d); err != nil {
			return err
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
			return erroConflito("Só é possível aceitar uma candidatura pendente")
		}
		if d.VagasInstrutorLivres() == 0 {
			return erroConflito("A equipe de instrutores já está completa")
		}
		if c.Instrutor == nil || c.Instrutor.Usuario == nil {
			return fmt.Errorf("candidatura %d sem instrutor carregado", c.ID)
		}
		if c.Instrutor.Status != models.CadastroAprovado {
			return erroConflito("O cadastro deste instrutor não está aprovado")
		}
		// Conflito de interesse (rodada 4): candidato inscrito como atleta não entra (desafio travado).
		if err := exigirInstrutorNaoInscrito(repo, c.Instrutor.IDUsuario, d,
			"Este instrutor está inscrito como atleta neste desafio e não pode entrar na equipe"); err != nil {
			return err
		}
		responsavel := d.ResponsavelEquipe() == nil
		if err := repo.AdicionarMembro(&models.DesafioInstrutor{IDDesafio: d.ID, IDInstrutor: c.IDInstrutor,
			Origem: models.EquipeOrigemCandidatura}); err != nil {
			if repositories.EhDuplicata(err) {
				return erroConflito("Este instrutor já está na equipe")
			}
			return err
		}
		if responsavel {
			if _, err := repo.DefinirResponsavel(d.ID, c.IDInstrutor); err != nil {
				return err
			}
		}
		if ok, err := cr.MudarStatus(c.ID, models.CandidaturaPendente, models.CandidaturaEscolhida); err != nil {
			return err
		} else if !ok {
			return erroConflito("A candidatura já mudou de estado")
		}
		var recusados []uint
		if len(d.Equipe)+1 >= d.InstrutoresVagas {
			if recusados, err = cr.RecusarPendentes(d.ID, c.ID); err != nil {
				return err
			}
		}
		aceito, titulo, idDesafio := c.Instrutor.IDUsuario, d.Titulo, d.ID
		ev.add(func() {
			texto := fmt.Sprintf("Sua candidatura ao desafio \"%s\" foi aceita: você está na equipe de instrutores.", titulo)
			if responsavel {
				texto = fmt.Sprintf("Sua candidatura ao desafio \"%s\" foi aceita: você está na equipe e é o instrutor responsável (lança o pódio).", titulo)
			}
			s.notif.Notificar(aceito, models.NotifCandidaturaEscolhida, "Candidatura aceita", texto, linkDesafio(idDesafio))
			s.notif.NotificarVarios(recusados, models.NotifCandidaturaRecusada, "Candidatura não aceita",
				fmt.Sprintf("A equipe de instrutores do desafio \"%s\" foi completada e sua candidatura foi encerrada.", titulo), linkDesafio(idDesafio))
		})
		return nil
	})
}

// RecusarCandidatura (academia dona, desafio aberto) recusa uma candidatura pendente e avisa o instrutor.
func (s *desafioService) RecusarCandidatura(userID, id, idCandidatura uint) (*models.CandidaturaResponse, error) {
	ctx, err := s.contextoAtivo(userID)
	if err != nil {
		return nil, err
	}
	var resp models.CandidaturaResponse
	err = s.comDesafioTravado(id, func(tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if err := exigirGestaoEquipe(ctx, d); err != nil {
			return err
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
			return erroConflito("Só é possível recusar uma candidatura pendente")
		}
		if ok, err := cr.MudarStatus(c.ID, models.CandidaturaPendente, models.CandidaturaRecusada); err != nil {
			return err
		} else if !ok {
			return erroConflito("A candidatura já mudou de estado")
		}
		c.Status = models.CandidaturaRecusada
		idAcademia := d.IDAcademia
		if idAcademia == nil {
			idAcademia = new(uint)
		}
		resp = paraCandidaturaResponse(c, idAcademia)
		if c.Instrutor != nil {
			alvo, titulo, idDesafio := c.Instrutor.IDUsuario, d.Titulo, d.ID
			ev.add(func() {
				s.notif.Notificar(alvo, models.NotifCandidaturaRecusada, "Candidatura não aceita",
					fmt.Sprintf("A academia não aceitou sua candidatura à equipe de instrutores de \"%s\".", titulo), linkDesafio(idDesafio))
			})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// IndicarInstrutor (academia dona, desafio aberto, vaga livre) põe na equipe um instrutor VINCULADO e
// APROVADO (T3), como na criação. Sem responsável, ele vira o responsável. Se o indicado tinha
// candidatura pendente, ela vira aceita. Ao preencher a última vaga, as pendentes são recusadas.
func (s *desafioService) IndicarInstrutor(userID, id, idInstrutor uint) (*models.DesafioResponse, error) {
	return s.acao(userID, id, func(ctx *contextoUsuario, tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if err := exigirGestaoEquipe(ctx, d); err != nil {
			return err
		}
		if d.Membro(idInstrutor) != nil {
			return erroConflito("Este instrutor já está na equipe")
		}
		if d.VagasInstrutorLivres() == 0 {
			return erroConflito("A equipe de instrutores já está completa: aumente instrutores_vagas ou remova alguém")
		}
		i, err := s.instrutorVinculadoTravado(tx, idInstrutor, ctx.AcademiaID)
		if err != nil {
			return err
		}
		if err := exigirInstrutorNaoInscrito(repo, i.IDUsuario, d,
			"Este instrutor está inscrito como atleta neste desafio e não pode entrar na equipe"); err != nil {
			return err
		}
		responsavel := d.ResponsavelEquipe() == nil
		if err := repo.AdicionarMembro(&models.DesafioInstrutor{IDDesafio: d.ID, IDInstrutor: i.ID,
			Origem: models.EquipeOrigemIndicado}); err != nil {
			if repositories.EhDuplicata(err) {
				return erroConflito("Este instrutor já está na equipe")
			}
			return err
		}
		if responsavel {
			if _, err := repo.DefinirResponsavel(d.ID, i.ID); err != nil {
				return err
			}
		}
		cr := s.candidaturas.ComTx(tx)
		if c, err := cr.BuscarDoInstrutor(d.ID, i.ID); err != nil {
			return err
		} else if c != nil && c.Status == models.CandidaturaPendente {
			if _, err := cr.MudarStatus(c.ID, models.CandidaturaPendente, models.CandidaturaEscolhida); err != nil {
				return err
			}
		}
		var recusados []uint
		if len(d.Equipe)+1 >= d.InstrutoresVagas {
			if recusados, err = cr.RecusarPendentes(d.ID, 0); err != nil {
				return err
			}
		}
		alvo, titulo, idDesafio := i.IDUsuario, d.Titulo, d.ID
		ev.add(func() {
			texto := fmt.Sprintf("A academia indicou você para a equipe de instrutores do desafio \"%s\".", titulo)
			if responsavel {
				texto = fmt.Sprintf("A academia indicou você para a equipe de instrutores do desafio \"%s\" como responsável (lança o pódio).", titulo)
			}
			s.notif.Notificar(alvo, models.NotifInstrutorDesignado, "Você foi indicado para uma equipe", texto, linkDesafio(idDesafio))
			s.notif.NotificarVarios(recusados, models.NotifCandidaturaRecusada, "Candidatura não aceita",
				fmt.Sprintf("A equipe de instrutores do desafio \"%s\" foi completada e sua candidatura foi encerrada.", titulo), linkDesafio(idDesafio))
		})
		return nil
	})
}

// RemoverDaEquipe (academia dona, desafio aberto = antes de iniciar) tira um membro da equipe e o avisa.
// Membro vindo de candidatura fica com a candidatura `removida`. Se era o responsável, o próximo pelo
// padrão (primeiro indicado; sem indicados, o primeiro aceito) assume e é avisado; equipe vazia fica sem
// responsável (espelho id_instrutor = NULL). A reserva não muda (cobre as vagas, preenchidas ou não).
func (s *desafioService) RemoverDaEquipe(userID, id, idInstrutor uint) (*models.DesafioResponse, error) {
	return s.acao(userID, id, func(ctx *contextoUsuario, tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if err := exigirGestaoEquipe(ctx, d); err != nil {
			return err
		}
		m := d.Membro(idInstrutor)
		if m == nil {
			return erroNaoEncontrado("Este instrutor não está na equipe do desafio")
		}
		ok, err := repo.RemoverMembro(d.ID, idInstrutor)
		if err != nil {
			return err
		}
		if !ok {
			return erroConflito("A equipe mudou. Atualize a página e tente de novo")
		}
		cr := s.candidaturas.ComTx(tx)
		if c, err := cr.BuscarDoInstrutor(d.ID, idInstrutor); err != nil {
			return err
		} else if c != nil && c.Status == models.CandidaturaEscolhida {
			if _, err := cr.MudarStatus(c.ID, models.CandidaturaEscolhida, models.CandidaturaRemovida); err != nil {
				return err
			}
		}
		var novoResp *models.DesafioInstrutor
		if m.Responsavel {
			restantes := make([]models.DesafioInstrutor, 0, len(d.Equipe))
			for _, x := range d.Equipe {
				if x.IDInstrutor != idInstrutor {
					restantes = append(restantes, x)
				}
			}
			novoResp = responsavelPadrao(restantes)
			if novoResp == nil {
				if err := repo.LimparResponsavel(d.ID); err != nil {
					return err
				}
			} else if _, err := repo.DefinirResponsavel(d.ID, novoResp.IDInstrutor); err != nil {
				return err
			}
		}
		titulo, idDesafio := d.Titulo, d.ID
		var removido, promovido uint
		if m.Instrutor != nil {
			removido = m.Instrutor.IDUsuario
		}
		if novoResp != nil && novoResp.Instrutor != nil {
			promovido = novoResp.Instrutor.IDUsuario
		}
		ev.add(func() {
			if removido != 0 {
				s.notif.Notificar(removido, models.NotifInstrutorRemovido, "Você saiu de uma equipe",
					fmt.Sprintf("A academia removeu você da equipe de instrutores do desafio \"%s\".", titulo), linkDesafio(idDesafio))
			}
			if promovido != 0 {
				s.notif.Notificar(promovido, models.NotifInstrutorDesignado, "Você é o instrutor responsável",
					fmt.Sprintf("Você passou a ser o instrutor responsável (lança o pódio) do desafio \"%s\".", titulo), linkDesafio(idDesafio))
			}
		})
		return nil
	})
}

// responsavelPadrao aplica a regra T5 a uma equipe: o primeiro indicado; sem indicados, o primeiro aceito
// (por ordem de entrada). nil com a equipe vazia.
func responsavelPadrao(equipe []models.DesafioInstrutor) *models.DesafioInstrutor {
	var melhor *models.DesafioInstrutor
	for i := range equipe {
		m := &equipe[i]
		if melhor == nil {
			melhor = m
			continue
		}
		mi, bi := m.Origem == models.EquipeOrigemIndicado, melhor.Origem == models.EquipeOrigemIndicado
		if mi && !bi || mi == bi && (m.CriadoEm.Before(melhor.CriadoEm) || m.CriadoEm.Equal(melhor.CriadoEm) && m.ID < melhor.ID) {
			melhor = m
		}
	}
	return melhor
}

// DefinirResponsavel (academia dona, desafio aberto ou em andamento) escolhe o responsável entre os
// membros da equipe. Avisa o novo e o anterior.
func (s *desafioService) DefinirResponsavel(userID, id, idInstrutor uint) (*models.DesafioResponse, error) {
	return s.acao(userID, id, func(ctx *contextoUsuario, tx *gorm.DB, repo *repositories.DesafioRepository, d *models.Desafio, ev *eventos) error {
		if err := exigirDono(ctx, d); err != nil {
			return err
		}
		if d.Status != models.StatusAberto && d.Status != models.StatusEmAndamento {
			return erroConflito("O responsável só pode ser trocado com o desafio aberto ou em andamento")
		}
		m := d.Membro(idInstrutor)
		if m == nil {
			return erroNaoEncontrado("Este instrutor não está na equipe do desafio")
		}
		if m.Responsavel {
			return nil // idempotente
		}
		anterior := d.ResponsavelEquipe()
		ok, err := repo.DefinirResponsavel(d.ID, idInstrutor)
		if err != nil {
			if repositories.EhDuplicata(err) {
				return erroConflito("A equipe mudou. Atualize a página e tente de novo")
			}
			return err
		}
		if !ok {
			return erroNaoEncontrado("Este instrutor não está na equipe do desafio")
		}
		titulo, idDesafio := d.Titulo, d.ID
		var novo, antigo uint
		if m.Instrutor != nil {
			novo = m.Instrutor.IDUsuario
		}
		if anterior != nil && anterior.Instrutor != nil {
			antigo = anterior.Instrutor.IDUsuario
		}
		ev.add(func() {
			if novo != 0 {
				s.notif.Notificar(novo, models.NotifInstrutorDesignado, "Você é o instrutor responsável",
					fmt.Sprintf("Você passou a ser o instrutor responsável (lança o pódio) do desafio \"%s\".", titulo), linkDesafio(idDesafio))
			}
			if antigo != 0 {
				s.notif.Notificar(antigo, models.NotifInstrutorRemovido, "Responsável trocado",
					fmt.Sprintf("A academia definiu outro instrutor como responsável do desafio \"%s\". Você continua na equipe.", titulo), linkDesafio(idDesafio))
			}
		})
		return nil
	})
}

// ListarVagas (instrutor) lista os "trabalhos oferecidos": desafios abertos, com data futura e vaga livre
// na equipe de instrutores, do evento mais próximo ao mais distante. Cada item traz `vaga` (condições
// por instrutor, vagas livres e a candidatura do próprio instrutor).
func (s *desafioService) ListarVagas(userID uint, f FiltroListagem) (*models.ListaDesafiosResponse, error) {
	ctx, err := s.contexto(userID)
	if err != nil {
		return nil, err
	}
	if err := exigirInstrutor(ctx, "Apenas instrutores veem as vagas abertas"); err != nil {
		return nil, err
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
