package services

import (
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"gymScore-backend/internal/client"
	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/pkg/utils"
)

// prazoSaqueOrfao: um saque 'pending' SEM transferência gravada há mais que isso é "órfão" (o processo
// caiu entre o débito e o Asaas, ou o Asaas respondeu de forma ambígua). Órfão NUNCA é reembolsado
// automaticamente: a transferência pode existir no Asaas. Ele é marcado para revisão do admin, que
// confere no painel do Asaas pela externalReference "saque-<id>".
const prazoSaqueOrfao = 5 * time.Minute

// tentativasGravarTransferencia: retentativas ao gravar o ID da transferência já criada no Asaas.
const tentativasGravarTransferencia = 3

// ReferenciaSaque é a externalReference enviada ao Asaas para o saque local `id`.
func ReferenciaSaque(id uint) string { return fmt.Sprintf("saque-%d", id) }

// CarteiraService expõe saldo, extrato e saques PIX do usuário do token.
type CarteiraService interface {
	Saldo(userID uint) (*models.SaldoResponse, error)
	Extrato(userID uint, pagina, limite int) (*models.ExtratoResponse, error)
	Sacar(userID uint, req *models.SaqueRequest) (*models.SaqueResponse, error)
	// ConsultarSaque faz o polling no Asaas; reembolsa automaticamente se falhou/cancelou.
	ConsultarSaque(userID, id uint) (*models.SaqueResponse, error)
	// ReconciliarSaques (job) consulta no Asaas os saques pending antigos e aplica o status; órfãos
	// (sem transferência gravada) vão para revisão manual. Devolve quantos foram processados.
	ReconciliarSaques(limite int) (int, error)
	// Pendentes lista os créditos do pódio ainda fora do saldo (janela de contestação) e os já liberados.
	Pendentes(userID uint, pagina, limite int) (*models.CarteiraPendentesResponse, error)
	// Debitos lista os débitos do usuário (abertos primeiro); com débito aberto o saque é bloqueado.
	Debitos(userID uint) (*models.CarteiraDebitosResponse, error)
}

type carteiraService struct {
	carteira *repositories.CarteiraRepository
	usuarios repositories.UsuarioRepository
	asaas    AsaasGateway
	notif    Notificador
	agora    func() time.Time
	// tarifaSaque é debitada junto de cada saque (0 = sem tarifa; PLATAFORMA_TAXA_MODO=inscricao_saque).
	tarifaSaque int64
}

// NovoCarteiraService cria o serviço de carteira.
func NovoCarteiraService(
	carteira *repositories.CarteiraRepository,
	usuarios repositories.UsuarioRepository,
	asaas AsaasGateway,
	notif Notificador,
	tarifaSaqueCentavos int64,
) CarteiraService {
	if tarifaSaqueCentavos < 0 {
		tarifaSaqueCentavos = 0
	}
	return &carteiraService{carteira: carteira, usuarios: usuarios, asaas: asaas, notif: notif, agora: time.Now,
		tarifaSaque: tarifaSaqueCentavos}
}

func (s *carteiraService) Saldo(userID uint) (*models.SaldoResponse, error) {
	v, err := s.carteira.Saldo(userID)
	if err != nil {
		return nil, err
	}
	tot, err := repositories.NovoPagamentoPendenteRepository(s.carteira.DB()).TotaisDoUsuario(userID)
	if err != nil {
		return nil, err
	}
	deb, err := repositories.DebitoAbertoDoUsuario(s.carteira.DB(), userID)
	if err != nil {
		return nil, err
	}
	return &models.SaldoResponse{SaldoCentavos: v, Saldo: utils.CentavosParaReais(v), TarifaSaqueCentavos: s.tarifaSaque,
		ALiberarCentavos: tot.ALiberar, RetidoCentavos: tot.Retido, DebitoAbertoCentavos: deb, SaqueBloqueado: deb > 0}, nil
}

func (s *carteiraService) Pendentes(userID uint, pagina, limite int) (*models.CarteiraPendentesResponse, error) {
	pagina, limite = normalizarPagina(pagina, limite)
	repo := repositories.NovoPagamentoPendenteRepository(s.carteira.DB())
	tot, err := repo.TotaisDoUsuario(userID)
	if err != nil {
		return nil, err
	}
	ps, total, err := repo.DoUsuario(userID, pagina, limite)
	if err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(ps))
	for _, p := range ps {
		ids = append(ids, p.IDDesafio)
	}
	titulos, err := TitulosDesafios(s.carteira.DB(), ids)
	if err != nil {
		return nil, err
	}
	resp := &models.CarteiraPendentesResponse{ALiberarCentavos: tot.ALiberar, RetidoCentavos: tot.Retido,
		Itens: make([]models.PagamentoPendenteResponse, 0, len(ps)), Total: total, Pagina: pagina, Limite: limite}
	for _, p := range ps {
		resp.Itens = append(resp.Itens, PendenteParaResponse(p, titulos[p.IDDesafio], nil, false))
	}
	return resp, nil
}

func (s *carteiraService) Debitos(userID uint) (*models.CarteiraDebitosResponse, error) {
	ds, err := repositories.NovoDebitoRepository(s.carteira.DB()).DoUsuario(userID)
	if err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(ds))
	for _, d := range ds {
		ids = append(ids, d.IDDesafio)
	}
	titulos, err := TitulosDesafios(s.carteira.DB(), ids)
	if err != nil {
		return nil, err
	}
	resp := &models.CarteiraDebitosResponse{Itens: make([]models.DebitoResponse, 0, len(ds))}
	for _, d := range ds {
		if d.QuitadoEm == nil {
			resp.AbertoCentavos += d.SaldoRestanteCentavos
		}
		resp.Itens = append(resp.Itens, debitoResponse(d, titulos[d.IDDesafio], nil))
	}
	resp.SaqueBloqueado = resp.AbertoCentavos > 0
	return resp, nil
}

// CodigoDebitoAberto é o `codigo` do 409 do saque recusado por débito aberto.
const CodigoDebitoAberto = "debito_aberto"

// ErroSaqueComDebito é o 409 do saque com débito aberto (data.debito_aberto_centavos = total devido).
func ErroSaqueComDebito(total int64) *ErroNegocio {
	e := erroConflito(fmt.Sprintf("Saque bloqueado: você tem um débito aberto de %s (anulação de resultado). "+
		"Os próximos créditos na carteira o abatem.", reaisBR(total)))
	e.Codigo = CodigoDebitoAberto
	e.Dados = map[string]any{"debito_aberto_centavos": total}
	return e
}

func (s *carteiraService) Extrato(userID uint, pagina, limite int) (*models.ExtratoResponse, error) {
	pagina, limite = normalizarPagina(pagina, limite)
	itens, total, err := s.carteira.Extrato(userID, pagina, limite)
	if err != nil {
		return nil, err
	}
	resp := &models.ExtratoResponse{Itens: make([]models.TransacaoResponse, 0, len(itens)), Total: total, Pagina: pagina, Limite: limite}
	for i := range itens {
		resp.Itens = append(resp.Itens, *paraResponse(&itens[i]))
	}
	return resp, nil
}

// saqueResponse monta o DTO do saque, com a tarifa efetivamente cobrada (0 se devolvida).
func (s *carteiraService) saqueResponse(t *models.Transacao) (*models.SaqueResponse, error) {
	r := paraSaqueResponse(t)
	tarifa, err := s.carteira.TarifaDoSaque(t.ID, t.IDUsuario)
	if err != nil {
		return nil, err
	}
	r.TarifaCentavos = tarifa
	return r, nil
}

func paraSaqueResponse(t *models.Transacao) *models.SaqueResponse {
	valor := -t.ValorCentavos
	r := &models.SaqueResponse{ID: t.ID, ValorCentavos: valor, Valor: utils.CentavosParaReais(valor),
		Status: t.Status, AsaasTransferID: t.AsaasTransferID, CriadoEm: t.CreatedAt, EmRevisao: t.EmRevisao}
	if t.Descricao != nil {
		r.Descricao = *t.Descricao
	}
	return r
}

// Sacar debita ANTES de chamar o Asaas (o dinheiro nunca fica disponível em dobro). Com tarifa de
// saque, o débito é valor + tarifa (o Asaas recebe só o valor pedido).
//
// Resultado da chamada ao Asaas (proteção contra pagamento em dobro):
//   - sucesso: grava o ID da transferência (com retentativas; se não gravar, o saque vai para revisão);
//   - erro DEFINITIVO (4xx de validação: a transferência certamente não existe): falha o saque e
//     devolve valor + tarifa;
//   - erro AMBÍGUO (timeout, rede, 5xx, resposta ilegível): a transferência pode ter sido criada. O
//     saque fica pending, debitado, e é marcado para revisão do admin. Nada é devolvido.
func (s *carteiraService) Sacar(userID uint, req *models.SaqueRequest) (*models.SaqueResponse, error) {
	chave := strings.TrimSpace(req.ChavePix)
	if len(chave) < 3 {
		return nil, erroRequisicao("Chave PIX inválida")
	}
	t, err := s.carteira.IniciarSaqueComTarifa(userID, req.ValorCentavos, s.tarifaSaque, "Saque PIX ("+req.TipoChave+")")
	if err != nil {
		var deb *repositories.ErroDebitoAberto
		if errors.As(err, &deb) {
			return nil, ErroSaqueComDebito(deb.TotalCentavos)
		}
		if errors.Is(err, repositories.ErrSaldoInsuficiente) {
			if s.tarifaSaque > 0 {
				return nil, erro(ErroSaldoInsuficiente.Status, fmt.Sprintf("Saldo insuficiente: o saque de %s mais a tarifa de %s exige %s",
					reaisBR(req.ValorCentavos), reaisBR(s.tarifaSaque), reaisBR(req.ValorCentavos+s.tarifaSaque)))
			}
			return nil, ErroSaldoInsuficiente
		}
		return nil, err
	}

	tr, err := s.asaas.CriarTransferenciaPix(req.ValorCentavos, chave, req.TipoChave,
		fmt.Sprintf("Saque GymScore #%d", t.ID), ReferenciaSaque(t.ID))
	if err != nil {
		if client.ErroDefinitivo(err) {
			log.Printf("[SAQUE] Asaas recusou a transferência do saque %d: %v", t.ID, err)
			s.falhar(t.ID, userID, req.ValorCentavos, "A transferência foi recusada pelo banco/Asaas")
			return nil, erro(502, "Não foi possível processar o saque agora. O valor foi devolvido ao seu saldo")
		}
		log.Printf("[SAQUE] resposta AMBÍGUA do Asaas no saque %d (%s), mantido pending para revisão: %v", t.ID, ReferenciaSaque(t.ID), err)
		s.marcarRevisao(t.ID, "Resposta ambígua do Asaas ao criar a transferência ("+ReferenciaSaque(t.ID)+"): "+err.Error())
		atual, errB := s.carteira.BuscarSaque(t.ID, userID)
		if errB != nil {
			return nil, errB
		}
		return s.saqueResponse(atual)
	}
	if err := s.gravarTransferencia(t.ID, tr.ID); err != nil {
		// A transferência existe no Asaas mas o ID não foi gravado: NÃO reembolsar (pagaria duas vezes).
		log.Printf("[SAQUE] ERRO: saque %d criado no Asaas (%s) mas o ID não foi gravado: %v", t.ID, tr.ID, err)
		s.marcarRevisao(t.ID, fmt.Sprintf("Transferência %s criada no Asaas, mas o ID não foi gravado", tr.ID))
	}
	t.AsaasTransferID = &tr.ID
	return s.aplicarStatus(userID, t, tr.Status)
}

// gravarTransferencia grava o ID da transferência com algumas retentativas (deadlock/conexão).
func (s *carteiraService) gravarTransferencia(id uint, transferID string) error {
	var err error
	for i := 0; i < tentativasGravarTransferencia; i++ {
		if err = s.carteira.MarcarTransferencia(id, transferID); err == nil {
			return nil
		}
		time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
	}
	return err
}

// marcarRevisao marca o saque para revisão manual (só loga se não conseguir: o saque continua
// pending e a reconciliação tenta de novo).
func (s *carteiraService) marcarRevisao(id uint, motivo string) {
	if _, err := s.carteira.MarcarSaqueEmRevisao(id, motivo); err != nil {
		log.Printf("[SAQUE] ERRO ao marcar saque %d para revisão: %v", id, err)
	}
}

// falhar reembolsa (idempotente) e notifica uma única vez.
func (s *carteiraService) falhar(id, userID uint, valor int64, motivo string) {
	reembolsou, err := s.carteira.FalharSaque(id)
	if err != nil {
		log.Printf("[SAQUE] ERRO ao reembolsar saque %d: %v", id, err)
		return
	}
	if reembolsou {
		s.notif.Notificar(userID, models.NotifSaqueFalhou, "Saque não concluído",
			fmt.Sprintf("Seu saque de %s falhou e o valor (e a tarifa, se houve) voltou ao saldo. %s", reaisBR(valor), motivo), "/carteira")
	}
}

// aplicarStatus traduz o status do Asaas: DONE conclui; FAILED/CANCELLED reembolsa.
func (s *carteiraService) aplicarStatus(userID uint, t *models.Transacao, statusAsaas string) (*models.SaqueResponse, error) {
	valor := -t.ValorCentavos
	switch strings.ToUpper(statusAsaas) {
	case "DONE":
		concluiu, err := s.carteira.ConcluirSaque(t.ID)
		if err != nil {
			return nil, err
		}
		if concluiu {
			s.notif.Notificar(userID, models.NotifSaqueConcluido, "Saque concluído",
				fmt.Sprintf("Seu saque PIX de %s foi concluído.", reaisBR(valor)), "/carteira")
		}
	case "FAILED", "CANCELLED":
		s.falhar(t.ID, userID, valor, "Status do Asaas: "+strings.ToLower(statusAsaas)+".")
	}
	atual, err := s.carteira.BuscarSaque(t.ID, userID)
	if err != nil {
		return nil, err
	}
	return s.saqueResponse(atual)
}

func (s *carteiraService) ConsultarSaque(userID, id uint) (*models.SaqueResponse, error) {
	t, err := s.carteira.BuscarSaque(id, userID)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, erroNaoEncontrado("Saque não encontrado")
	}
	if err := s.reconciliarSaque(t); err != nil {
		return nil, err
	}
	if t, err = s.carteira.BuscarSaque(id, userID); err != nil {
		return nil, err
	}
	return s.saqueResponse(t)
}

// reconciliarSaque aplica ao saque pending o que se sabe dele no Asaas. Sem transferência gravada,
// após o prazo, o saque vai para revisão (nunca é reembolsado sem confirmação de que nada existe).
func (s *carteiraService) reconciliarSaque(t *models.Transacao) error {
	if t.Status != models.TransacaoPending {
		return nil
	}
	if t.AsaasTransferID == nil {
		if !t.EmRevisao && s.agora().Sub(t.CreatedAt) > prazoSaqueOrfao {
			s.marcarRevisao(t.ID, "Saque sem transferência registrada após o prazo: conferir no Asaas a externalReference "+ReferenciaSaque(t.ID))
		}
		return nil
	}
	tr, err := s.asaas.BuscarTransferencia(*t.AsaasTransferID)
	if err != nil {
		// Mantém pending; tenta de novo no próximo ciclo
		log.Printf("[SAQUE] falha ao consultar transferência %s: %v", *t.AsaasTransferID, err)
		return nil
	}
	_, err = s.aplicarStatus(t.IDUsuario, t, tr.Status)
	return err
}

// ReconciliarSaques é o passo do job de reconciliação para saques.
func (s *carteiraService) ReconciliarSaques(limite int) (int, error) {
	ts, err := s.carteira.ListarSaquesParaReconciliar(s.agora().Add(-prazoSaqueOrfao), limite)
	if err != nil {
		return 0, err
	}
	for i := range ts {
		if err := s.reconciliarSaque(&ts[i]); err != nil {
			log.Printf("[RECONCILIACAO] saque %d: %v", ts[i].ID, err)
		}
	}
	return len(ts), nil
}
