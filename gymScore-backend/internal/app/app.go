// Package app monta o servidor Fiber (injeção de dependências + middlewares + rotas).
// Fica separado do main para que os testes de integração subam a mesma aplicação.
package app

import (
	"log"
	"regexp"

	"gymScore-backend/internal/client"
	"gymScore-backend/internal/config"
	"gymScore-backend/internal/controllers"
	"gymScore-backend/internal/jobs"
	"gymScore-backend/internal/middlewares"
	"gymScore-backend/internal/repositories"
	"gymScore-backend/internal/routes"
	"gymScore-backend/internal/services"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/swagger"

	"gorm.io/gorm"
	_ "gymScore-backend/docs" // registra a especificação do Swagger
)

// Dependencias são as integrações externas substituíveis (testes usam fakes).
type Dependencias struct {
	Asaas  services.AsaasGateway
	Mailer services.Mailer
}

// limiteJob é quantos itens cada passo dos jobs trata por rodada.
const limiteJob = 100

// limiteCorpoPadrao é o limite de corpo de toda rota (JSON); só o upload de evidência tem limite próprio.
const limiteCorpoPadrao = 1 * 1024 * 1024

// folgaMultipart cobre os cabeçalhos/bordas do multipart além da imagem.
const folgaMultipart = 64 * 1024

// rotaEvidencia é a única rota com corpo acima de limiteCorpoPadrao (POST multipart, 1 imagem).
var rotaEvidencia = regexp.MustCompile(`^/api/denuncias/[0-9]+/evidencias/?$`)

// Construir injeta as dependências e devolve o app Fiber pronto para Listen/Test.
func Construir(cfg *config.Config, db *gorm.DB, dep Dependencias) *fiber.App {
	app, _ := ConstruirComJobs(cfg, db, dep)
	return app
}

// ConstruirComJobs é Construir mais as tarefas periódicas (expiração de desafios e reconciliação
// PIX), que o main agenda com jobs.Iniciar e para no shutdown.
func ConstruirComJobs(cfg *config.Config, db *gorm.DB, dep Dependencias) (*fiber.App, []jobs.Tarefa) {
	// Repositories
	usuarioRepo := repositories.NovoUsuarioRepository(db)
	desafioRepo := repositories.NovoDesafioRepository(db)
	amizadeRepo := repositories.NovoAmizadeRepository(db)
	transacaoRepo := repositories.NovoTransacaoRepository(db)
	carteiraRepo := repositories.NovoCarteiraRepository(db)
	treinoRepo := repositories.NovoTreinoRepository(db)
	resetRepo := repositories.NovoResetSenhaRepository(db)
	notifRepo := repositories.NovoNotificacaoRepository(db)
	rankingRepo := repositories.NovoRankingRepository(db)
	instrutorRepo := repositories.NovoInstrutorRepository(db)
	avaliacaoRepo := repositories.NovoAvaliacaoRepository(db)
	academiaRepo := repositories.NovoAcademiaRepository(db)
	candidaturaRepo := repositories.NovoCandidaturaRepository(db)
	receitaRepo := repositories.NovoReceitaRepository(db)
	painelRepo := repositories.NovoPainelRepository(db)
	vinculoInstrutorRepo := repositories.NovoVinculoInstrutorRepository(db)
	adminRepo := repositories.NovoAdminRepository(db)
	auditoriaRepo := repositories.NovoAuditoriaRepository(db)
	moderacaoRepo := repositories.NovoModeracaoRepository(db)
	adminConsultaRepo := repositories.NovoAdminConsultaRepository(db)
	denunciaRepo := repositories.NovoDenunciaRepository(db)
	punicaoRepo := repositories.NovoPunicaoRepository(db)
	pendenteRepo := repositories.NovoPagamentoPendenteRepository(db)
	eventoRepo := repositories.NovoModeracaoEventoRepository(db)
	moderacaoConsultaRepo := repositories.NovoModeracaoConsultaRepository(db)

	// Cliente Anthropic (opcional): sem chave, as recomendações caem na heurística.
	var anthropicClient *client.AnthropicClient
	if cfg.IAHabilitada() {
		anthropicClient = client.NovoAnthropicClient(cfg.AnthropicAPIKey, cfg.AnthropicModel)
		log.Println("[Anthropic] cliente inicializado, modelo:", cfg.AnthropicModel)
	} else {
		log.Println("[Anthropic] ANTHROPIC_API_KEY não configurada — recomendações usarão heurística")
	}

	// Services
	notifSvc := services.NovoNotificacaoService(notifRepo)
	moderacaoSvc := services.NovoModeracaoService(db, denunciaRepo, punicaoRepo, pendenteRepo, eventoRepo, moderacaoConsultaRepo,
		usuarioRepo, academiaRepo, auditoriaRepo, notifSvc, services.ModeracaoConfig{
			PrazoAcademia: cfg.PrazoAcademiaDenuncia(), LimiteDia: cfg.LimiteDenunciasDia(),
			EvidenciaMaxBytes: cfg.EvidenciaMaxBytes(), EvidenciasDir: cfg.DirEvidencias(), PrazoRecurso: cfg.PrazoRecurso(),
		})
	usuarioSvc := services.NovoUsuarioServiceComModeracao(usuarioRepo, desafioRepo, moderacaoSvc)
	senhaSvc := services.NovoSenhaService(usuarioRepo, resetRepo, dep.Mailer, cfg.AppBaseURL)
	desafioSvc := services.NovoDesafioService(db, desafioRepo, carteiraRepo, amizadeRepo, usuarioRepo,
		academiaRepo, instrutorRepo, avaliacaoRepo, notifSvc, candidaturaRepo, receitaRepo, cfg.PlataformaTaxaInscricaoPct, cfg.JanelaContestacao())
	amizadeSvc := services.NovoAmizadeService(amizadeRepo, usuarioRepo, notifSvc)
	pixSvc := services.NovoPIXService(dep.Asaas, usuarioRepo, transacaoRepo, carteiraRepo, notifSvc)
	carteiraSvc := services.NovoCarteiraService(carteiraRepo, usuarioRepo, dep.Asaas, notifSvc, cfg.TarifaSaqueAtiva())
	rankingSvc := services.NovoRankingService(rankingRepo)
	treinoSvc := services.NovoTreinoService(treinoRepo, usuarioRepo)
	instrutorSvc := services.NovoInstrutorService(db, instrutorRepo, usuarioRepo, vinculoInstrutorRepo, desafioRepo, notifSvc)
	recomendacaoSvc := services.NovoRecomendacaoService(desafioRepo, treinoRepo, usuarioRepo, anthropicClient)
	academiaSvc := services.NovoAcademiaService(db, academiaRepo, usuarioRepo)
	academiaGestaoSvc := services.NovoAcademiaGestaoService(db, academiaRepo, desafioRepo, painelRepo, vinculoInstrutorRepo, notifSvc)

	adminSvc := services.NovoAdminService(db, adminRepo, auditoriaRepo, moderacaoRepo, adminConsultaRepo, carteiraRepo,
		desafioRepo, desafioSvc, dep.Asaas, notifSvc, services.AdminConfig{
			Segredo: cfg.SegredoAdmin(), TTL: cfg.TTLAdmin(), Carencia: cfg.DesafioCarencia,
		})

	tarefas := []jobs.Tarefa{
		{Nome: "expirar-desafios", Executar: func() error {
			n, err := desafioSvc.ExpirarVencidos(cfg.DesafioCarencia, limiteJob)
			if n > 0 {
				log.Printf("[JOBS] %d desafio(s) vencido(s) cancelado(s)", n)
			}
			return err
		}},
		{Nome: "reconciliar-depositos", Executar: func() error { _, err := pixSvc.ReconciliarDepositos(limiteJob); return err }},
		{Nome: "reconciliar-saques", Executar: func() error { _, err := carteiraSvc.ReconciliarSaques(limiteJob); return err }},
		{Nome: "escalonar-denuncias", Executar: func() error {
			n, err := moderacaoSvc.EscalonarDenuncias(limiteJob)
			if n > 0 {
				log.Printf("[JOBS] %d denúncia(s) escalonada(s) ao admin por prazo vencido", n)
			}
			return err
		}},
		{Nome: "fim-suspensao", Executar: func() error { _, err := moderacaoSvc.EncerrarSuspensoes(limiteJob); return err }},
		{Nome: "liberar-pagamentos", Executar: func() error {
			n, err := moderacaoSvc.LiberarPagamentos(limiteJob)
			if n > 0 {
				log.Printf("[JOBS] %d pagamento(s) do pódio liberado(s) após a janela de contestação", n)
			}
			return err
		}},
	}

	// BodyLimit do servidor = o maior limite (upload de evidência); LimiteCorpo devolve 1 MiB ao resto.
	limiteServidor := limiteCorpoPadrao
	if n := int(cfg.EvidenciaMaxBytes()) + folgaMultipart; n > limiteServidor {
		limiteServidor = n
	}
	app := fiber.New(fiber.Config{
		AppName:      "GymScore API v1.0.0",
		BodyLimit:    limiteServidor,
		ErrorHandler: ErrorHandler,
	})

	// Middlewares globais
	app.Use(middlewares.RecoverMiddleware(cfg.IsProduction()))
	app.Use(middlewares.LoggerMiddleware())
	if cors := middlewares.CORSMiddleware(cfg); cors != nil {
		app.Use(cors)
	}
	app.Use(middlewares.LimiteCorpo(limiteCorpoPadrao, func(c *fiber.Ctx) bool {
		return c.Method() == fiber.MethodPost && rotaEvidencia.MatchString(c.Path())
	}))

	routes.Setup(app, cfg, routes.Controllers{
		Usuario:      controllers.NovoUsuarioController(usuarioSvc, senhaSvc, cfg, desafioRepo),
		Desafio:      controllers.NovoDesafioController(desafioSvc),
		Amizade:      controllers.NovoAmizadeController(amizadeSvc),
		PIX:          controllers.NovoPIXController(pixSvc, cfg),
		Treino:       controllers.NovoTreinoController(treinoSvc),
		Webhook:      controllers.NovoWebhookController(pixSvc, cfg),
		Carteira:     controllers.NovoCarteiraController(carteiraSvc),
		Notificacao:  controllers.NovoNotificacaoController(notifSvc),
		Ranking:      controllers.NovoRankingController(rankingSvc),
		Instrutor:    controllers.NovoInstrutorController(instrutorSvc),
		Academia:     controllers.NovoAcademiaController(academiaSvc, academiaGestaoSvc),
		Recomendacao: controllers.NovoRecomendacaoController(recomendacaoSvc),
		Plataforma:   controllers.NovoPlataformaController(cfg),
		CEP:          controllers.NovoCEPController(services.NovoCEPService(cfg.ViaCEPURL)),
		Sessoes:      usuarioRepo,
		UsuarioRepo:  usuarioRepo,
		Admin:        controllers.NovoAdminController(adminSvc, cfg).ComModeracao(moderacaoSvc),
		AdminEstados: adminRepo,
		Moderacao:    controllers.NovoModeracaoController(moderacaoSvc, cfg.EvidenciaMaxBytes()),
	})
	// Swagger UI: só fora de produção (não expõe o mapa da API)
	if !cfg.IsProduction() {
		app.Get("/swagger/*", swagger.HandlerDefault)
	}
	return app, tarefas
}

// ErrorHandler é o handler global de erros do Fiber: 4xx do Fiber mantêm a mensagem
// (ex.: 404 de rota); qualquer outro erro vira 500 genérico.
func ErrorHandler(c *fiber.Ctx, err error) error {
	code := fiber.StatusInternalServerError
	message := "Erro interno do servidor"

	if e, ok := err.(*fiber.Error); ok {
		code = e.Code
		message = e.Message
	}

	return c.Status(code).JSON(fiber.Map{
		"success": false,
		"error":   message,
	})
}
