package services

import (
	"fmt"
	"regexp"
	"strings"

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
	CriarUsuario(req *models.CriarUsuarioRequest) (*models.UsuarioResponse, error)
	Login(req *models.LoginRequest, jwtSecret string) (*models.LoginResponse, error)
	// Perfil devolve o perfil COMPLETO — só deve ser chamado com o ID do JWT.
	Perfil(id uint) (*models.UsuarioResponse, error)
	// Publico devolve apenas id/nome/sobrenome de qualquer usuário.
	Publico(id uint) (*models.UsuarioPublico, error)
	Buscar(userID uint, termo string) ([]models.UsuarioPublico, error)
	// AlterarSenha troca a senha, revoga os tokens antigos e devolve um token NOVO para a sessão atual.
	AlterarSenha(userID uint, versaoToken int, req *models.AlterarSenhaRequest, jwtSecret string) (string, error)
	// Logout revoga o token informado (versão `versao` do usuário). Idempotente.
	Logout(userID uint, versao int) error
	AtualizarPerfil(userID uint, req *models.AtualizarPerfilRequest) (*models.UsuarioResponse, error)
}

// usuarioService é a implementação concreta da camada de serviço
type usuarioService struct {
	repo repositories.UsuarioRepository
}

// NovoUsuarioService cria uma nova instância do serviço de usuários
func NovoUsuarioService(repo repositories.UsuarioRepository) UsuarioService {
	return &usuarioService{repo: repo}
}

// CriarUsuario valida os dados e persiste um novo usuário no banco
func (s *usuarioService) CriarUsuario(req *models.CriarUsuarioRequest) (*models.UsuarioResponse, error) {
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

	usuario := &models.Usuario{
		Nome:           req.Nome,
		Sobrenome:      req.Sobrenome,
		Email:          req.Email,
		CPF:            req.CPF,
		Senha:          string(hashSenha),
		DataNascimento: req.DataNascimento,
		Genero:         req.Genero,
	}
	if err := s.repo.Criar(usuario); err != nil {
		if repositories.EhDuplicata(err) { // corrida entre duas requisições
			return nil, erroConflito("e-mail ou CPF já cadastrado")
		}
		return nil, fmt.Errorf("erro ao criar usuário: %w", err)
	}
	return toUsuarioResponse(usuario), nil
}

// Login autentica o usuário e retorna um token JWT
func (s *usuarioService) Login(req *models.LoginRequest, jwtSecret string) (*models.LoginResponse, error) {
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

	token, err := utils.GerarToken(usuario.ID, usuario.Email, usuario.TokenVersao, jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("erro ao gerar token: %w", err)
	}
	return &models.LoginResponse{Token: token, Usuario: *toUsuarioResponse(usuario)}, nil
}

// Perfil retorna o perfil completo do usuário.
func (s *usuarioService) Perfil(id uint) (*models.UsuarioResponse, error) {
	usuario, err := s.repo.BuscarPorID(id)
	if err != nil {
		return nil, err
	}
	if usuario == nil {
		return nil, erroNaoEncontrado("Usuário não encontrado")
	}
	return toUsuarioResponse(usuario), nil
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
func (s *usuarioService) AlterarSenha(userID uint, versaoToken int, req *models.AlterarSenhaRequest, jwtSecret string) (string, error) {
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
	return utils.GerarToken(userID, usuario.Email, versao, jwtSecret)
}

// Logout incrementa a versão do token (só se ainda for a do token apresentado, para que
// um token antigo/roubado não consiga derrubar a sessão atual do dono).
func (s *usuarioService) Logout(userID uint, versao int) error {
	_, err := s.repo.RevogarTokens(userID, versao)
	return err
}

// AtualizarPerfil atualiza nome, sobrenome e gênero do usuário autenticado
func (s *usuarioService) AtualizarPerfil(userID uint, req *models.AtualizarPerfilRequest) (*models.UsuarioResponse, error) {
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
	if err := s.repo.AtualizarPerfil(userID, campos); err != nil {
		return nil, fmt.Errorf("erro ao atualizar perfil: %w", err)
	}
	return s.Perfil(userID)
}

// toUsuarioResponse converte o model no perfil completo (somente para o dono)
func toUsuarioResponse(u *models.Usuario) *models.UsuarioResponse {
	nascimento := u.DataNascimento
	if len(nascimento) > 10 { // o driver devolve "1995-03-15T00:00:00Z" para colunas DATE
		nascimento = nascimento[:10]
	}
	return &models.UsuarioResponse{
		ID:             u.ID,
		Nome:           u.Nome,
		Sobrenome:      u.Sobrenome,
		Email:          u.Email,
		CPF:            u.CPF,
		DataNascimento: nascimento,
		Genero:         u.Genero,
		SaldoCentavos:  u.SaldoCentavos,
		Saldo:          utils.CentavosParaReais(u.SaldoCentavos),
		CriadoEm:       u.CriadoEm,
	}
}
