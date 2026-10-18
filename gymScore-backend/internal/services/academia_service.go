package services

import (
	"errors"
	"fmt"
	"regexp"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gynScore-backend/internal/models"
	"gynScore-backend/internal/repositories"
	"gynScore-backend/pkg/utils"
)

// AcademiaService define as operações de negócio para o cadastro de contas de academia
type AcademiaService interface {
	Criar(req *models.CriarAcademiaRequest) (*models.Academia, error)
}

// academiaService é a implementação concreta da camada de serviço
type academiaService struct {
	db          *gorm.DB
	repo        repositories.AcademiaRepository
	usuarioRepo repositories.UsuarioRepository
}

// NovoAcademiaService cria uma nova instância do serviço de cadastro de academias
func NovoAcademiaService(db *gorm.DB, repo repositories.AcademiaRepository, usuarioRepo repositories.UsuarioRepository) AcademiaService {
	return &academiaService{db: db, repo: repo, usuarioRepo: usuarioRepo}
}

var cnpjNaoNumericoRegex = regexp.MustCompile(`[^0-9]`)

// Criar valida os dados e persiste, em uma única transação, o usuário (perfil "academia",
// sem CPF) e o registro de academia vinculado a ele
func (s *academiaService) Criar(req *models.CriarAcademiaRequest) (*models.Academia, error) {
	// 1. Validação de e-mail
	if !utils.ValidarEmail(req.Email) {
		return nil, errors.New("e-mail inválido")
	}

	// 2. Validação RIGOROSA de CNPJ (18 caracteres: 00.000.000/0000-00)
	if len(req.CNPJ) != 18 {
		return nil, errors.New("CNPJ deve estar no formato 00.000.000/0000-00 (18 caracteres)")
	}
	cnpjLimpo := cnpjNaoNumericoRegex.ReplaceAllString(req.CNPJ, "")
	if len(cnpjLimpo) != 14 {
		return nil, errors.New("CNPJ informado contém caracteres inválidos ou quantidade de dígitos incorreta")
	}

	// 3. Verificar duplicidade de e-mail e CNPJ
	existenteEmail, _ := s.usuarioRepo.BuscarPorEmail(req.Email)
	if existenteEmail != nil {
		return nil, errors.New("e-mail já cadastrado")
	}
	existenteCNPJ, _ := s.repo.BuscarPorCNPJ(req.CNPJ)
	if existenteCNPJ != nil {
		return nil, errors.New("CNPJ já cadastrado")
	}

	// 4. Hash da senha com bcrypt
	hashSenha, err := bcrypt.GenerateFromPassword([]byte(req.Senha), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("erro ao processar senha: %w", err)
	}

	usuario := &models.Usuario{
		Nome:      req.Nome,
		Sobrenome: req.Sobrenome,
		Email:     req.Email,
		CPF:       nil, // Conta de academia é pessoa jurídica, sem CPF
		Senha:     string(hashSenha),
		Perfil:    "academia",
	}
	academia := &models.Academia{
		RazaoSocial:  req.RazaoSocial,
		NomeFantasia: req.NomeFantasia,
		CNPJ:         req.CNPJ,
		Endereco:     req.Endereco,
		Telefone:     req.Telefone,
	}

	// 5. Criar usuário + academia atomicamente
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(usuario).Error; err != nil {
			return fmt.Errorf("erro ao criar usuário: %w", err)
		}
		academia.IDUsuario = usuario.ID
		if err := s.repo.Criar(tx, academia); err != nil {
			return fmt.Errorf("erro ao criar academia: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return academia, nil
}
