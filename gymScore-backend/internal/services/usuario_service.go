package services

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gynScore-backend/internal/models"
	"gynScore-backend/internal/repositories"
	"gynScore-backend/pkg/utils"
)

// UsuarioService define as operações de negócio para usuários
type UsuarioService interface {
	CriarUsuario(req *models.CriarUsuarioRequest) (*models.UsuarioResponse, error)
	Login(req *models.LoginRequest, jwtSecret string) (*models.LoginResponse, error)
	BuscarPorID(id uint) (*models.UsuarioResponse, error)
	Listar() ([]models.UsuarioResponse, error)
	AlterarSenha(userID uint, req *models.AlterarSenhaRequest) error
	RecuperarSenha(req *models.RecuperarSenhaRequest) error
	AtualizarPerfil(userID uint, req *models.AtualizarPerfilRequest) (*models.UsuarioResponse, error)
	BuscarPublico(id uint) (*models.UsuarioPublicoResponse, error)
	HistoricoDesafios(id uint, desafioRepo repositories.DesafioRepository) ([]models.Desafio, error)
	Buscar(q string) ([]models.UsuarioPublicoResponse, error)
	AtualizarUltimaVez(userID uint) error
	// Ingressos retorna as participações confirmadas do usuário com código de ingresso
	Ingressos(userID uint) ([]models.IngressoResponse, error)
}

// usuarioService é a implementação concreta da camada de serviço
type usuarioService struct {
	repo        repositories.UsuarioRepository
	desafioRepo repositories.DesafioRepository
}

// NovoUsuarioService cria uma nova instância do serviço de usuários
func NovoUsuarioService(repo repositories.UsuarioRepository, desafioRepo repositories.DesafioRepository) UsuarioService {
	return &usuarioService{repo: repo, desafioRepo: desafioRepo}
}

// CriarUsuario valida os dados e persiste um novo usuário no banco
func (s *usuarioService) CriarUsuario(req *models.CriarUsuarioRequest) (*models.UsuarioResponse, error) {
	// 1. Validação de e-mail via regex
	if !utils.ValidarEmail(req.Email) {
		return nil, errors.New("e-mail inválido")
	}

	// 2. Validação RIGOROSA de CPF conforme a request (14 caracteres: 000.000.000-00)
	if len(req.CPF) != 14 {
		return nil, errors.New("CPF deve estar no formato 000.000.000-00 (14 caracteres)")
	}

	// Limpar apenas para validar o algoritmo, mas salvar o original da request
	re := regexp.MustCompile(`[^0-9]`)
	cpfLimpo := re.ReplaceAllString(req.CPF, "")
	if len(cpfLimpo) != 11 {
		return nil, errors.New("CPF informado contém caracteres inválidos ou quantidade de dígitos incorreta")
	}
	
	if !utils.ValidarCPF(cpfLimpo) {
		return nil, errors.New("CPF informado é inválido")
	}

	// 3. Verificar duplicidade de e-mail e CPF (usando o valor exato da request)
	existenteEmail, _ := s.repo.BuscarPorEmail(req.Email)
	if existenteEmail != nil {
		return nil, errors.New("e-mail já cadastrado")
	}

	existenteCPF, _ := s.repo.BuscarPorCPF(req.CPF)
	if existenteCPF != nil {
		return nil, errors.New("CPF já cadastrado")
	}

	// 4. Hash da senha com bcrypt
	hashSenha, err := bcrypt.GenerateFromPassword([]byte(req.Senha), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("erro ao processar senha: %w", err)
	}

	// 5. Criar entidade e salvar exatamente como enviado
	usuario := &models.Usuario{
		Nome:           req.Nome,
		Sobrenome:      req.Sobrenome,
		Email:          req.Email,
		CPF:            strPtr(req.CPF), // Mantém o valor exato da request
		Senha:          string(hashSenha),
		DataNascimento: strPtr(req.DataNascimento),
		Genero:         strPtr(req.Genero),
		Saldo:          0.00,
	}

	if err := s.repo.Criar(usuario); err != nil {
		return nil, fmt.Errorf("erro ao criar usuário: %w", err)
	}

	return toUsuarioResponse(usuario), nil
}

// Login autentica o usuário e retorna um token JWT
func (s *usuarioService) Login(req *models.LoginRequest, jwtSecret string) (*models.LoginResponse, error) {
	usuario, err := s.repo.BuscarPorEmail(req.Email)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar usuário: %w", err)
	}
	if usuario == nil {
		return nil, errors.New("usuário ou senha inválidos")
	}

	// Comparar senha com o hash armazenado
	if err := bcrypt.CompareHashAndPassword([]byte(usuario.Senha), []byte(req.Senha)); err != nil {
		return nil, errors.New("usuário ou senha inválidos")
	}

	// Gerar token JWT
	token, err := utils.GerarToken(usuario.ID, usuario.Email, jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("erro ao gerar token: %w", err)
	}

	return &models.LoginResponse{
		Token:   token,
		Usuario: *toUsuarioResponse(usuario),
	}, nil
}

// BuscarPorID retorna os dados públicos de um usuário pelo ID
func (s *usuarioService) BuscarPorID(id uint) (*models.UsuarioResponse, error) {
	usuario, err := s.repo.BuscarPorID(id)
	if err != nil {
		return nil, fmt.Errorf("erro ao buscar usuário: %w", err)
	}
	if usuario == nil {
		return nil, nil
	}
	return toUsuarioResponse(usuario), nil
}

// Listar retorna todos os usuários cadastrados
func (s *usuarioService) Listar() ([]models.UsuarioResponse, error) {
	usuarios, err := s.repo.Listar()
	if err != nil {
		return nil, fmt.Errorf("erro ao listar usuários: %w", err)
	}

	var respostas []models.UsuarioResponse
	for _, u := range usuarios {
		respostas = append(respostas, *toUsuarioResponse(&u))
	}
	return respostas, nil
}

// AlterarSenha valida a senha atual e persiste a nova senha hasheada
func (s *usuarioService) AlterarSenha(userID uint, req *models.AlterarSenhaRequest) error {
	if len(req.NovaSenha) < 6 {
		return errors.New("nova senha deve ter pelo menos 6 caracteres")
	}

	usuario, err := s.repo.BuscarPorID(userID)
	if err != nil || usuario == nil {
		return errors.New("usuário não encontrado")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(usuario.Senha), []byte(req.SenhaAtual)); err != nil {
		return errors.New("senha atual incorreta")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NovaSenha), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("erro ao processar nova senha: %w", err)
	}

	usuario.Senha = string(hash)
	return s.repo.Atualizar(usuario)
}

// RecuperarSenha redefine a senha após validar e-mail + CPF (fluxo sem e-mail)
func (s *usuarioService) RecuperarSenha(req *models.RecuperarSenhaRequest) error {
	if len(req.NovaSenha) < 6 {
		return errors.New("nova senha deve ter pelo menos 6 caracteres")
	}

	usuario, err := s.repo.BuscarPorEmail(req.Email)
	if err != nil || usuario == nil {
		return errors.New("e-mail não encontrado")
	}

	if derefStr(usuario.CPF) != req.CPF {
		return errors.New("CPF não corresponde ao e-mail informado")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NovaSenha), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("erro ao processar nova senha: %w", err)
	}

	usuario.Senha = string(hash)
	return s.repo.Atualizar(usuario)
}

// AtualizarPerfil atualiza nome, sobrenome e gênero do usuário autenticado
func (s *usuarioService) AtualizarPerfil(userID uint, req *models.AtualizarPerfilRequest) (*models.UsuarioResponse, error) {
	usuario, err := s.repo.BuscarPorID(userID)
	if err != nil || usuario == nil {
		return nil, errors.New("usuário não encontrado")
	}

	if req.Nome != "" {
		usuario.Nome = req.Nome
	}
	if req.Sobrenome != "" {
		usuario.Sobrenome = req.Sobrenome
	}
	if req.Genero != "" {
		usuario.Genero = strPtr(req.Genero)
	}

	if err := s.repo.Atualizar(usuario); err != nil {
		return nil, fmt.Errorf("erro ao atualizar perfil: %w", err)
	}

	return toUsuarioResponse(usuario), nil
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

// HistoricoDesafios retorna os desafios encerrados nos quais o usuário participou
func (s *usuarioService) HistoricoDesafios(id uint, desafioRepo repositories.DesafioRepository) ([]models.Desafio, error) {
	return desafioRepo.ListarHistoricoUsuario(id)
}

// Buscar pesquisa usuários por nome, sobrenome ou username (case-insensitive, até 20 resultados)
func (s *usuarioService) Buscar(q string) ([]models.UsuarioPublicoResponse, error) {
	usuarios, err := s.repo.Buscar(q)
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
	if err != nil || u == nil {
		return errors.New("usuário não encontrado")
	}
	now := time.Now()
	u.UltimaVez = &now
	return s.repo.Atualizar(u)
}

// Ingressos retorna os desafios em que o usuário é participante, com código de ingresso gerado
func (s *usuarioService) Ingressos(userID uint) ([]models.IngressoResponse, error) {
	desafios, err := s.desafioRepo.ListarParticipacoesUsuario(userID)
	if err != nil {
		return nil, err
	}
	var lista []models.IngressoResponse
	for _, d := range desafios {
		codigo := fmt.Sprintf("AC-%04X-%03d", d.ID, userID)
		var data time.Time
		if d.DataEncerramento != nil {
			data = *d.DataEncerramento
		}
		lista = append(lista, models.IngressoResponse{
			IDDesafio:      d.ID,
			Titulo:         d.Titulo,
			Data:           data,
			Local:          d.Local,
			Status:         string(d.Status),
			CodigoIngresso: codigo,
		})
	}
	return lista, nil
}

// toUsuarioResponse converte o model para o DTO de resposta autenticada
func toUsuarioResponse(u *models.Usuario) *models.UsuarioResponse {
	return &models.UsuarioResponse{
		ID:             u.ID,
		Nome:           u.Nome,
		Sobrenome:      u.Sobrenome,
		Email:          u.Email,
		CPF:            u.CPF,
		DataNascimento: derefStr(u.DataNascimento),
		Genero:         derefStr(u.Genero),
		Saldo:          u.Saldo,
		Perfil:         u.Perfil,
		Username:       u.Username,
		Elo:            utils.CalcularElo(u.Pontos),
		Pontos:         u.Pontos,
		UltimaVez:      u.UltimaVez,
		CriadoEm:       u.CriadoEm,
	}
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

// derefStr retorna o valor apontado por p, ou "" se p for nil
func derefStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
