package services

import (
	"fmt"
	"strings"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// ─── Saques ──────────────────────────────────────────────────────────────────

// saqueParaAcao carrega o saque (404 se não existir ou não for saque).
func (s *adminService) saqueParaAcao(id uint) (*models.Transacao, error) {
	t, err := s.consultas.BuscarTransacao(id)
	if err != nil {
		return nil, err
	}
	if t == nil || t.Tipo != models.TipoSaque {
		return nil, erroNaoEncontrado("Saque não encontrado")
	}
	return t, nil
}

// respostaSaque relê o saque e monta a resposta da ação.
func (s *adminService) respostaSaque(id uint, alterado bool, statusAsaas string) (*models.SaqueAcaoResponse, error) {
	t, err := s.consultas.BuscarTransacao(id)
	if err != nil {
		return nil, err
	}
	itens, err := s.saquesAdmin([]models.Transacao{*t})
	if err != nil {
		return nil, err
	}
	return &models.SaqueAcaoResponse{Alterado: alterado, StatusAsaas: statusAsaas, Saque: itens[0]}, nil
}

// statusTransferencia consulta o Asaas quando o saque tem transferência gravada ("" se não tem).
func (s *adminService) statusTransferencia(t *models.Transacao) (string, error) {
	if t.AsaasTransferID == nil || *t.AsaasTransferID == "" {
		return "", nil
	}
	tr, err := s.asaas.BuscarTransferencia(*t.AsaasTransferID)
	if err != nil || tr == nil {
		return "", erroAsaas
	}
	return strings.ToUpper(tr.Status), nil
}

func (s *adminService) avisarReembolso(t *models.Transacao) {
	s.notif.Notificar(t.IDUsuario, models.NotifSaqueFalhou, "Saque não concluído",
		fmt.Sprintf("Seu saque de %s não foi concluído e o valor (e a tarifa, se houve) voltou ao saldo.", reaisBR(-t.ValorCentavos)), "/carteira")
}

func (s *adminService) avisarConclusao(t *models.Transacao) {
	s.notif.Notificar(t.IDUsuario, models.NotifSaqueConcluido, "Saque concluído",
		fmt.Sprintf("Seu saque PIX de %s foi concluído.", reaisBR(-t.ValorCentavos)), "/carteira")
}

// ReconciliarSaque consulta a transferência no Asaas e aplica o status (DONE conclui; FAILED/CANCELLED
// reembolsa valor + tarifa), pelos mesmos métodos idempotentes do polling. Saque sem transferência
// gravada não tem o que consultar (409): conferir no Asaas pela referência e reembolsar/marcar pago.
func (s *adminService) ReconciliarSaque(ator AtorAdmin, id uint, motivo string) (*models.SaqueAcaoResponse, error) {
	motivo, err := motivoValido(motivo)
	if err != nil {
		return nil, err
	}
	t, err := s.saqueParaAcao(id)
	if err != nil {
		return nil, err
	}
	if t.Status != models.TransacaoPending {
		return nil, erroConflito("Só saques pendentes podem ser reconciliados (status atual: " + t.Status + ")")
	}
	if t.AsaasTransferID == nil || *t.AsaasTransferID == "" {
		return nil, erroConflito("Saque sem transferência registrada no Asaas: confira pela referência " +
			ReferenciaSaque(t.ID) + " e use reembolsar ou marcar como pago")
	}
	status, err := s.statusTransferencia(t)
	if err != nil {
		return nil, err
	}
	resultado, alterado := "sem_mudanca", false
	err = s.db.Transaction(func(tx *gorm.DB) error {
		switch status {
		case "DONE":
			if alterado, err = s.carteira.ConcluirSaqueTx(tx, id); err != nil {
				return err
			}
			if alterado {
				resultado = "concluido"
			}
		case "FAILED", "CANCELLED":
			if alterado, err = s.carteira.FalharSaqueTx(tx, id); err != nil {
				return err
			}
			if alterado {
				resultado = "reembolsado"
			}
		}
		return s.auditar(tx, ator, AcaoSaqueReconciliar, AlvoTransacao, id, motivo,
			map[string]any{"status_asaas": status, "resultado": resultado, "transferencia": *t.AsaasTransferID})
	})
	if err != nil {
		return nil, err
	}
	switch resultado {
	case "concluido":
		s.avisarConclusao(t)
	case "reembolsado":
		s.avisarReembolso(t)
	}
	return s.respostaSaque(id, alterado, status)
}

// ReembolsarSaque (superadmin) devolve valor + tarifa de um saque pendente (inclusive em revisão)
// reaproveitando FalharSaqueTx (UPDATE condicional: chamar de novo não reembolsa duas vezes). Se a
// transferência existe no Asaas e já foi paga (DONE), recusa (409).
func (s *adminService) ReembolsarSaque(ator AtorAdmin, id uint, motivo string) (*models.SaqueAcaoResponse, error) {
	if !ator.superadmin() {
		return nil, ErroSuperadmin
	}
	motivo, err := motivoValido(motivo)
	if err != nil {
		return nil, err
	}
	t, err := s.saqueParaAcao(id)
	if err != nil {
		return nil, err
	}
	switch t.Status {
	case models.TransacaoFailed:
		return s.respostaSaque(id, false, "") // já reembolsado: idempotente
	case models.TransacaoPending:
	default:
		return nil, erroConflito("Saque já concluído: não pode ser reembolsado")
	}
	status, err := s.statusTransferencia(t)
	if err != nil {
		return nil, err
	}
	if status == "DONE" {
		return nil, erroConflito("O Asaas informa a transferência como concluída (DONE): use marcar como pago")
	}
	var reembolsou bool
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if reembolsou, err = s.carteira.FalharSaqueTx(tx, id); err != nil || !reembolsou {
			return err // outro processo resolveu antes: nada mudou, nada a auditar
		}
		return s.auditar(tx, ator, AcaoSaqueReembolsar, AlvoTransacao, id, motivo,
			map[string]any{"valor_centavos": -t.ValorCentavos, "status_asaas": status, "em_revisao": t.EmRevisao})
	})
	if err != nil {
		return nil, err
	}
	if reembolsou {
		s.avisarReembolso(t)
	}
	return s.respostaSaque(id, reembolsou, status)
}

// MarcarSaquePago (superadmin) conclui um saque pendente confirmado manualmente no Asaas (a tarifa
// continua cobrada), via ConcluirSaqueTx (idempotente). Recusa se o Asaas diz que falhou/cancelou.
func (s *adminService) MarcarSaquePago(ator AtorAdmin, id uint, motivo string) (*models.SaqueAcaoResponse, error) {
	if !ator.superadmin() {
		return nil, ErroSuperadmin
	}
	motivo, err := motivoValido(motivo)
	if err != nil {
		return nil, err
	}
	t, err := s.saqueParaAcao(id)
	if err != nil {
		return nil, err
	}
	switch t.Status {
	case models.TransacaoConcluido:
		return s.respostaSaque(id, false, "")
	case models.TransacaoPending:
	default:
		return nil, erroConflito("Saque já reembolsado/falhou: não pode ser marcado como pago")
	}
	status, err := s.statusTransferencia(t)
	if err != nil {
		return nil, err
	}
	if status == "FAILED" || status == "CANCELLED" {
		return nil, erroConflito("O Asaas informa a transferência como " + strings.ToLower(status) + ": use reembolsar")
	}
	var concluiu bool
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if concluiu, err = s.carteira.ConcluirSaqueTx(tx, id); err != nil || !concluiu {
			return err
		}
		return s.auditar(tx, ator, AcaoSaqueMarcarPago, AlvoTransacao, id, motivo,
			map[string]any{"valor_centavos": -t.ValorCentavos, "status_asaas": status, "em_revisao": t.EmRevisao})
	})
	if err != nil {
		return nil, err
	}
	if concluiu {
		s.avisarConclusao(t)
	}
	return s.respostaSaque(id, concluiu, status)
}

// ─── Usuários ────────────────────────────────────────────────────────────────
// Bloquear/desbloquear do painel viraram punição (banimento) — ver ModeracaoService.BloquearUsuario.

// ─── Desafios ────────────────────────────────────────────────────────────────

// CancelarDesafio cancela pelo painel com a MESMA lógica do cancelamento da academia (estorno
// integral das entradas, devolução da reserva, candidaturas recusadas, notificações), sem exigir dono.
func (s *adminService) CancelarDesafio(ator AtorAdmin, id uint, motivo string) (*models.DesafioDetalheAdminResponse, error) {
	motivo, err := motivoValido(motivo)
	if err != nil {
		return nil, err
	}
	_, err = s.desafioSvc.CancelarPeloAdmin(id, func(tx *gorm.DB, d *models.Desafio) error {
		return s.auditar(tx, ator, AcaoDesafioCancelar, AlvoDesafio, id, motivo, map[string]any{
			"status_anterior": d.Status, "titulo": d.Titulo, "valor_entrada_centavos": d.ValorEntradaCentavos,
			"reserva_academia_centavos": d.ReservaAcademiaCentavos,
		})
	})
	if err != nil {
		return nil, err
	}
	return s.Desafio(id)
}

// ─── Aprovação de academias / instrutores ────────────────────────────────────

// decidir aplica a decisão (só de 'pendente'), grava a auditoria e avisa a conta.
func (s *adminService) decidir(ator AtorAdmin, tipo string, id uint, aprovar bool, motivo string) (*models.CadastroAdminResponse, error) {
	motivo, err := motivoValido(motivo)
	if err != nil {
		return nil, err
	}
	status := models.CadastroRejeitado
	if aprovar {
		status = models.CadastroAprovado
	}
	var (
		idUsuario uint
		atual     string
	)
	switch tipo {
	case AlvoAcademia:
		a, err := s.consultas.BuscarAcademia(id)
		if err != nil {
			return nil, err
		}
		if a == nil {
			return nil, erroNaoEncontrado("Academia não encontrada")
		}
		idUsuario, atual = a.IDUsuario, a.Status
	default:
		i, err := s.consultas.BuscarInstrutor(id)
		if err != nil {
			return nil, err
		}
		if i == nil {
			return nil, erroNaoEncontrado("Instrutor não encontrado")
		}
		idUsuario, atual = i.IDUsuario, i.Status
	}
	if atual != models.CadastroPendente {
		return nil, erroConflito("O cadastro não está pendente (status atual: " + atual + ")")
	}
	acao := map[string]map[bool]string{
		AlvoAcademia:  {true: AcaoAcademiaAprovar, false: AcaoAcademiaRejeitar},
		AlvoInstrutor: {true: AcaoInstrutorAprovar, false: AcaoInstrutorRejeitar},
	}[tipo][aprovar]
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var mudou bool
		var err error
		if tipo == AlvoAcademia {
			mudou, err = s.moderacao.DecidirAcademia(tx, id, status, &motivo, ator.ID, s.agora())
		} else {
			mudou, err = s.moderacao.DecidirInstrutor(tx, id, status, &motivo, ator.ID, s.agora())
		}
		if err != nil {
			return err
		}
		if !mudou {
			return erroConflito("O cadastro já foi decidido por outro administrador")
		}
		return s.auditar(tx, ator, acao, tipo, id, motivo, map[string]any{"status": status, "id_usuario": idUsuario})
	})
	if err != nil {
		return nil, err
	}
	nome := "academia"
	if tipo == AlvoInstrutor {
		nome = "instrutor"
	}
	if aprovar {
		s.notif.Notificar(idUsuario, models.NotifCadastroAprovado, "Cadastro aprovado",
			fmt.Sprintf("Seu cadastro de %s foi aprovado pela equipe GymScore. Você já pode operar.", nome), "/menu")
	} else {
		s.notif.Notificar(idUsuario, models.NotifCadastroRejeitado, "Cadastro não aprovado",
			fmt.Sprintf("Seu cadastro de %s não foi aprovado. Motivo: %s", nome, motivo), "/menu")
	}
	var c models.CadastroAdminResponse
	if tipo == AlvoAcademia {
		a, err := s.consultas.BuscarAcademia(id)
		if err != nil || a == nil {
			return nil, err
		}
		c = models.NovoCadastroAcademia(a)
	} else {
		i, err := s.consultas.BuscarInstrutor(id)
		if err != nil || i == nil {
			return nil, err
		}
		c = models.NovoCadastroInstrutor(i)
	}
	return &c, nil
}

func (s *adminService) DecidirAcademia(ator AtorAdmin, id uint, aprovar bool, motivo string) (*models.CadastroAdminResponse, error) {
	return s.decidir(ator, AlvoAcademia, id, aprovar, motivo)
}

func (s *adminService) DecidirInstrutor(ator AtorAdmin, id uint, aprovar bool, motivo string) (*models.CadastroAdminResponse, error) {
	return s.decidir(ator, AlvoInstrutor, id, aprovar, motivo)
}

// ─── Administradores (superadmin) ────────────────────────────────────────────

func hashSenhaTemporaria() (string, string, error) {
	senha, err := GerarSenhaTemporaria()
	if err != nil {
		return "", "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(senha), bcrypt.DefaultCost)
	return senha, string(hash), err
}

// CriarAdmin cria um admin com senha temporária (devolvida uma única vez).
func (s *adminService) CriarAdmin(ator AtorAdmin, req *models.CriarAdminRequest) (*models.AdminSenhaTemporariaResponse, error) {
	if !ator.superadmin() {
		return nil, ErroSuperadmin
	}
	motivo, err := motivoValido(req.Motivo)
	if err != nil {
		return nil, err
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if existente, err := s.admins.BuscarPorEmail(email); err != nil {
		return nil, err
	} else if existente != nil {
		return nil, erroConflito("Já existe um administrador com este e-mail")
	}
	senha, hash, err := hashSenhaTemporaria()
	if err != nil {
		return nil, err
	}
	criadoPor := ator.ID
	a := &models.Administrador{Nome: strings.TrimSpace(req.Nome), Email: email, SenhaHash: hash, Papel: req.Papel,
		Ativo: true, CriadoPor: &criadoPor}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.admins.Criar(tx, a); err != nil {
			return err
		}
		return s.auditar(tx, ator, AcaoAdminCriar, AlvoAdmin, a.ID, motivo, map[string]any{"email": email, "papel": req.Papel})
	})
	if err != nil {
		if repositories.EhDuplicata(err) {
			return nil, erroConflito("Já existe um administrador com este e-mail")
		}
		return nil, err
	}
	return &models.AdminSenhaTemporariaResponse{Admin: models.NovoAdminResponse(a), SenhaTemporaria: senha}, nil
}

// protegerUltimoSuperadmin trava as linhas de superadmins ativos (FOR UPDATE, sempre ANTES da linha
// do alvo, na mesma ordem em toda operação) e recusa tirar o último superadmin ativo.
func (s *adminService) protegerUltimoSuperadmin(tx *gorm.DB, alvo *models.Administrador) error {
	n, err := s.admins.ContarSuperadminsAtivos(tx)
	if err != nil {
		return err
	}
	if alvo.Papel == models.PapelSuperadmin && alvo.Ativo && n <= 1 {
		return erroConflito("Não é possível remover ou rebaixar o último superadmin ativo")
	}
	return nil
}

// travarAlvo trava os superadmins e depois a linha do admin alvo (404 se não existir).
func (s *adminService) travarAlvo(tx *gorm.DB, id uint) (*models.Administrador, error) {
	if _, err := s.admins.ContarSuperadminsAtivos(tx); err != nil {
		return nil, err
	}
	alvo, err := s.admins.TravarPorID(tx, id)
	if err != nil {
		return nil, err
	}
	if alvo == nil {
		return nil, erroNaoEncontrado("Administrador não encontrado")
	}
	return alvo, nil
}

// AlterarAtivoAdmin desativa/reativa (incrementa token_versao: a sessão do alvo cai na hora).
// Não desativa a si mesmo nem o último superadmin ativo. false = já estava assim.
func (s *adminService) AlterarAtivoAdmin(ator AtorAdmin, id uint, ativo bool, motivo string) (bool, error) {
	if !ator.superadmin() {
		return false, ErroSuperadmin
	}
	motivo, err := motivoValido(motivo)
	if err != nil {
		return false, err
	}
	if !ativo && id == ator.ID {
		return false, erroConflito("Você não pode desativar a própria conta")
	}
	var mudou bool
	err = s.db.Transaction(func(tx *gorm.DB) error {
		alvo, err := s.travarAlvo(tx, id)
		if err != nil {
			return err
		}
		if alvo.Ativo == ativo {
			return nil
		}
		if !ativo {
			if err := s.protegerUltimoSuperadmin(tx, alvo); err != nil {
				return err
			}
		}
		if mudou, err = s.admins.AlterarAtivo(tx, id, ativo); err != nil || !mudou {
			return err
		}
		acao := AcaoAdminReativar
		if !ativo {
			acao = AcaoAdminDesativar
		}
		return s.auditar(tx, ator, acao, AlvoAdmin, id, motivo, map[string]any{"email": alvo.Email, "papel": alvo.Papel})
	})
	return mudou, err
}

// TrocarPapelAdmin muda o papel (incrementa token_versao). Não rebaixa o último superadmin ativo.
func (s *adminService) TrocarPapelAdmin(ator AtorAdmin, id uint, papel, motivo string) (bool, error) {
	if !ator.superadmin() {
		return false, ErroSuperadmin
	}
	if papel != models.PapelSuperadmin && papel != models.PapelAdmin {
		return false, erroRequisicao("Papel inválido (use superadmin ou admin)")
	}
	motivo, err := motivoValido(motivo)
	if err != nil {
		return false, err
	}
	var mudou bool
	err = s.db.Transaction(func(tx *gorm.DB) error {
		alvo, err := s.travarAlvo(tx, id)
		if err != nil {
			return err
		}
		if alvo.Papel == papel {
			return nil
		}
		if papel == models.PapelAdmin {
			if err := s.protegerUltimoSuperadmin(tx, alvo); err != nil {
				return err
			}
		}
		if mudou, err = s.admins.AlterarPapel(tx, id, papel); err != nil || !mudou {
			return err
		}
		return s.auditar(tx, ator, AcaoAdminTrocarPapel, AlvoAdmin, id, motivo,
			map[string]any{"email": alvo.Email, "de": alvo.Papel, "para": papel})
	})
	return mudou, err
}

// ResetarSenhaAdmin gera uma senha temporária nova (revoga os tokens do alvo).
func (s *adminService) ResetarSenhaAdmin(ator AtorAdmin, id uint, motivo string) (*models.AdminSenhaTemporariaResponse, error) {
	if !ator.superadmin() {
		return nil, ErroSuperadmin
	}
	motivo, err := motivoValido(motivo)
	if err != nil {
		return nil, err
	}
	senha, hash, err := hashSenhaTemporaria()
	if err != nil {
		return nil, err
	}
	var alvo *models.Administrador
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if alvo, err = s.admins.TravarPorID(tx, id); err != nil {
			return err
		}
		if alvo == nil {
			return erroNaoEncontrado("Administrador não encontrado")
		}
		if _, err := s.admins.AlterarSenha(tx, id, hash); err != nil {
			return err
		}
		return s.auditar(tx, ator, AcaoAdminResetarSenha, AlvoAdmin, id, motivo, map[string]any{"email": alvo.Email})
	})
	if err != nil {
		return nil, err
	}
	return &models.AdminSenhaTemporariaResponse{Admin: models.NovoAdminResponse(alvo), SenhaTemporaria: senha}, nil
}
