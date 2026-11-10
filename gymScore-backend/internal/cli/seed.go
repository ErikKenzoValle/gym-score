package cli

// Comando `seed-teste`: cria (ou atualiza) contas FICTÍCIAS para testar o app à mão.
//
//	go run . seed-teste
//
// Recusa rodar com APP_ENV=production. Idempotente: as contas são reconhecidas pelo e-mail e
// reaproveitadas; o saldo inicial só é creditado uma vez (linha `ajuste` do ledger com descrição
// própria). Todas as contas usam a senha SenhaContasTeste.

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/internal/services"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// SenhaContasTeste é a senha de TODAS as contas criadas pelo seed (inclusive o admin: só existe fora
// de produção, por isso pode ficar abaixo do mínimo de 12 caracteres das senhas de admin).
const SenhaContasTeste = "teste123"

// E-mails fictícios das contas do seed (domínio reservado `.test`, nunca entrega e-mail).
const (
	EmailSeedAtleta            = "atleta@gymscore.test"
	EmailSeedInstrutor         = "instrutor@gymscore.test"
	EmailSeedInstrutorPendente = "instrutor.pendente@gymscore.test"
	EmailSeedAcademia          = "academia@gymscore.test"
	EmailSeedAcademiaPendente  = "academia.pendente@gymscore.test"
	EmailSeedAdmin             = "admin@gymscore.test"
)

// Valores fixos do seed.
const (
	SaldoInicialSeedCentavos int64 = 50_000 // R$ 500,00 para o atleta comum
	descricaoSaldoSeed             = "Saldo inicial de teste (seed-teste)"
	TituloDesafioSeed              = "Desafio de teste: 5 km na esteira"
	entradaDesafioSeed       int64 = 2_000 // R$ 20,00
	instrutorPctSeed               = 10    // POR instrutor (rodada 5)
	// InstrutoresVagasSeed: a equipe do desafio de teste tem 2 vagas — o instrutor aprovado indicado (e
	// responsável) e 1 vaga aberta à candidatura.
	InstrutoresVagasSeed = 2
	duracaoEventoSeed    = 2 * time.Hour
	// TituloDesafioAvancadoSeed: 2º desafio aberto, com filtros (nível 5+ e 18+) e premiação de 5 posições
	// definida pela academia — a atleta de teste (nível alto pelo histórico) entra; a conta pendente não.
	TituloDesafioAvancadoSeed = "Desafio de teste: Funcional avançado (nível 5+)"
	nivelMinAvancadoSeed      = 5
	idadeMinAvancadoSeed      = 18
	resumoSeed                = "Desafio fictício criado pelo seed-teste para testar o app à mão."
	regrasSeed                = "Regras de teste: apresente o ingresso (QR) ao instrutor, cumpra o percurso/circuito " +
		"inteiro e respeite as orientações da equipe. A classificação é pela marca registrada pelo instrutor responsável."
)

// divisaoAvancadoSeed é a divisão do prêmio (pontos-base) do desafio avançado: 35/25/20/12/8%.
var divisaoAvancadoSeed = []int{3500, 2500, 2000, 1200, 800}

// ErrSeedEmProducao é devolvido quando APP_ENV indica produção.
var ErrSeedEmProducao = errors.New("seed-teste recusado: APP_ENV=production (as contas de teste nunca existem em produção)")

// ContaSeed descreve uma conta criada/atualizada pelo seed.
type ContaSeed struct {
	Email  string
	Papel  string
	ID     uint // usuarios.id (ou administradores.id para o admin)
	Criada bool // false = já existia e foi atualizada
}

// ResultadoSeed resume o que o seed fez.
type ResultadoSeed struct {
	Contas        []ContaSeed
	IDAdmin       uint
	IDAcademia    uint // academias.id da academia aprovada
	IDInstrutor   uint // instrutores.id do instrutor aprovado
	IDDesafio     uint
	DesafioCriado bool
	// Rodada 6: 2º desafio aberto (filtros + premiação configurada) e o histórico para os gráficos.
	IDDesafioAvancado uint
	HistoricoCriado   int // desafios encerrados de histórico criados nesta execução
	TreinosCriados    int // contas que ganharam o histórico de treinos nesta execução
	SaldoCreditado    bool
	IDAuditoria       uint
	TaxaPlataforma    int
	PercentualPote    int
	DataEventoDoSeed  time.Time
}

// EhProducao diz se o APP_ENV é de produção (mesma regra de config.IsProduction, sem diferenciar
// maiúsculas e espaços — na dúvida, recusa).
func EhProducao(appEnv string) bool {
	e := strings.ToLower(strings.TrimSpace(appEnv))
	return e == "production" || e == "prod"
}

// SeedTeste cria/atualiza as contas de teste numa única transação (tudo ou nada) e registra a
// auditoria `cli_seed`. taxaPct é a taxa da plataforma gravada no desafio (PLATAFORMA_TAXA_INSCRICAO_PCT).
func SeedTeste(appEnv string, db *gorm.DB, taxaPct int, saida io.Writer) (*ResultadoSeed, error) {
	if EhProducao(appEnv) {
		return nil, ErrSeedEmProducao
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(SenhaContasTeste), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	s := &semeador{hash: string(hash), agora: time.Now(), res: &ResultadoSeed{TaxaPlataforma: taxaPct},
		carteira: repositories.NovoCarteiraRepository(db)}
	err = db.Transaction(func(tx *gorm.DB) error {
		s.tx = tx
		return s.executar(taxaPct)
	})
	if err != nil {
		return nil, err
	}
	if saida != nil {
		imprimirSeed(saida, s.res)
	}
	return s.res, nil
}

type semeador struct {
	tx       *gorm.DB
	hash     string
	agora    time.Time
	res      *ResultadoSeed
	carteira *repositories.CarteiraRepository
}

func (s *semeador) executar(taxaPct int) error {
	// 1. Admin (superadmin): aprova os cadastros e assina a auditoria.
	admin, err := s.admin()
	if err != nil {
		return err
	}
	s.res.IDAdmin = admin.ID

	// 2. Academia aprovada (com endereço estruturado) e academia pendente.
	acad, err := s.academia(EmailSeedAcademia, "Carla", "Gestora", "Academia Teste GymScore Ltda", "Academia Teste",
		cnpjFicticio("990000010001"), models.CadastroAprovado, admin.ID,
		models.EnderecoAcademiaRequest{CEP: "01001-000", Logradouro: "Rua Fictícia dos Testes", Numero: "100",
			Complemento: "Sala 1", Bairro: "Centro", Cidade: "São Paulo", UF: "SP"})
	if err != nil {
		return err
	}
	s.res.IDAcademia = acad.ID
	if _, err := s.academia(EmailSeedAcademiaPendente, "Pedro", "Pendente", "Academia Pendente de Teste Ltda",
		"Academia Pendente", cnpjFicticio("990000020001"), models.CadastroPendente, admin.ID,
		models.EnderecoAcademiaRequest{CEP: "20010-000", Logradouro: "Avenida Imaginária", Numero: "S/N",
			Bairro: "Centro", Cidade: "Rio de Janeiro", UF: "rj"}); err != nil {
		return err
	}

	// 3. Atletas: comum (com saldo), instrutor aprovado vinculado à academia, pedido de instrutor pendente.
	atleta, err := s.atleta(EmailSeedAtleta, "Ana", "Atleta", "ana_atleta", cpfFicticio("900000001"), "F", "atleta comum")
	if err != nil {
		return err
	}
	instr, err := s.atleta(EmailSeedInstrutor, "Igor", "Instrutor", "igor_instrutor", cpfFicticio("900000002"), "M",
		"atleta + modo instrutor aprovado")
	if err != nil {
		return err
	}
	pend, err := s.atleta(EmailSeedInstrutorPendente, "Paula", "Pendente", "paula_pendente", cpfFicticio("900000003"),
		"F", "atleta com pedido de instrutor pendente")
	if err != nil {
		return err
	}
	i, err := s.instrutor(instr.ID, "000001-G/SP", "Crossfit e corrida", models.CadastroAprovado, &acad.ID, admin.ID)
	if err != nil {
		return err
	}
	s.res.IDInstrutor = i.ID
	if _, err := s.instrutor(pend.ID, "000002-G/RJ", "Musculação", models.CadastroPendente, nil, admin.ID); err != nil {
		return err
	}

	// 4. Saldo inicial do atleta comum: SÓ pelo ledger (Creditar `ajuste`), uma única vez.
	if err := s.saldoInicial(atleta.ID); err != nil {
		return err
	}

	// 5. Desafio aberto da academia aprovada: equipe de 2 vagas com o instrutor aprovado indicado e
	// responsável e 1 vaga aberta à candidatura (sem reserva: instrutor só por percentual).
	if err := s.desafio(acad, i, taxaPct); err != nil {
		return err
	}

	// 5b. Rodada 6: histórico para os gráficos de teia e o Nível GymScore (desafios encerrados GRATUITOS —
	// nenhum dinheiro se move — e treinos registrados nos últimos 180 dias), o 2º desafio aberto com filtros
	// e premiação configurada, e o nível recalculado das contas.
	if err := s.historico(acad, atleta, pend, instr, i); err != nil {
		return err
	}
	if err := s.desafioAvancado(acad, i, taxaPct); err != nil {
		return err
	}
	if err := services.RecalcularGymScoreNaTransacao(s.tx, []uint{atleta.ID, instr.ID, pend.ID}, s.agora); err != nil {
		return err
	}

	// 6. Auditoria na mesma transação.
	emails := make([]string, 0, len(s.res.Contas))
	for _, c := range s.res.Contas {
		emails = append(emails, c.Email)
	}
	a := &models.AdminAuditoria{}
	if err := services.RegistrarAuditoria(repositories.NovoAuditoriaRepository(s.tx), s.tx, admin.ID, "cli",
		services.AcaoCLISeed, services.AlvoAdmin, admin.ID, "Contas de teste criadas/atualizadas pelo seed-teste",
		map[string]any{"contas": emails, "id_desafio": s.res.IDDesafio, "desafio_criado": s.res.DesafioCriado,
			"saldo_creditado": s.res.SaldoCreditado}); err != nil {
		return err
	}
	if err := s.tx.Where("acao = ? AND id_admin = ?", services.AcaoCLISeed, admin.ID).Order("id DESC").
		Limit(1).Find(a).Error; err != nil {
		return err
	}
	s.res.IDAuditoria = a.ID
	return nil
}

// senhaConfere evita regravar o hash (e derrubar sessões) quando a senha já é a de teste.
func senhaConfere(hash string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(SenhaContasTeste)) == nil
}

func (s *semeador) registrar(email, papel string, id uint, criada bool) {
	s.res.Contas = append(s.res.Contas, ContaSeed{Email: email, Papel: papel, ID: id, Criada: criada})
}

func (s *semeador) admin() (*models.Administrador, error) {
	repo := repositories.NovoAdminRepository(s.tx)
	var a models.Administrador
	err := s.tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("email = ?", EmailSeedAdmin).Limit(1).Find(&a).Error
	if err != nil {
		return nil, err
	}
	if a.ID == 0 {
		a = models.Administrador{Nome: "Admin de Teste", Email: EmailSeedAdmin, SenhaHash: s.hash,
			Papel: models.PapelSuperadmin, Ativo: true}
		if err := repo.Criar(s.tx, &a); err != nil {
			return nil, err
		}
		s.registrar(EmailSeedAdmin, "superadmin (painel /admin)", a.ID, true)
		return &a, nil
	}
	if !senhaConfere(a.SenhaHash) {
		if _, err := repo.AlterarSenha(s.tx, a.ID, s.hash); err != nil {
			return nil, err
		}
	}
	if a.Papel != models.PapelSuperadmin {
		if _, err := repo.AlterarPapel(s.tx, a.ID, models.PapelSuperadmin); err != nil {
			return nil, err
		}
	}
	if !a.Ativo {
		if _, err := repo.AlterarAtivo(s.tx, a.ID, true); err != nil {
			return nil, err
		}
	}
	s.registrar(EmailSeedAdmin, "superadmin (painel /admin)", a.ID, false)
	return &a, nil
}

// usuario busca a conta pelo e-mail (travada) e cria ou atualiza nome/senha. Recusa reaproveitar uma
// conta de outro tipo (atleta × academia): o seed nunca converte contas.
func (s *semeador) usuario(email, perfil string, novo func() *models.Usuario, papel string) (*models.Usuario, error) {
	var u models.Usuario
	err := s.tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("email = ?", email).Limit(1).Find(&u).Error
	if err != nil {
		return nil, err
	}
	if u.ID == 0 {
		n := novo()
		n.Email, n.Perfil, n.Senha = email, perfil, s.hash
		if err := s.tx.Create(n).Error; err != nil {
			if repositories.EhDuplicata(err) {
				return nil, fmt.Errorf("não foi possível criar %s: CPF, username ou CNPJ fictício já usado por outra conta: %w", email, err)
			}
			return nil, err
		}
		s.registrar(email, papel, n.ID, true)
		return n, nil
	}
	if u.Perfil != perfil {
		return nil, fmt.Errorf("a conta %s já existe com tipo %q (esperado %q): o seed não converte contas", email, u.Perfil, perfil)
	}
	n := novo()
	campos := map[string]any{"nome": n.Nome, "sobrenome": n.Sobrenome}
	if !senhaConfere(u.Senha) {
		campos["senha"] = s.hash
	}
	if err := s.tx.Model(&models.Usuario{}).Where("id = ?", u.ID).Updates(campos).Error; err != nil {
		return nil, err
	}
	s.registrar(email, papel, u.ID, false)
	return &u, nil
}

func (s *semeador) atleta(email, nome, sobrenome, username, cpf, genero, papel string) (*models.Usuario, error) {
	return s.usuario(email, models.PerfilAtleta, func() *models.Usuario {
		nasc := time.Date(1995, time.March, 15, 0, 0, 0, 0, time.UTC)
		return &models.Usuario{Nome: nome, Sobrenome: sobrenome, Username: username, CPF: &cpf,
			DataNascimento: &nasc, Genero: &genero}
	}, papel)
}

func (s *semeador) academia(email, nome, sobrenome, razao, fantasia, cnpj, status string, idAdmin uint,
	end models.EnderecoAcademiaRequest) (*models.Academia, error) {
	papel := "academia aprovada"
	if status == models.CadastroPendente {
		papel = "academia pendente de aprovação"
	}
	u, err := s.usuario(email, models.PerfilAcademia, func() *models.Usuario {
		return &models.Usuario{Nome: nome, Sobrenome: sobrenome}
	}, papel)
	if err != nil {
		return nil, err
	}
	var a models.Academia
	if err := s.tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id_usuario = ?", u.ID).Limit(1).Find(&a).Error; err != nil {
		return nil, err
	}
	novo := a.ID == 0
	a.IDUsuario, a.RazaoSocial, a.NomeFantasia, a.Telefone = u.ID, razao, fantasia, "(11) 90000-0000"
	if novo {
		a.CNPJ = cnpj
	}
	if err := services.AplicarEnderecoAcademia(&a, &end); err != nil {
		return nil, err
	}
	a.Status, a.MotivoRejeicao, a.AprovadoPor, a.AprovadoEm = status, nil, nil, nil
	if status == models.CadastroAprovado {
		em := s.agora
		a.AprovadoPor, a.AprovadoEm = &idAdmin, &em
	}
	if novo {
		err = s.tx.Create(&a).Error
	} else {
		err = s.tx.Model(&models.Academia{}).Where("id = ?", a.ID).Updates(map[string]any{
			"razao_social": a.RazaoSocial, "nome_fantasia": a.NomeFantasia, "telefone": a.Telefone,
			"endereco": a.Endereco, "cep": a.CEP, "logradouro": a.Logradouro, "numero": a.Numero,
			"complemento": a.Complemento, "bairro": a.Bairro, "cidade": a.Cidade, "uf": a.UF,
			"status": a.Status, "motivo_rejeicao": nil, "aprovado_por": a.AprovadoPor, "aprovado_em": a.AprovadoEm,
		}).Error
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *semeador) instrutor(idUsuario uint, cref, especialidade, status string, idAcademia *uint, idAdmin uint) (*models.Instrutor, error) {
	var i models.Instrutor
	if err := s.tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id_usuario = ?", idUsuario).Limit(1).Find(&i).Error; err != nil {
		return nil, err
	}
	var aprovadoPor *uint
	var aprovadoEm *time.Time
	if status == models.CadastroAprovado {
		em := s.agora
		aprovadoPor, aprovadoEm = &idAdmin, &em
	}
	if i.ID == 0 {
		i = models.Instrutor{IDUsuario: idUsuario, CREF: cref, Especialidade: especialidade, IDAcademia: idAcademia,
			Status: status, AprovadoPor: aprovadoPor, AprovadoEm: aprovadoEm}
		if err := s.tx.Create(&i).Error; err != nil {
			return nil, err
		}
		return &i, nil
	}
	err := s.tx.Model(&models.Instrutor{}).Where("id = ?", i.ID).Updates(map[string]any{
		"cref": cref, "especialidade": especialidade, "id_academia": idAcademia, "id_academia_convite": nil,
		"convite_em": nil, "status": status, "motivo_rejeicao": nil, "aprovado_por": aprovadoPor, "aprovado_em": aprovadoEm,
	}).Error
	if err != nil {
		return nil, err
	}
	i.IDAcademia, i.Status = idAcademia, status
	return &i, nil
}

// saldoInicial credita o saldo do atleta comum pelo ledger (`ajuste`), só se ainda não houver o crédito do seed.
func (s *semeador) saldoInicial(idUsuario uint) error {
	var n int64
	err := s.tx.Model(&models.Transacao{}).
		Where("id_usuario = ? AND tipo = ? AND descricao = ?", idUsuario, models.TipoAjuste, descricaoSaldoSeed).
		Count(&n).Error
	if err != nil || n > 0 {
		return err
	}
	_, err = s.carteira.Creditar(s.tx, repositories.Lancamento{IDUsuario: idUsuario, Tipo: models.TipoAjuste,
		ValorCentavos: SaldoInicialSeedCentavos, Descricao: descricaoSaldoSeed})
	if err == nil {
		s.res.SaldoCreditado = true
	}
	return err
}

// desafio reaproveita o desafio aberto do seed (data futura, equipe de InstrutoresVagasSeed vagas) ou cria
// um novo, com o endereço da academia, início e fim, e o instrutor aprovado indicado como responsável.
// Configuração sem reserva (sem prêmio fixo; instrutor por percentual): nenhum dinheiro se move.
func (s *semeador) desafio(acad *models.Academia, instr *models.Instrutor, taxaPct int) error {
	var d models.Desafio
	err := s.tx.Where("id_academia = ? AND titulo = ? AND status = ? AND data_evento > ? AND instrutores_vagas = ?",
		acad.ID, TituloDesafioSeed, models.StatusAberto, s.agora.Add(24*time.Hour), InstrutoresVagasSeed).
		Order("id DESC").Limit(1).Find(&d).Error
	if err != nil {
		return err
	}
	if d.ID != 0 {
		s.res.IDDesafio = d.ID
		if d.DataEvento != nil {
			s.res.DataEventoDoSeed = *d.DataEvento
		}
		s.res.PercentualPote = d.PercentualPote
		if err := s.garantirTextoEDivisao(&d, nil); err != nil {
			return err
		}
		return s.garantirIndicado(&d, instr)
	}
	cfg := services.ConfigFinanceira{DividirPote: true, Taxa: taxaPct, Modo: models.InstrutorPercentual,
		Pct: instrutorPctSeed, Vagas: InstrutoresVagasSeed}
	if err := cfg.Validar(true); err != nil {
		return fmt.Errorf("configuração financeira do desafio de teste: %w", err)
	}
	if cfg.Reserva() != 0 {
		return errors.New("o desafio de teste não pode exigir reserva")
	}
	base := s.agora.AddDate(0, 0, 30)
	evento := time.Date(base.Year(), base.Month(), base.Day(), 8, 0, 0, 0, time.Local)
	fim := evento.Add(duracaoEventoSeed)
	desc := "Desafio fictício criado pelo seed-teste: inscreva-se com a conta de atleta."
	resumo, regras := resumoSeed, regrasSeed
	d = models.Desafio{Titulo: TituloDesafioSeed, Descricao: &desc, Resumo: &resumo, Regras: &regras, Categoria: "corrida",
		ValorEntradaCentavos: entradaDesafioSeed, DataEvento: &evento, DataFimEvento: &fim, Status: models.StatusAberto,
		Vagas: 20, PosicoesPodio: 3, IDCriador: acad.IDUsuario, IDAcademia: &acad.ID, IDInstrutor: &instr.ID,
		DividirPote: cfg.DividirPote, PercentualPote: cfg.PercentualPote, TaxaPlataformaPct: cfg.Taxa,
		InstrutorModoPagamento: cfg.Modo, InstrutorPct: cfg.Pct, InstrutoresVagas: cfg.Vagas,
		InstrutorSelecao: models.SelecaoDesignado}
	// O local do desafio é o endereço estruturado da academia (o formulário do app pré-preenche igual).
	if err := services.AplicarEnderecoDesafio(&d, enderecoDaAcademia(acad)); err != nil {
		return err
	}
	if err := repositories.NovoDesafioRepository(s.tx).Criar(&d); err != nil {
		return err
	}
	s.res.IDDesafio, s.res.DesafioCriado, s.res.DataEventoDoSeed, s.res.PercentualPote = d.ID, true, evento, d.PercentualPote
	if err := s.garantirTextoEDivisao(&d, nil); err != nil {
		return err
	}
	return s.garantirIndicado(&d, instr)
}

// garantirTextoEDivisao completa resumo/regras (desafios do seed anteriores à rodada 6) e grava a divisão
// do prêmio se o desafio ainda não tiver (divisao nil = padrão do tamanho do pódio). Idempotente.
func (s *semeador) garantirTextoEDivisao(d *models.Desafio, divisao []int) error {
	campos := map[string]any{}
	if d.Resumo == nil {
		campos["resumo"] = resumoSeed
	}
	if d.Regras == nil {
		campos["regras"] = regrasSeed
	}
	if len(campos) > 0 {
		if err := s.tx.Model(&models.Desafio{}).Where("id = ?", d.ID).Updates(campos).Error; err != nil {
			return err
		}
	}
	repo := repositories.NovoDesafioRepository(s.tx)
	atual, err := repo.ListarDivisaoPremio(d.ID)
	if err != nil || len(atual) == d.PosicoesPodio {
		return err
	}
	if divisao == nil {
		divisao = services.DivisaoPadraoBP(d.PosicoesPodio)
	}
	return repo.SubstituirDivisaoPremio(d.ID, divisao)
}

// desafioAvancado cria (uma vez) o 2º desafio aberto do seed: funcional, nível 5 a 10 e 18+ anos,
// 5 premiados com a divisão divisaoAvancadoSeed, equipe de 1 vaga com o instrutor aprovado. Sem reserva.
func (s *semeador) desafioAvancado(acad *models.Academia, instr *models.Instrutor, taxaPct int) error {
	var d models.Desafio
	err := s.tx.Where("id_academia = ? AND titulo = ? AND status = ? AND data_evento > ?",
		acad.ID, TituloDesafioAvancadoSeed, models.StatusAberto, s.agora.Add(24*time.Hour)).
		Order("id DESC").Limit(1).Find(&d).Error
	if err != nil {
		return err
	}
	if d.ID == 0 {
		cfg := services.ConfigFinanceira{DividirPote: true, Taxa: taxaPct, Modo: models.InstrutorPercentual,
			Pct: instrutorPctSeed, Vagas: 1}
		if err := cfg.Validar(true); err != nil {
			return fmt.Errorf("configuração financeira do desafio avançado de teste: %w", err)
		}
		base := s.agora.AddDate(0, 0, 40)
		evento := time.Date(base.Year(), base.Month(), base.Day(), 9, 0, 0, 0, time.Local)
		fim := evento.Add(duracaoEventoSeed)
		resumo := "Circuito funcional avançado (fictício): só para atletas de nível 5 ou mais e maiores de 18 anos."
		regras := regrasSeed + " Requisitos: nível GymScore 5 a 10 e 18 anos ou mais na data do evento. Premiação: 5 posições (35/25/20/12/8%)."
		nivelMin, idadeMin := nivelMinAvancadoSeed, idadeMinAvancadoSeed
		d = models.Desafio{Titulo: TituloDesafioAvancadoSeed, Resumo: &resumo, Regras: &regras, Categoria: "funcional",
			ValorEntradaCentavos: 1_500, DataEvento: &evento, DataFimEvento: &fim, Status: models.StatusAberto,
			Vagas: 30, PosicoesPodio: len(divisaoAvancadoSeed), NivelMin: &nivelMin, IdadeMin: &idadeMin,
			IDCriador: acad.IDUsuario, IDAcademia: &acad.ID, IDInstrutor: &instr.ID,
			DividirPote: cfg.DividirPote, PercentualPote: cfg.PercentualPote, TaxaPlataformaPct: cfg.Taxa,
			InstrutorModoPagamento: cfg.Modo, InstrutorPct: cfg.Pct, InstrutoresVagas: cfg.Vagas,
			InstrutorSelecao: models.SelecaoDesignado}
		if err := services.AplicarEnderecoDesafio(&d, enderecoDaAcademia(acad)); err != nil {
			return err
		}
		if err := repositories.NovoDesafioRepository(s.tx).Criar(&d); err != nil {
			return err
		}
	}
	s.res.IDDesafioAvancado = d.ID
	if err := s.garantirTextoEDivisao(&d, divisaoAvancadoSeed); err != nil {
		return err
	}
	return s.garantirIndicado(&d, instr)
}

// garantirIndicado põe o instrutor aprovado na equipe do desafio do seed como indicado e responsável
// (idempotente), mantendo o espelho legado desafios.id_instrutor.
func (s *semeador) garantirIndicado(d *models.Desafio, instr *models.Instrutor) error {
	repo := repositories.NovoDesafioRepository(s.tx)
	membro, err := repo.MembroDaEquipe(d.ID, instr.ID)
	if err != nil {
		return err
	}
	if !membro {
		if err := repo.AdicionarMembro(&models.DesafioInstrutor{IDDesafio: d.ID, IDInstrutor: instr.ID,
			Origem: models.EquipeOrigemIndicado}); err != nil {
			return err
		}
	}
	_, err = repo.DefinirResponsavel(d.ID, instr.ID)
	return err
}

// enderecoDaAcademia remonta o pedido de endereço a partir das colunas estruturadas da academia.
func enderecoDaAcademia(a *models.Academia) *models.EnderecoAcademiaRequest {
	v := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	return &models.EnderecoAcademiaRequest{CEP: v(a.CEP), Logradouro: v(a.Logradouro), Numero: v(a.Numero),
		Complemento: v(a.Complemento), Bairro: v(a.Bairro), Cidade: v(a.Cidade), UF: v(a.UF)}
}

func imprimirSeed(w io.Writer, r *ResultadoSeed) {
	fmt.Fprintf(w, "Contas de teste prontas (senha de todas: %s). NUNCA use em produção.\n", SenhaContasTeste)
	for _, c := range r.Contas {
		estado := "atualizada"
		if c.Criada {
			estado = "criada"
		}
		fmt.Fprintf(w, "  %-34s %-40s id %-6d %s\n", c.Email, c.Papel, c.ID, estado)
	}
	estado := "reaproveitado"
	if r.DesafioCriado {
		estado = "criado"
	}
	fmt.Fprintf(w, "Desafio aberto #%d (%s), evento em %s; equipe de %d vagas com %s indicado como responsável (1 vaga aberta).\n",
		r.IDDesafio, estado, r.DataEventoDoSeed.Format("02/01/2006 15:04"), InstrutoresVagasSeed, EmailSeedInstrutor)
	if r.SaldoCreditado {
		fmt.Fprintf(w, "Saldo inicial de R$ %d,%02d creditado a %s (ledger: ajuste).\n",
			SaldoInicialSeedCentavos/100, SaldoInicialSeedCentavos%100, EmailSeedAtleta)
	} else {
		fmt.Fprintf(w, "Saldo inicial de %s já existia (não creditado de novo).\n", EmailSeedAtleta)
	}
	fmt.Fprintf(w, "Desafio avançado #%d (nível 5+, 18+, 5 premiados). Histórico p/ gráficos: %d desafio(s) e %d conta(s) com treinos criados agora.\n",
		r.IDDesafioAvancado, r.HistoricoCriado, r.TreinosCriados)
	fmt.Fprintln(w, "Painel admin: /admin/login. Auditoria: cli_seed.")
}

// cpfFicticio completa os 2 dígitos verificadores de uma base de 9 dígitos (formato 000.000.000-00).
func cpfFicticio(base string) string {
	d := digitos(base)
	for j := 9; j < 11; j++ {
		soma := 0
		for i := 0; i < j; i++ {
			soma += d[i] * (j + 1 - i)
		}
		r := (soma * 10) % 11
		if r == 10 {
			r = 0
		}
		d = append(d, r)
	}
	return fmt.Sprintf("%d%d%d.%d%d%d.%d%d%d-%d%d", d[0], d[1], d[2], d[3], d[4], d[5], d[6], d[7], d[8], d[9], d[10])
}

// cnpjFicticio completa os 2 dígitos verificadores de uma base de 12 dígitos (formato 00.000.000/0000-00).
func cnpjFicticio(base string) string {
	d := digitos(base)
	pesos := []int{6, 5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	for j := 12; j < 14; j++ {
		soma := 0
		for i := 0; i < j; i++ {
			soma += d[i] * pesos[i+13-j]
		}
		r := soma % 11
		if r < 2 {
			r = 0
		} else {
			r = 11 - r
		}
		d = append(d, r)
	}
	return fmt.Sprintf("%d%d.%d%d%d.%d%d%d/%d%d%d%d-%d%d", d[0], d[1], d[2], d[3], d[4], d[5], d[6], d[7], d[8],
		d[9], d[10], d[11], d[12], d[13])
}

func digitos(s string) []int {
	d := make([]int, 0, len(s)+2)
	for _, r := range s {
		d = append(d, int(r-'0'))
	}
	return d
}
