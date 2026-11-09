package repositories

import (
	"errors"
	"time"

	"gymScore-backend/internal/models"

	"gorm.io/gorm"
)

// Colunas de `usuarios` seguras para as listagens do admin (sem `senha`).
var colunasUsuarioAdmin = []string{
	"id", "nome", "sobrenome", "email", "cpf", "perfil", "username", "elo", "pontos", "saldo_centavos",
	"ultima_vez", "token_versao", "suspenso_ate", "banido_em", "motivo_bloqueio", "criado_em", "atualizado_em",
}

// selecionarUsuarioResumo limita os preloads de exibição (dono/criador/participante) a colunas sem senha/CPF.
func selecionarUsuarioResumo(db *gorm.DB) *gorm.DB {
	return db.Select("id", "nome", "sobrenome", "email", "username", "perfil", "banido_em")
}

// Periodo é um intervalo [De, Ate) opcional (nil = sem limite daquele lado).
type Periodo struct {
	De  *time.Time
	Ate *time.Time
}

func (p Periodo) aplicar(q *gorm.DB, coluna string) *gorm.DB {
	if p.De != nil {
		q = q.Where(coluna+" >= ?", *p.De)
	}
	if p.Ate != nil {
		q = q.Where(coluna+" < ?", *p.Ate)
	}
	return q
}

// FiltroTransacoesAdmin parametriza a listagem do ledger no painel. Zerados = sem filtro.
type FiltroTransacoesAdmin struct {
	Tipo      models.TipoTransacao // qualquer um dos 11 tipos do ledger
	Status    string
	IDUsuario uint
	IDDesafio uint
	EmRevisao *bool // saques marcados para revisão (true) / não marcados (false)
	Periodo         // created_at
	Pagina    int
	Limite    int
}

// FiltroUsuariosAdmin parametriza a busca de usuários no painel.
type FiltroUsuariosAdmin struct {
	// Q procura (substring) em nome, sobrenome, email e username; CPF por igualdade exata.
	Q         string
	Perfil    string
	Bloqueado *bool
	// Situacao filtra pelo estado da moderação: ativo (sem banimento nem suspensão vigente), suspenso
	// (suspensão vigente, sem banimento) ou banido. Agora é o instante da comparação (zero = NOW()).
	Situacao string
	Agora    time.Time
	Pagina   int
	Limite   int
}

// Situações do filtro de usuários do painel.
const (
	SituacaoAtivo    = "ativo"
	SituacaoSuspenso = "suspenso"
	SituacaoBanido   = "banido"
)

// FiltroDesafiosAdmin parametriza a listagem de desafios no painel.
type FiltroDesafiosAdmin struct {
	Status     models.StatusDesafio
	IDAcademia uint
	// Vencidos: `aberto` com data_evento < LimiteAberto OU `em_andamento` com data_evento <
	// LimiteAndamento (o chamador subtrai a carência). Limites zerados = agora.
	Vencidos        bool
	LimiteAberto    time.Time
	LimiteAndamento time.Time
	Pagina          int
	Limite          int
}

// FiltroCadastrosAdmin parametriza a listagem de academias/instrutores (aprovação).
type FiltroCadastrosAdmin struct {
	Status string // pendente | aprovada | rejeitada | vazio = todos
	Pagina int
	Limite int
}

// FiltroReceitasAdmin parametriza a listagem de plataforma_receitas.
type FiltroReceitasAdmin struct {
	Origem    string // taxa_inscricao | tarifa_saque | vazio = todas
	IDDesafio uint
	Periodo   // criado_em
	Pagina    int
	Limite    int
}

// FiltroCandidaturasAdmin parametriza a listagem de desafio_candidaturas_instrutor.
type FiltroCandidaturasAdmin struct {
	IDDesafio   uint
	IDInstrutor uint
	Status      string // pendente | escolhida | recusada | retirada | vazio = todas
	Pagina      int
	Limite      int
}

// FiltroReservasAdmin parametriza a listagem de reservas (garantia da academia) por desafio.
type FiltroReservasAdmin struct {
	Status     models.StatusDesafio // vazio = todos
	IDAcademia uint
	Pagina     int
	Limite     int
}

// ReservaDesafioResumo é a reserva de um desafio: o valor vigente (coluna do desafio) e o saldo
// líquido no ledger (débitos reserva_desafio - créditos estorno_reserva efetivos, em centavos positivos).
type ReservaDesafioResumo struct {
	IDDesafio       uint                 `gorm:"column:id_desafio"`
	Titulo          string               `gorm:"column:titulo"`
	Status          models.StatusDesafio `gorm:"column:status"`
	IDAcademia      *uint                `gorm:"column:id_academia"`
	IDCriador       uint                 `gorm:"column:id_criador"`
	ReservaCentavos int64                `gorm:"column:reserva_centavos"` // desafios.reserva_academia_centavos
	RetidoCentavos  int64                `gorm:"column:retido_centavos"`  // -SUM(reserva_desafio + estorno_reserva)
}

// ResumoFinanceiroAdmin são os totais da conferência do dashboard (centavos). A conferência é
// SomaSaldos + CustodiaEntradas + CustodiaReservas + Receitas + PendentesUsuarios + PendentesPlataforma
// - DebitosAbertos == Depositos - Saques + Ajustes (Ajustes = linhas de abertura legadas).
// Pendentes = créditos do pódio na janela de contestação (a_liberar/retido): o desafio já saiu de
// aberto/em_andamento mas o dinheiro ainda não está nos saldos. DebitosAbertos = o que a anulação não
// conseguiu estornar do saldo (o usuário já tinha gasto/sacado): saiu do sistema e é "a receber".
// SomaLedger deve ser igual a SomaSaldos.
type ResumoFinanceiroAdmin struct {
	SomaSaldos       int64 // SUM(usuarios.saldo_centavos)
	SomaLedger       int64 // ledger efetivo (mesma regra do CarteiraRepository.SomaLedger)
	Depositos        int64 // depósitos received
	Saques           int64 // saques pending + concluido (positivo)
	Ajustes          int64 // linhas `ajuste` efetivas
	CustodiaEntradas int64 // entradas líquidas em desafios aberto/em_andamento
	CustodiaReservas int64 // reservas líquidas em desafios aberto/em_andamento
	Receitas         int64 // SUM(plataforma_receitas) (líquida: estorno_taxa é negativo)
	// Moderação (00013): pendentes a_liberar+retido por beneficiário e débitos em aberto.
	PendentesUsuarios   int64 // Σ pagamentos_pendentes (id_usuario NOT NULL) a_liberar/retido
	PendentesPlataforma int64 // Σ pagamentos_pendentes taxa_inscricao a_liberar/retido
	DebitosAbertos      int64 // Σ usuario_debitos.saldo_restante_centavos (quitado_em NULL)
}

// AdminConsultaRepository reúne as consultas SOMENTE LEITURA do painel admin (paginadas: página >= 1,
// limite 1..100, padrão 20). As escritas ficam em AdminRepository/AuditoriaRepository/ModeracaoRepository
// e nos repositórios de cada entidade.
type AdminConsultaRepository interface {
	ListarTransacoes(f FiltroTransacoesAdmin) ([]models.Transacao, int64, error)
	// BuscarTransacao devolve a linha do ledger (nil se não existir).
	BuscarTransacao(id uint) (*models.Transacao, error)
	// ListarSaques é ListarTransacoes com tipo = saque (status vazio = todos).
	ListarSaques(status string, emRevisao *bool, pagina, limite int) ([]models.Transacao, int64, error)
	// ListarUsuarios nunca carrega a coluna `senha`. O CPF vem completo: mascarar na borda da API.
	ListarUsuarios(f FiltroUsuariosAdmin) ([]models.Usuario, int64, error)
	// BuscarUsuario devolve o usuário sem a coluna `senha` (nil se não existir).
	BuscarUsuario(id uint) (*models.Usuario, error)
	// ListarDesafios carrega criador (resumo) e academia; não carrega participantes.
	ListarDesafios(f FiltroDesafiosAdmin) ([]models.Desafio, int64, error)
	// ListarAcademias / ListarInstrutores carregam o usuário de acesso (resumo, sem senha/CPF).
	ListarAcademias(f FiltroCadastrosAdmin) ([]models.Academia, int64, error)
	ListarInstrutores(f FiltroCadastrosAdmin) ([]models.Instrutor, int64, error)
	ListarReceitas(f FiltroReceitasAdmin) ([]models.PlataformaReceita, int64, error)
	// ReceitaPorOrigem soma plataforma_receitas por origem no período (chaves ausentes = 0).
	ReceitaPorOrigem(p Periodo) (map[string]int64, error)
	// ListarCandidaturas carrega o instrutor e o usuário dele (resumo).
	ListarCandidaturas(f FiltroCandidaturasAdmin) ([]models.CandidaturaInstrutor, int64, error)
	// ListarReservas lista desafios com reserva (coluna > 0 ou movimento no ledger), mais recentes primeiro.
	ListarReservas(f FiltroReservasAdmin) ([]ReservaDesafioResumo, int64, error)
	// MovimentosReserva devolve as linhas reserva_desafio/estorno_reserva do desafio (ordem cronológica).
	MovimentosReserva(idDesafio uint) ([]models.Transacao, error)
	ResumoFinanceiro() (ResumoFinanceiroAdmin, error)
	// TransacoesDerivadas devolve as linhas ligadas a `idOrigem` por id_transacao_origem (ex.: tarifa do saque).
	TransacoesDerivadas(idOrigem uint) ([]models.Transacao, error)
	// ContarDesafiosPorStatus conta desafios por status (dashboard).
	ContarDesafiosPorStatus() (map[string]int64, error)
	// BuscarAcademia / BuscarInstrutor carregam o usuário (resumo). nil se não existir.
	BuscarAcademia(id uint) (*models.Academia, error)
	BuscarInstrutor(id uint) (*models.Instrutor, error)
	// TarifasDosSaques devolve, por id de saque, a tarifa efetivamente cobrada (centavos positivos).
	TarifasDosSaques(ids []uint) (map[uint]int64, error)
	// UsuariosResumo devolve id, nome, sobrenome, email, perfil e banido_em dos usuários pedidos.
	UsuariosResumo(ids []uint) (map[uint]models.Usuario, error)
	// TotaisLedgerDesafio soma o ledger efetivo (received/concluido) do desafio por tipo.
	TotaisLedgerDesafio(idDesafio uint) (map[string]int64, error)
	// ParticipacoesUsuario devolve os desafios em que o usuário está inscrito (mais recentes primeiro, até 100).
	ParticipacoesUsuario(idUsuario uint) ([]models.Desafio, error)
}

type adminConsultaRepository struct{ db *gorm.DB }

// NovoAdminConsultaRepository cria uma nova instância do repositório de consultas do admin
func NovoAdminConsultaRepository(db *gorm.DB) AdminConsultaRepository {
	return &adminConsultaRepository{db}
}

// paginarContando conta o total e devolve a página (offset/limit normalizados).
func paginarContando[T any](q *gorm.DB, pagina, limite int, ordem string, prep func(*gorm.DB) *gorm.DB) ([]T, int64, error) {
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset, lim := paginarAdmin(pagina, limite)
	if prep != nil {
		q = prep(q)
	}
	lista := []T{}
	err := q.Order(ordem).Offset(offset).Limit(lim).Find(&lista).Error
	return lista, total, err
}

func (r *adminConsultaRepository) ListarTransacoes(f FiltroTransacoesAdmin) ([]models.Transacao, int64, error) {
	q := r.db.Model(&models.Transacao{})
	if f.Tipo != "" {
		q = q.Where("tipo = ?", f.Tipo)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.IDUsuario != 0 {
		q = q.Where("id_usuario = ?", f.IDUsuario)
	}
	if f.IDDesafio != 0 {
		q = q.Where("id_desafio = ?", f.IDDesafio)
	}
	if f.EmRevisao != nil {
		q = q.Where("em_revisao = ?", *f.EmRevisao)
	}
	q = f.Periodo.aplicar(q, "created_at")
	return paginarContando[models.Transacao](q, f.Pagina, f.Limite, "created_at DESC, id DESC", nil)
}

func (r *adminConsultaRepository) BuscarTransacao(id uint) (*models.Transacao, error) {
	var t models.Transacao
	err := r.db.First(&t, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (r *adminConsultaRepository) ListarSaques(status string, emRevisao *bool, pagina, limite int) ([]models.Transacao, int64, error) {
	return r.ListarTransacoes(FiltroTransacoesAdmin{
		Tipo: models.TipoSaque, Status: status, EmRevisao: emRevisao, Pagina: pagina, Limite: limite,
	})
}

func (r *adminConsultaRepository) ListarUsuarios(f FiltroUsuariosAdmin) ([]models.Usuario, int64, error) {
	q := r.db.Model(&models.Usuario{})
	if f.Q != "" {
		like := "%" + escapeLike(f.Q) + "%"
		q = q.Where("(nome LIKE ? OR sobrenome LIKE ? OR email LIKE ? OR username LIKE ? OR cpf = ?)",
			like, like, like, like, f.Q)
	}
	// Filtro por papel (rodada 4): "atleta" e "academia" são o TIPO DE CONTA (todas as contas de atleta,
	// inclusive as que também são instrutor); "instrutor" = conta de atleta com o modo instrutor liberado
	// (pedido aprovado). Pedidos pendentes/rejeitados: GET /api/admin/instrutores?status=.
	switch f.Perfil {
	case "":
	case "instrutor":
		q = q.Where("perfil = 'atleta' AND EXISTS (SELECT 1 FROM instrutores il WHERE il.id_usuario = usuarios.id AND il.status = 'aprovada')")
	default:
		q = q.Where("perfil = ?", f.Perfil)
	}
	agora := f.Agora
	if agora.IsZero() {
		agora = time.Now()
	}
	switch f.Situacao {
	case SituacaoBanido:
		q = q.Where("banido_em IS NOT NULL")
	case SituacaoSuspenso:
		q = q.Where("banido_em IS NULL AND suspenso_ate > ?", agora)
	case SituacaoAtivo:
		q = q.Where("banido_em IS NULL AND (suspenso_ate IS NULL OR suspenso_ate <= ?)", agora)
	}
	// Bloqueado (compatibilidade) = banido (bloqueio unificado nas punições, 00019).
	if f.Bloqueado != nil {
		if *f.Bloqueado {
			q = q.Where("banido_em IS NOT NULL")
		} else {
			q = q.Where("banido_em IS NULL")
		}
	}
	return paginarContando[models.Usuario](q, f.Pagina, f.Limite, "id DESC",
		func(q *gorm.DB) *gorm.DB { return q.Select(colunasUsuarioAdmin) })
}

func (r *adminConsultaRepository) BuscarUsuario(id uint) (*models.Usuario, error) {
	var u models.Usuario
	err := r.db.Select(colunasUsuarioAdmin).First(&u, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *adminConsultaRepository) ListarDesafios(f FiltroDesafiosAdmin) ([]models.Desafio, int64, error) {
	q := r.db.Model(&models.Desafio{})
	if f.Vencidos {
		agora := time.Now()
		aberto, andamento := f.LimiteAberto, f.LimiteAndamento
		if aberto.IsZero() {
			aberto = agora
		}
		if andamento.IsZero() {
			andamento = agora
		}
		q = q.Where("((status = ? AND data_evento < ?) OR (status = ? AND "+sqlVencidoAndamento+"))",
			models.StatusAberto, aberto, models.StatusEmAndamento, andamento, andamento)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	if f.IDAcademia != 0 {
		q = q.Where("id_academia = ?", f.IDAcademia)
	}
	return paginarContando[models.Desafio](q, f.Pagina, f.Limite, "criado_em DESC, id DESC",
		func(q *gorm.DB) *gorm.DB {
			return q.Preload("Criador", selecionarUsuarioResumo).Preload("Academia")
		})
}

func (r *adminConsultaRepository) listarCadastros(model any, status string) *gorm.DB {
	q := r.db.Model(model)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	return q
}

func (r *adminConsultaRepository) ListarAcademias(f FiltroCadastrosAdmin) ([]models.Academia, int64, error) {
	return paginarContando[models.Academia](r.listarCadastros(&models.Academia{}, f.Status), f.Pagina, f.Limite,
		"criado_em ASC, id ASC", func(q *gorm.DB) *gorm.DB { return q.Preload("Usuario", selecionarUsuarioResumo) })
}

func (r *adminConsultaRepository) ListarInstrutores(f FiltroCadastrosAdmin) ([]models.Instrutor, int64, error) {
	return paginarContando[models.Instrutor](r.listarCadastros(&models.Instrutor{}, f.Status), f.Pagina, f.Limite,
		"criado_em ASC, id ASC", func(q *gorm.DB) *gorm.DB { return q.Preload("Usuario", selecionarUsuarioResumo) })
}

func (r *adminConsultaRepository) ListarReceitas(f FiltroReceitasAdmin) ([]models.PlataformaReceita, int64, error) {
	q := r.db.Model(&models.PlataformaReceita{})
	if f.Origem != "" {
		q = q.Where("origem = ?", f.Origem)
	}
	if f.IDDesafio != 0 {
		q = q.Where("id_desafio = ?", f.IDDesafio)
	}
	q = f.Periodo.aplicar(q, "criado_em")
	return paginarContando[models.PlataformaReceita](q, f.Pagina, f.Limite, "criado_em DESC, id DESC", nil)
}

func (r *adminConsultaRepository) ReceitaPorOrigem(p Periodo) (map[string]int64, error) {
	var linhas []struct {
		Origem string
		Total  int64
	}
	q := p.aplicar(r.db.Model(&models.PlataformaReceita{}), "criado_em")
	err := q.Select("origem, COALESCE(SUM(valor_centavos), 0) AS total").Group("origem").Scan(&linhas).Error
	out := map[string]int64{models.ReceitaTaxaInscricao: 0, models.ReceitaTarifaSaque: 0}
	for _, l := range linhas {
		out[l.Origem] = l.Total
	}
	return out, err
}

func (r *adminConsultaRepository) ListarCandidaturas(f FiltroCandidaturasAdmin) ([]models.CandidaturaInstrutor, int64, error) {
	q := r.db.Model(&models.CandidaturaInstrutor{})
	if f.IDDesafio != 0 {
		q = q.Where("id_desafio = ?", f.IDDesafio)
	}
	if f.IDInstrutor != 0 {
		q = q.Where("id_instrutor = ?", f.IDInstrutor)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	return paginarContando[models.CandidaturaInstrutor](q, f.Pagina, f.Limite, "criado_em DESC, id DESC",
		func(q *gorm.DB) *gorm.DB {
			return q.Preload("Instrutor").Preload("Instrutor.Usuario", selecionarUsuarioResumo)
		})
}

// ListarReservas: desafios com reserva vigente > 0 ou com qualquer movimento de reserva no ledger.
func (r *adminConsultaRepository) ListarReservas(f FiltroReservasAdmin) ([]ReservaDesafioResumo, int64, error) {
	base := r.db.Table("desafios d").
		Joins(`LEFT JOIN (SELECT id_desafio, -SUM(valor_centavos) AS retido FROM transacoes
		         WHERE tipo IN (?, ?) AND status IN (?, ?) GROUP BY id_desafio) m ON m.id_desafio = d.id`,
			models.TipoReservaDesafio, models.TipoEstornoReserva, models.TransacaoReceived, models.TransacaoConcluido).
		Where("(d.reserva_academia_centavos > 0 OR m.id_desafio IS NOT NULL)")
	if f.Status != "" {
		base = base.Where("d.status = ?", f.Status)
	}
	if f.IDAcademia != 0 {
		base = base.Where("d.id_academia = ?", f.IDAcademia)
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	offset, lim := paginarAdmin(f.Pagina, f.Limite)
	lista := []ReservaDesafioResumo{}
	err := base.Select(`d.id AS id_desafio, d.titulo, d.status, d.id_academia, d.id_criador,
			d.reserva_academia_centavos AS reserva_centavos, COALESCE(m.retido, 0) AS retido_centavos`).
		Order("d.id DESC").Offset(offset).Limit(lim).Scan(&lista).Error
	return lista, total, err
}

func (r *adminConsultaRepository) MovimentosReserva(idDesafio uint) ([]models.Transacao, error) {
	lista := []models.Transacao{}
	err := r.db.Where("id_desafio = ? AND tipo IN ?", idDesafio,
		[]models.TipoTransacao{models.TipoReservaDesafio, models.TipoEstornoReserva}).
		Order("created_at ASC, id ASC").Find(&lista).Error
	return lista, err
}

func (r *adminConsultaRepository) ResumoFinanceiro() (ResumoFinanceiroAdmin, error) {
	var res ResumoFinanceiroAdmin
	efetivo := "status IN ('received','concluido')"
	err := r.db.Raw(`SELECT
		(SELECT COALESCE(SUM(saldo_centavos), 0) FROM usuarios) AS soma_saldos,
		(SELECT COALESCE(SUM(valor_centavos), 0) FROM transacoes
		  WHERE ` + efetivo + ` OR (tipo = 'saque' AND status = 'pending')) AS soma_ledger,
		(SELECT COALESCE(SUM(valor_centavos), 0) FROM transacoes WHERE tipo = 'deposito' AND status = 'received') AS depositos,
		(SELECT COALESCE(-SUM(valor_centavos), 0) FROM transacoes WHERE tipo = 'saque' AND status IN ('pending','concluido')) AS saques,
		(SELECT COALESCE(SUM(valor_centavos), 0) FROM transacoes WHERE tipo = 'ajuste' AND ` + efetivo + `) AS ajustes,
		(SELECT COALESCE(-SUM(t.valor_centavos), 0) FROM transacoes t JOIN desafios d ON d.id = t.id_desafio
		  WHERE d.status IN ('aberto','em_andamento') AND t.tipo IN ('entrada_desafio','estorno_desafio')
		    AND t.status IN ('received','concluido')) AS custodia_entradas,
		(SELECT COALESCE(-SUM(t.valor_centavos), 0) FROM transacoes t JOIN desafios d ON d.id = t.id_desafio
		  WHERE d.status IN ('aberto','em_andamento') AND t.tipo IN ('reserva_desafio','estorno_reserva')
		    AND t.status IN ('received','concluido')) AS custodia_reservas,
		(SELECT COALESCE(SUM(valor_centavos), 0) FROM plataforma_receitas) AS receitas,
		(SELECT COALESCE(SUM(valor_centavos), 0) FROM pagamentos_pendentes
		  WHERE status IN ('a_liberar','retido') AND id_usuario IS NOT NULL) AS pendentes_usuarios,
		(SELECT COALESCE(SUM(valor_centavos), 0) FROM pagamentos_pendentes
		  WHERE status IN ('a_liberar','retido') AND id_usuario IS NULL) AS pendentes_plataforma,
		(SELECT COALESCE(SUM(saldo_restante_centavos), 0) FROM usuario_debitos WHERE quitado_em IS NULL) AS debitos_abertos`).
		Scan(&res).Error
	return res, err
}

func (r *adminConsultaRepository) TransacoesDerivadas(idOrigem uint) ([]models.Transacao, error) {
	lista := []models.Transacao{}
	err := r.db.Where("id_transacao_origem = ?", idOrigem).Order("id ASC").Find(&lista).Error
	return lista, err
}

func (r *adminConsultaRepository) ContarDesafiosPorStatus() (map[string]int64, error) {
	var linhas []struct {
		Status string
		Total  int64
	}
	err := r.db.Model(&models.Desafio{}).Select("status, COUNT(*) AS total").Group("status").Scan(&linhas).Error
	out := map[string]int64{}
	for _, l := range linhas {
		out[l.Status] = l.Total
	}
	return out, err
}

func (r *adminConsultaRepository) BuscarAcademia(id uint) (*models.Academia, error) {
	var a models.Academia
	err := r.db.Preload("Usuario", selecionarUsuarioResumo).First(&a, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *adminConsultaRepository) BuscarInstrutor(id uint) (*models.Instrutor, error) {
	var i models.Instrutor
	err := r.db.Preload("Usuario", selecionarUsuarioResumo).First(&i, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &i, nil
}

func (r *adminConsultaRepository) TarifasDosSaques(ids []uint) (map[uint]int64, error) {
	out := make(map[uint]int64, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var linhas []struct {
		IDTransacaoOrigem uint
		Total             int64
	}
	err := r.db.Model(&models.Transacao{}).
		Select("id_transacao_origem, -SUM(valor_centavos) AS total").
		Where("id_transacao_origem IN ? AND tipo = ? AND status = ?", ids, models.TipoTarifaSaque, models.TransacaoConcluido).
		Group("id_transacao_origem").Scan(&linhas).Error
	for _, l := range linhas {
		out[l.IDTransacaoOrigem] = l.Total
	}
	return out, err
}

func (r *adminConsultaRepository) UsuariosResumo(ids []uint) (map[uint]models.Usuario, error) {
	out := make(map[uint]models.Usuario, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	var lista []models.Usuario
	err := selecionarUsuarioResumo(r.db.Model(&models.Usuario{})).Where("id IN ?", ids).Find(&lista).Error
	for _, u := range lista {
		out[u.ID] = u
	}
	return out, err
}

func (r *adminConsultaRepository) TotaisLedgerDesafio(idDesafio uint) (map[string]int64, error) {
	var linhas []struct {
		Tipo  string
		Total int64
	}
	err := r.db.Model(&models.Transacao{}).Select("tipo, COALESCE(SUM(valor_centavos), 0) AS total").
		Where("id_desafio = ? AND status IN ?", idDesafio, []string{models.TransacaoReceived, models.TransacaoConcluido}).
		Group("tipo").Scan(&linhas).Error
	out := map[string]int64{}
	for _, l := range linhas {
		out[l.Tipo] = l.Total
	}
	return out, err
}

func (r *adminConsultaRepository) ParticipacoesUsuario(idUsuario uint) ([]models.Desafio, error) {
	lista := []models.Desafio{}
	err := r.db.Joins("JOIN desafio_participantes dp ON dp.id_desafio = desafios.id").
		Where("dp.id_usuario = ?", idUsuario).
		Preload("Participantes", "id_usuario = ?", idUsuario).
		Order("desafios.criado_em DESC, desafios.id DESC").Limit(100).Find(&lista).Error
	return lista, err
}
