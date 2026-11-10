package services

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/pkg/utils"

	"golang.org/x/crypto/bcrypt"
)

// hashInexistente é comparado quando o e-mail não existe, para o tempo de resposta do
// login não revelar se a conta existe.
var hashInexistente, _ = bcrypt.GenerateFromPassword([]byte("gymscore-sem-usuario"), bcrypt.DefaultCost)

var apenasDigitos = regexp.MustCompile(`[^0-9]`)

// UsuarioService define as operações de negócio para usuários
type UsuarioService interface {
	CriarUsuario(req *models.CriarUsuarioRequest) (*models.UsuarioSessaoResponse, error)
	// Login autentica e entra no modo modoPreferido(id) (último modo usado neste navegador; nil = nenhum)
	// se a conta ainda puder usá-lo; senão no modo atleta.
	Login(req *models.LoginRequest, jwtSecret string, modoPreferido func(idUsuario uint) string) (*models.LoginResponse, error)
	// Perfil devolve o perfil COMPLETO — só deve ser chamado com o ID do JWT; `modo` é o claim do token.
	Perfil(id uint, modo string) (*models.UsuarioResponse, error)
	// TrocarModo emite um JWT novo (mesma token_versao) com o claim `modo`, depois de conferir no banco
	// que a conta pode usar o modo (instrutor só com o pedido aprovado).
	TrocarModo(userID uint, versaoToken int, modo, jwtSecret string) (*models.LoginResponse, error)
	// Publico devolve apenas id/nome/sobrenome de qualquer usuário.
	Publico(id uint) (*models.UsuarioPublico, error)
	Buscar(userID uint, termo string) ([]models.UsuarioPublico, error)
	// AlterarSenha troca a senha, revoga os tokens antigos e devolve um token NOVO para a sessão atual.
	// O token novo mantém o modo da sessão (`modo`).
	AlterarSenha(userID uint, versaoToken int, modo string, req *models.AlterarSenhaRequest, jwtSecret string) (string, error)
	// Logout revoga o token informado (versão `versao` do usuário). Idempotente.
	Logout(userID uint, versao int) error
	AtualizarPerfil(userID uint, modo string, req *models.AtualizarPerfilRequest) (*models.UsuarioResponse, error)
	BuscarPublico(id uint) (*models.UsuarioPublicoResponse, error)
	HistoricoDesafios(id uint, desafioRepo *repositories.DesafioRepository) ([]models.HistoricoDesafioResponse, error)
	// BuscarPorTermo é a busca geral por nome/sobrenome/username (sem excluir o próprio usuário).
	BuscarPorTermo(q string) ([]models.UsuarioPublicoResponse, error)
	AtualizarUltimaVez(userID uint) error
	// Ingressos retorna as participações confirmadas do usuário com código de ingresso
	Ingressos(userID uint) ([]models.IngressoResponse, error)
}

// ConsultaBanimento informa a punição de banimento vigente para o 403 `conta_banida` do login (Q1).
type ConsultaBanimento interface {
	Banimento(idUsuario uint) (*models.BanimentoInfo, error)
}

// usuarioService é a implementação concreta da camada de serviço
type usuarioService struct {
	repo        repositories.UsuarioRepository
	desafioRepo *repositories.DesafioRepository
	banimentos  ConsultaBanimento // nil = login do banido responde só o código, sem detalhes
}

// NovoUsuarioService cria uma nova instância do serviço de usuários
func NovoUsuarioService(repo repositories.UsuarioRepository, desafioRepo *repositories.DesafioRepository) UsuarioService {
	return &usuarioService{repo: repo, desafioRepo: desafioRepo}
}

// NovoUsuarioServiceComModeracao é NovoUsuarioService com a consulta do banimento (detalhes no 403).
func NovoUsuarioServiceComModeracao(repo repositories.UsuarioRepository, desafioRepo *repositories.DesafioRepository,
	banimentos ConsultaBanimento) UsuarioService {
	return &usuarioService{repo: repo, desafioRepo: desafioRepo, banimentos: banimentos}
}

// CriarUsuario valida os dados e persiste um novo usuário no banco
func (s *usuarioService) CriarUsuario(req *models.CriarUsuarioRequest) (*models.UsuarioSessaoResponse, error) {
	req.Email = strings.TrimSpace(req.Email)
	if !utils.ValidarEmail(req.Email) {
		return nil, erroRequisicao("e-mail inválido")
	}

	// CPF no formato 000.000.000-00 (14 caracteres), com dígitos verificadores válidos
	if len(req.CPF) != 14 {
		return nil, erroRequisicao("CPF deve estar no formato 000.000.000-00 (14 caracteres)")
	}
	if len(apenasDigitos.ReplaceAllString(req.CPF, "")) != 11 {
		return nil, erroRequisicao("CPF informado contém caracteres inválidos ou quantidade de dígitos incorreta")
	}
	if !utils.ValidarCPF(req.CPF) {
		return nil, erroRequisicao("CPF informado é inválido")
	}

	if existente, err := s.repo.BuscarPorEmail(req.Email); err != nil {
		return nil, err
	} else if existente != nil {
		return nil, erroConflito("e-mail já cadastrado")
	}
	if existente, err := s.repo.BuscarPorCPF(req.CPF); err != nil {
		return nil, err
	} else if existente != nil {
		return nil, erroConflito("CPF já cadastrado")
	}

	hashSenha, err := bcrypt.GenerateFromPassword([]byte(req.Senha), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("erro ao processar senha: %w", err)
	}

	// req.DataNascimento já foi validado como "YYYY-MM-DD" (validate:"datetime=2006-01-02"),
	// mas tratamos o erro de novo aqui em vez de confiar só no middleware de validação.
	// ParseInLocation (não Parse): a conexão com o banco usa o fuso local (config.AplicarFusoDSN);
	// um time.Time em UTC meia-noite grava um dia ANTES na coluna DATE em fusos negativos
	// (ex.: 1995-03-15T00:00:00Z vira 1995-03-14 num fuso -03:00) — mesma lição de parseDataEncerramento.
	nascimento, err := time.ParseInLocation("2006-01-02", req.DataNascimento, time.Local)
	if err != nil {
		return nil, erroRequisicao("data_nascimento inválida (use YYYY-MM-DD)")
	}

	usuario := &models.Usuario{
		Nome:           req.Nome,
		Sobrenome:      req.Sobrenome,
		Email:          req.Email,
		CPF:            strPtr(req.CPF), // Mantém o valor exato da request
		Senha:          string(hashSenha),
		DataNascimento: &nascimento,
		Genero:         strPtr(req.Genero),
		Perfil:         models.PerfilAtleta,
	}
	if err := s.repo.Criar(usuario); err != nil {
		if repositories.EhDuplicata(err) { // corrida entre duas requisições
			return nil, erroConflito("e-mail ou CPF já cadastrado")
		}
		return nil, fmt.Errorf("erro ao criar usuário: %w", err)
	}
	return toUsuarioSessao(usuario), nil
}

// Login autentica o usuário e retorna um token JWT
func (s *usuarioService) Login(req *models.LoginRequest, jwtSecret string, modoPreferido func(idUsuario uint) string) (*models.LoginResponse, error) {
	credenciaisInvalidas := erro(401, "usuário ou senha inválidos")

	usuario, err := s.repo.BuscarPorEmail(strings.TrimSpace(req.Email))
	if err != nil {
		return nil, err
	}
	if usuario == nil {
		_ = bcrypt.CompareHashAndPassword(hashInexistente, []byte(req.Senha))
		return nil, credenciaisInvalidas
	}
	if err := bcrypt.CompareHashAndPassword([]byte(usuario.Senha), []byte(req.Senha)); err != nil {
		return nil, credenciaisInvalidas
	}
	// Só depois da senha correta: não revela a quem não sabe a senha que a conta existe/está bloqueada.
	// Banido: 403 conta_banida com o motivo e o caminho do recurso (Q1); nenhuma sessão é emitida.
	if usuario.Banido() {
		var info *models.BanimentoInfo
		if s.banimentos != nil {
			if info, err = s.banimentos.Banimento(usuario.ID); err != nil {
				return nil, err
			}
		}
		return nil, ErroContaBanida(info)
	}

	papeis, err := s.papeis(usuario)
	if err != nil {
		return nil, err
	}
	// Último modo usado (cookie de preferência) só vale se a conta AINDA pode usá-lo; senão atleta.
	modo := papeis.ModoEfetivo("")
	if modo != "" && modoPreferido != nil {
		if pref := modoPreferido(usuario.ID); pref != "" && papeis.PodeUsarModo(pref) {
			modo = pref
		}
	}
	token, err := utils.GerarTokenModo(usuario.ID, usuario.Email, usuario.TokenVersao, modo, jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("erro ao gerar token: %w", err)
	}
	sessao := toUsuarioSessao(usuario)
	sessao.SessaoModos = papeis.Sessao(modo)
	sessao.Moderacao = usuario.ModeracaoSessao(time.Now())
	return &models.LoginResponse{Token: token, Modo: modo, Usuario: *sessao}, nil
}

// TrocarModo troca o modo ativo da sessão (atleta ↔ instrutor) sem deslogar. O modo pedido é conferido
// no banco (nunca confia no corpo): instrutor exige o pedido de instrutor APROVADO. A token_versao não
// muda (as outras sessões continuam válidas, cada uma no seu modo). Punições valem nos dois modos: o
// banido nem chega aqui (AuthMiddleware) e a suspensão continua no token novo.
func (s *usuarioService) TrocarModo(userID uint, versaoToken int, modo, jwtSecret string) (*models.LoginResponse, error) {
	if !models.ModoValido(modo) {
		return nil, erroRequisicao("modo inválido (use atleta ou instrutor)")
	}
	usuario, err := s.repo.BuscarPorID(userID)
	if err != nil {
		return nil, err
	}
	if usuario == nil {
		return nil, erro(401, "Usuário não encontrado")
	}
	if usuario.Banido() {
		return nil, ErroContaBanida(nil)
	}
	papeis, err := s.papeis(usuario)
	if err != nil {
		return nil, err
	}
	if papeis.Conta != models.PerfilAtleta {
		return nil, erroProibido("Contas de academia não têm modo atleta/instrutor")
	}
	if !papeis.PodeUsarModo(modo) {
		return nil, ErroModoIndisponivel(papeis)
	}
	token, err := utils.GerarTokenModo(usuario.ID, usuario.Email, versaoToken, modo, jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("erro ao gerar token: %w", err)
	}
	sessao := toUsuarioSessao(usuario)
	sessao.SessaoModos = papeis.Sessao(modo)
	sessao.Moderacao = usuario.ModeracaoSessao(time.Now())
	return &models.LoginResponse{Token: token, Modo: modo, Usuario: *sessao}, nil
}

// Perfil retorna o perfil completo do usuário, com os modos da sessão (`modo` = claim do token).
func (s *usuarioService) Perfil(id uint, modo string) (*models.UsuarioResponse, error) {
	usuario, err := s.repo.BuscarPorID(id)
	if err != nil {
		return nil, err
	}
	if usuario == nil {
		return nil, erroNaoEncontrado("Usuário não encontrado")
	}
	papeis, err := s.papeis(usuario)
	if err != nil {
		return nil, err
	}
	resp := toUsuarioResponse(usuario)
	resp.SessaoModos = papeis.Sessao(modo)
	return resp, nil
}

// papeis lê o tipo de conta e o pedido de instrutor (uma consulta).
func (s *usuarioService) papeis(u *models.Usuario) (*models.PapeisUsuario, error) {
	p, err := s.repo.PapeisDoUsuario(u.ID)
	if err != nil {
		return nil, err
	}
	if p == nil { // usuário apagado entre as consultas: só o tipo de conta
		p = &models.PapeisUsuario{IDUsuario: u.ID, Conta: u.Perfil}
	}
	return p, nil
}

// Publico retorna os dados públicos de um usuário.
func (s *usuarioService) Publico(id uint) (*models.UsuarioPublico, error) {
	usuario, err := s.repo.BuscarPorID(id)
	if err != nil {
		return nil, err
	}
	if usuario == nil {
		return nil, erroNaoEncontrado("Usuário não encontrado")
	}
	pub := usuario.Publico()
	return &pub, nil
}

// Buscar procura usuários por nome (mín. 2 caracteres, máx. 20 resultados), sem o próprio usuário.
func (s *usuarioService) Buscar(userID uint, termo string) ([]models.UsuarioPublico, error) {
	termo = strings.TrimSpace(termo)
	if len([]rune(termo)) < 2 {
		return nil, erroRequisicao("Informe pelo menos 2 caracteres para buscar")
	}
	usuarios, err := s.repo.Buscar(termo, userID, 20)
	if err != nil {
		return nil, err
	}
	out := make([]models.UsuarioPublico, 0, len(usuarios))
	for i := range usuarios {
		out = append(out, usuarios[i].Publico())
	}
	return out, nil
}

// AlterarSenha valida a senha atual, persiste a nova (revogando tokens antigos) e emite um novo token
func (s *usuarioService) AlterarSenha(userID uint, versaoToken int, modo string, req *models.AlterarSenhaRequest, jwtSecret string) (string, error) {
	if len(req.NovaSenha) < 8 {
		return "", erroRequisicao("nova senha deve ter pelo menos 8 caracteres")
	}
	usuario, err := s.repo.BuscarPorID(userID)
	if err != nil {
		return "", err
	}
	if usuario == nil {
		return "", erroNaoEncontrado("Usuário não encontrado")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(usuario.Senha), []byte(req.SenhaAtual)); err != nil {
		return "", erroRequisicao("senha atual incorreta")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NovaSenha), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("erro ao processar nova senha: %w", err)
	}
	// UPDATE condicionado à versão do token usado: duas trocas simultâneas com o mesmo token
	// não passam ambas (a perdedora vê RowsAffected == 0 e recebe 401).
	versao, ok, err := s.repo.AtualizarSenha(userID, string(hash), versaoToken)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", erro(401, "Sessão encerrada. Faça login novamente")
	}
	return utils.GerarTokenModo(userID, usuario.Email, versao, modo, jwtSecret)
}

// Logout incrementa a versão do token (só se ainda for a do token apresentado, para que
// um token antigo/roubado não consiga derrubar a sessão atual do dono).
func (s *usuarioService) Logout(userID uint, versao int) error {
	_, err := s.repo.RevogarTokens(userID, versao)
	return err
}

// AtualizarPerfil atualiza nome, sobrenome e gênero do usuário autenticado
func (s *usuarioService) AtualizarPerfil(userID uint, modo string, req *models.AtualizarPerfilRequest) (*models.UsuarioResponse, error) {
	campos := map[string]any{}
	if req.Nome != "" {
		campos["nome"] = req.Nome
	}
	if req.Sobrenome != "" {
		campos["sobrenome"] = req.Sobrenome
	}
	if req.Genero != "" {
		campos["genero"] = req.Genero
	}
	if req.DataNascimento != "" {
		nasc, err := time.ParseInLocation("2006-01-02", req.DataNascimento, time.Local)
		if err != nil {
			return nil, erroRequisicao("data_nascimento inválida (use AAAA-MM-DD)")
		}
		if nasc.After(time.Now()) || time.Now().Year()-nasc.Year() > models.IdadeFiltroMax {
			return nil, erroRequisicao("data_nascimento fora do intervalo aceito")
		}
		u, err := s.repo.BuscarPorID(userID)
		if err != nil {
			return nil, err
		}
		if u == nil {
			return nil, erroNaoEncontrado("Usuário não encontrado")
		}
		if u.DataNascimento == nil {
			// UPDATE condicional (WHERE data_nascimento IS NULL): de N PATCHes simultâneos só um grava.
			ok, err := s.repo.DefinirDataNascimento(userID, nasc)
			if err != nil {
				return nil, fmt.Errorf("erro ao atualizar perfil: %w", err)
			}
			if !ok {
				// Outra requisição gravou antes: só aceita se for a mesma data (idempotente).
				if u, err = s.repo.BuscarPorID(userID); err != nil {
					return nil, err
				}
				if u == nil {
					return nil, erroNaoEncontrado("Usuário não encontrado")
				}
				if formatarData(u.DataNascimento) != req.DataNascimento {
					return nil, erroConflito("A data de nascimento já foi informada e não pode ser alterada")
				}
			}
		} else if formatarData(u.DataNascimento) != req.DataNascimento {
			return nil, erroConflito("A data de nascimento já foi informada e não pode ser alterada")
		}
	}
	if err := s.repo.AtualizarPerfil(userID, campos); err != nil {
		return nil, fmt.Errorf("erro ao atualizar perfil: %w", err)
	}
	return s.Perfil(userID, modo)
}

// BuscarPublico retorna os dados públicos de um usuário pelo ID
func (s *usuarioService) BuscarPublico(id uint) (*models.UsuarioPublicoResponse, error) {
	u, err := s.repo.BuscarPorID(id)
	if err != nil {
		return nil, err
	}
	if u == nil {
		return nil, nil
	}
	return toUsuarioPublicoResponse(u), nil
}

// HistoricoDesafios retorna os desafios encerrados em que o usuário esteve inscrito, com o resultado
// DELE (posição no pódio, marca, prêmio) — nunca o dos demais participantes.
func (s *usuarioService) HistoricoDesafios(id uint, desafioRepo *repositories.DesafioRepository) ([]models.HistoricoDesafioResponse, error) {
	linhas, err := desafioRepo.ListarHistorico(id)
	if err != nil {
		return nil, err
	}
	resp := make([]models.HistoricoDesafioResponse, 0, len(linhas))
	for _, l := range linhas {
		item := models.HistoricoDesafioResponse{
			ID: l.IDDesafio, Titulo: l.Titulo, Categoria: l.Categoria, Status: l.Status, Local: derefStr(l.Local),
			DataEvento: l.DataEvento, DataEncerramento: l.DataEncerramento,
			ValorEntradaCentavos: l.ValorEntradaCentavos, ValorEntrada: utils.CentavosParaReais(l.ValorEntradaCentavos),
			ParticipantesCount: l.Participantes, Presente: l.Presente, Posicao: l.Posicao, Marca: derefStr(l.Marca),
			Vencedor: l.Vencedor, PremioCentavos: l.PremioCentavos, Premio: utils.CentavosParaReais(l.PremioCentavos),
			CriadoEm: l.CriadoEm,
		}
		if l.IDAcademia != nil && l.NomeFantasia != nil {
			item.Academia = &models.AcademiaResumo{ID: *l.IDAcademia, NomeFantasia: *l.NomeFantasia}
		}
		resp = append(resp, item)
	}
	return resp, nil
}

// BuscarPorTermo pesquisa usuários por nome, sobrenome ou username (case-insensitive, até 20 resultados)
func (s *usuarioService) BuscarPorTermo(q string) ([]models.UsuarioPublicoResponse, error) {
	usuarios, err := s.repo.BuscarPorTermo(q)
	if err != nil {
		return nil, err
	}
	var resp []models.UsuarioPublicoResponse
	for _, u := range usuarios {
		resp = append(resp, *toUsuarioPublicoResponse(&u))
	}
	return resp, nil
}

// AtualizarUltimaVez registra o timestamp atual como última vez online do usuário
func (s *usuarioService) AtualizarUltimaVez(userID uint) error {
	u, err := s.repo.BuscarPorID(userID)
	if err != nil {
		return err
	}
	if u == nil {
		return erroNaoEncontrado("Usuário não encontrado")
	}
	return s.repo.AtualizarUltimaVez(userID, time.Now())
}

// Ingressos retorna as inscrições do usuário como ingressos: o código REAL do QR (o mesmo
// `seu_codigo_ingresso` do desafio, aceito em POST /desafios/:id/presenca), os dados do evento e a presença.
func (s *usuarioService) Ingressos(userID uint) ([]models.IngressoResponse, error) {
	linhas, err := s.desafioRepo.ListarIngressos(userID)
	if err != nil {
		return nil, err
	}
	lista := make([]models.IngressoResponse, 0, len(linhas))
	for _, l := range linhas {
		ing := models.IngressoResponse{
			IDDesafio: l.IDDesafio, Titulo: l.Titulo, Categoria: l.Categoria, Data: l.DataEvento,
			Local: derefStr(l.Local), Status: string(l.Status), CodigoIngresso: l.CodigoIngresso,
			ValorEntradaCentavos: l.ValorEntradaCentavos, Presente: l.Presente, PresenteEm: l.PresenteEm,
		}
		if l.IDAcademia != nil && l.NomeFantasia != nil {
			ing.Academia = &models.AcademiaResumo{ID: *l.IDAcademia, NomeFantasia: *l.NomeFantasia}
		}
		lista = append(lista, ing)
	}
	return lista, nil
}

// toUsuarioResponse converte o model no perfil completo (somente para o dono)
func toUsuarioResponse(u *models.Usuario) *models.UsuarioResponse {
	return &models.UsuarioResponse{
		ID:             u.ID,
		Nome:           u.Nome,
		Sobrenome:      u.Sobrenome,
		Email:          u.Email,
		CPF:            u.CPF,
		DataNascimento: formatarData(u.DataNascimento),
		Genero:         derefStr(u.Genero),
		SaldoCentavos:  u.SaldoCentavos,
		Saldo:          utils.CentavosParaReais(u.SaldoCentavos),
		SessaoModos:    models.SessaoModos{Perfil: u.Perfil, ModosDisponiveis: []string{}},
		Username:       u.Username,
		Elo:            utils.CalcularElo(u.Pontos),
		Pontos:         u.Pontos,
		UltimaVez:      u.UltimaVez,
		CriadoEm:       u.CriadoEm,
		Moderacao:      u.ModeracaoSessao(time.Now()),
	}
}

// toUsuarioSessao é o resumo sem CPF e saldo (login e cadastro).
func toUsuarioSessao(u *models.Usuario) *models.UsuarioSessaoResponse {
	return &models.UsuarioSessaoResponse{ID: u.ID, Nome: u.Nome, Sobrenome: u.Sobrenome, Email: u.Email,
		SessaoModos: sessaoBasica(u), Username: u.Username, Elo: utils.CalcularElo(u.Pontos), Pontos: u.Pontos}
}

// sessaoBasica é o bloco de modos de uma conta recém-criada (sem pedido de instrutor).
func sessaoBasica(u *models.Usuario) models.SessaoModos {
	p := models.PapeisUsuario{IDUsuario: u.ID, Conta: u.Perfil}
	return p.Sessao("")
}

// toUsuarioPublicoResponse converte o model para o DTO de perfil público (sem CPF, email, saldo)
func toUsuarioPublicoResponse(u *models.Usuario) *models.UsuarioPublicoResponse {
	return &models.UsuarioPublicoResponse{
		ID:        u.ID,
		Nome:      u.Nome,
		Sobrenome: u.Sobrenome,
		Username:  u.Username,
		Elo:       utils.CalcularElo(u.Pontos),
		Pontos:    u.Pontos,
		UltimaVez: u.UltimaVez,
		Perfil:    u.Perfil,
	}
}

// strPtr retorna um ponteiro para a string informada (usado para o campo CPF, nulo em contas "academia")
func strPtr(s string) *string {
	return &s
}

// formatarData devolve a data no formato "2006-01-02" (sem hora/fuso), ou "" se t for nil
// (contas de perfil "academia" não têm data de nascimento).
func formatarData(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

// derefStr retorna o valor apontado por p, ou "" se p for nil
func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
