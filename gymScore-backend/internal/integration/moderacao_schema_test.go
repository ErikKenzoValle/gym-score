//go:build integration

package integration

// Rodada 3 (dev-db): migração sobre o estado do RDS de testes (v13 = moderação da Sprint 7 + parciais da
// antiga 00014), conversão do bloqueio em punição (00019) e os repositórios da moderação.

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"gymScore-backend/database"
	"gymScore-backend/internal/config"
	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func versaoGoose(t *testing.T, d *gorm.DB) int64 {
	t.Helper()
	var v int64
	d.Raw("SELECT MAX(version_id) FROM goose_db_version WHERE is_applied = 1").Scan(&v)
	return v
}

func existe(t *testing.T, d *gorm.DB, q string, args ...any) bool {
	t.Helper()
	var n int64
	if err := d.Raw(q, args...).Scan(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n > 0
}

// versaoFinal é a última migration (00020_modo_instrutor_endereco).
const versaoFinal = 20

func colunaExiste(t *testing.T, d *gorm.DB, tabela, coluna string) bool {
	return existe(t, d, `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE()
		AND table_name = ? AND column_name = ?`, tabela, coluna)
}

// Reproduz o RDS de testes: goose em 13 (moderação) + o que a antiga 00014 deixou ao falhar na FK para
// `administradores` (colunas sem FK, índices, procedure órfã). O app (database.Migrar) tem que subir.
func TestMigracao_EstadoParcialDoRDS(t *testing.T) {
	d, sqlDB := bancoQA(t, "rdsparcial", 12)
	L := semearLegado(t, d)
	if _, err := providerQA(t, sqlDB).UpTo(context.Background(), 13); err != nil {
		t.Fatalf("UpTo 13: %v", err)
	}
	for _, q := range []string{
		"ALTER TABLE usuarios ADD COLUMN bloqueado TINYINT(1) NOT NULL DEFAULT 0, ADD COLUMN bloqueado_motivo VARCHAR(500) NULL, ADD COLUMN bloqueado_em DATETIME(3) NULL",
		"ALTER TABLE academias ADD COLUMN status ENUM('pendente','aprovada','rejeitada') NOT NULL DEFAULT 'aprovada', ADD COLUMN motivo_rejeicao VARCHAR(500) NULL, ADD COLUMN aprovado_por BIGINT UNSIGNED NULL, ADD COLUMN aprovado_em DATETIME(3) NULL",
		"ALTER TABLE academias MODIFY COLUMN status ENUM('pendente','aprovada','rejeitada') NOT NULL DEFAULT 'pendente'",
		"ALTER TABLE academias ADD KEY idx_academias_status (status, criado_em), ADD KEY fk_academias_aprovado_por (aprovado_por)",
		"CREATE PROCEDURE _goose_00014_add() BEGIN END",
	} {
		execQA(t, d, q)
	}
	var nUsu, nTx int64
	d.Raw("SELECT COUNT(*) FROM usuarios").Scan(&nUsu)
	d.Raw("SELECT COUNT(*) FROM transacoes").Scan(&nTx)
	conferirInvariante(t, d, "antes")

	if err := database.Migrar(context.Background(), sqlDB); err != nil {
		t.Fatalf("database.Migrar sobre o estado do RDS: %v", err)
	}
	if v := versaoGoose(t, d); v != versaoFinal {
		t.Errorf("versão goose %d, esperada %d", v, versaoFinal)
	}
	if existe(t, d, "SELECT COUNT(*) FROM information_schema.routines WHERE routine_schema = DATABASE() AND routine_name LIKE '\\_goose\\_%'") {
		t.Errorf("sobrou procedure _goose_* temporária")
	}
	if !existe(t, d, `SELECT COUNT(*) FROM information_schema.referential_constraints WHERE constraint_schema = DATABASE()
		AND constraint_name = 'fk_academias_aprovado_por' AND referenced_table_name = 'administradores'`) {
		t.Errorf("fk_academias_aprovado_por não foi criada sobre o índice existente")
	}
	if colunaExiste(t, d, "usuarios", "bloqueado") {
		t.Errorf("usuarios.bloqueado deveria ter sido removida pela 00019")
	}
	var st string
	d.Raw("SELECT status FROM academias WHERE id_usuario = ?", L.academia).Scan(&st)
	if st != "aprovada" {
		t.Errorf("academia existente com status %q", st)
	}
	d.Raw("SELECT status FROM instrutores WHERE id_usuario = ?", L.instrutor).Scan(&st)
	if st != "aprovada" {
		t.Errorf("instrutor existente com status %q", st)
	}
	var nUsu2, nTx2 int64
	d.Raw("SELECT COUNT(*) FROM usuarios").Scan(&nUsu2)
	d.Raw("SELECT COUNT(*) FROM transacoes").Scan(&nTx2)
	if nUsu2 != nUsu || nTx2 < nTx {
		t.Errorf("contagens mudaram: usuarios %d->%d, transacoes %d->%d", nUsu, nUsu2, nTx, nTx2)
	}
	conferirInvariante(t, d, "depois")

	// Segunda subida (outra instância/boot): nada a fazer.
	if err := database.Migrar(context.Background(), sqlDB); err != nil {
		t.Fatalf("Migrar de novo: %v", err)
	}
}

// 00019: conta com bloqueado = 1 (bloqueio da rodada 2) vira punição de banimento com o admin da auditoria.
func TestMigracao_BloqueioViraBanimento(t *testing.T) {
	d, sqlDB := bancoQA(t, "bloqueio", 18)
	u := usuarioLegado(t, d, "atleta")
	execQA(t, d, "INSERT INTO administradores (nome, email, senha_hash, papel) VALUES ('Super', 's@x.test', 'h', 'superadmin'), ('Adm', 'a@x.test', 'h', 'admin')")
	var idAdm uint
	d.Raw("SELECT id FROM administradores WHERE email = 'a@x.test'").Scan(&idAdm)
	execQA(t, d, "INSERT INTO admin_auditoria (id_admin, acao, alvo_tipo, alvo_id, motivo) VALUES (?, 'usuario_bloquear', 'usuario', ?, 'golpe')", idAdm, u)
	execQA(t, d, "UPDATE usuarios SET bloqueado = 1, bloqueado_motivo = 'golpe no pix', bloqueado_em = '2026-11-01 10:00:00.000' WHERE id = ?", u)

	if _, err := providerQA(t, sqlDB).Up(context.Background()); err != nil {
		t.Fatalf("Up 19: %v", err)
	}
	var p models.Punicao
	if err := d.Where("id_usuario = ? AND tipo = 'banimento'", u).First(&p).Error; err != nil {
		t.Fatalf("punição não criada: %v", err)
	}
	if p.AplicadaPorAdmin == nil || *p.AplicadaPorAdmin != idAdm || p.AplicadaPor != nil || p.Justificativa != "golpe no pix" {
		t.Errorf("punição migrada: %+v", p)
	}
	var usu models.Usuario
	d.Select("id", "banido_em", "motivo_bloqueio").First(&usu, u)
	if !usu.Banido() || usu.MotivoBloqueio == nil || *usu.MotivoBloqueio != "golpe no pix" {
		t.Errorf("estado efetivo: %+v", usu)
	}
	// Down 19 é recusado (há ação de admin na moderação); sem ela, desce e sobe. (A 00020 desce antes.)
	if _, err := providerQA(t, sqlDB).DownTo(context.Background(), 18); err == nil || !strings.Contains(err.Error(), "nao reversivel") {
		t.Errorf("Down deveria recusar: %v", err)
	}
}

// Down 19..13 e Up de novo num banco limpo (schema volta e reaplica).
func TestMigracao_ModeracaoDownUp(t *testing.T) {
	d, sqlDB := bancoQA(t, "downup", 19)
	p := providerQA(t, sqlDB)
	ctx := context.Background()
	if _, err := p.DownTo(ctx, 12); err != nil {
		t.Fatalf("DownTo 12: %v", err)
	}
	if existe(t, d, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name IN ('denuncias','punicoes','administradores')") {
		t.Errorf("tabelas da moderação/admin sobraram após Down")
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("Up de novo: %v", err)
	}
	if versaoGoose(t, d) != versaoFinal || colunaExiste(t, d, "usuarios", "bloqueado") || !colunaExiste(t, d, "punicoes", "aplicada_por_admin") {
		t.Errorf("schema após Down+Up incorreto")
	}
}

// Clone do RDS (critério MD6): só roda com RDS_CLONE_DB = nome de um banco já restaurado do dump no
// mesmo servidor do TEST_DB_DSN. Ver PLANO.md (Relatório DEV_DB rodada 3) para o comando de restauração.
func TestMigracao_CloneDoRDS(t *testing.T) {
	nome := os.Getenv("RDS_CLONE_DB")
	if nome == "" {
		t.Skip("RDS_CLONE_DB não definido")
	}
	d, err := gorm.Open(mysql.Open(config.AplicarFusoDSN(strings.Replace(dsnAdmin, "/?", "/"+nome+"?", 1), time.Local)), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := d.DB()
	type contagens struct {
		Usuarios, Transacoes, Desafios, Participantes, SomaSaldos, SomaLedger int64
		// 00020: registros de instrutor, contas de academia e o texto do endereço ficam intactos.
		Instrutores, Academias, ContasAcademia, HashEndereco int64
	}
	contar := func() contagens {
		var c contagens
		d.Raw(`SELECT (SELECT COUNT(*) FROM usuarios) usuarios, (SELECT COUNT(*) FROM transacoes) transacoes,
			(SELECT COUNT(*) FROM instrutores) instrutores, (SELECT COUNT(*) FROM academias) academias,
			(SELECT COUNT(*) FROM usuarios WHERE perfil = 'academia') contas_academia,
			(SELECT COALESCE(SUM(CRC32(CONCAT(id, ':', COALESCE(endereco, '')))), 0) FROM academias) hash_endereco,
			(SELECT COUNT(*) FROM desafios) desafios, (SELECT COUNT(*) FROM desafio_participantes) participantes,
			(SELECT COALESCE(SUM(saldo_centavos),0) FROM usuarios) soma_saldos,
			(SELECT COALESCE(SUM(valor_centavos),0) FROM transacoes WHERE status IN ('received','concluido') OR (tipo='saque' AND status='pending')) soma_ledger`).Scan(&c)
		return c
	}
	antes := contar()
	t.Logf("antes: versão %d, %+v", versaoGoose(t, d), antes)
	conferirInvariante(t, d, "clone antes")
	if err := database.Migrar(context.Background(), sqlDB); err != nil {
		t.Fatalf("database.Migrar no clone do RDS: %v", err)
	}
	depois := contar()
	t.Logf("depois: versão %d, %+v", versaoGoose(t, d), depois)
	if versaoGoose(t, d) != versaoFinal {
		t.Errorf("versão final %d", versaoGoose(t, d))
	}
	// A 00017 pode estornar inscrição de conta academia em desafio ativo (gera linhas e muda saldos),
	// por isso transações/participantes só são comparados quando ela não teve o que fazer.
	if depois.Usuarios != antes.Usuarios || depois.Desafios != antes.Desafios || depois.Transacoes < antes.Transacoes {
		t.Errorf("dados mudaram: %+v -> %+v", antes, depois)
	}
	if depois.Transacoes == antes.Transacoes && (depois.SomaSaldos != antes.SomaSaldos || depois.Participantes != antes.Participantes) {
		t.Errorf("saldos/participações mudaram sem estorno: %+v -> %+v", antes, depois)
	}
	if depois.Instrutores != antes.Instrutores || depois.Academias != antes.Academias ||
		depois.ContasAcademia != antes.ContasAcademia || depois.HashEndereco != antes.HashEndereco {
		t.Errorf("00020 mexeu em instrutores/academias/endereço: %+v -> %+v", antes, depois)
	}
	if existe(t, d, `SELECT COUNT(*) FROM instrutores i JOIN usuarios u ON u.id = i.id_usuario WHERE u.perfil <> 'atleta'`) {
		t.Errorf("registro de instrutor em conta que não é de atleta após a 00020")
	}
	if depois.SomaSaldos != depois.SomaLedger {
		t.Errorf("Σ saldos %d != Σ ledger %d", depois.SomaSaldos, depois.SomaLedger)
	}
	conferirInvariante(t, d, "clone depois")
	if err := database.Migrar(context.Background(), sqlDB); err != nil {
		t.Fatalf("Migrar de novo no clone: %v", err)
	}
}

// Repositórios da moderação sobre o banco do harness (00001–00019).
func TestModeracao_Repositorios(t *testing.T) {
	agora := time.Now()
	a, b := novoUsuario(t), novoUsuario(t)
	adm := novoAdminDB(t, models.PapelAdmin)
	den := repositories.NovoDenunciaRepository(db)
	pun := repositories.NovoPunicaoRepository(db)
	pen := repositories.NovoPagamentoPendenteRepository(db)
	deb := repositories.NovoDebitoRepository(db)
	evt := repositories.NovoModeracaoEventoRepository(db)
	dm := repositories.NovoDesafioModeracaoRepository(db)
	cart := repositories.NovoCarteiraRepository(db)

	// Denúncia: 1 aberta por trio; assumir/decidir por CAS.
	d1 := &models.Denuncia{IDDenunciante: a.ID, IDDenunciado: b.ID, Categoria: models.DenunciaCondutaAbusiva, Descricao: strings.Repeat("x", 30)}
	if err := den.Criar(nil, d1); err != nil {
		t.Fatal(err)
	}
	if err := den.Criar(nil, &models.Denuncia{IDDenunciante: a.ID, IDDenunciado: b.ID, Categoria: models.DenunciaOutro, Descricao: strings.Repeat("y", 30)}); !repositories.EhDuplicata(err) {
		t.Errorf("segunda denúncia aberta deveria dar 1062: %v", err)
	}
	if ok, err := den.AssumirComoAdmin(nil, d1.ID, adm.ID); !ok || err != nil {
		t.Errorf("assumir: %v %v", ok, err)
	}
	if ok, _ := den.AssumirComoAcademia(nil, d1.ID, 1, a.ID); ok {
		t.Errorf("academia não pode assumir denúncia do admin")
	}
	if ok, err := den.Decidir(nil, d1.ID, models.DenunciaProcedente, "comprovado", agora, models.JurisdicaoAdmin, nil); !ok || err != nil {
		t.Errorf("decidir: %v %v", ok, err)
	}
	if ok, _ := den.Decidir(nil, d1.ID, models.DenunciaImprocedente, "de novo", agora, models.JurisdicaoAdmin, nil); ok {
		t.Errorf("decisão é terminal")
	}
	if o, _ := den.ProximaOrdemEvidencia(nil, d1.ID); o != 1 {
		t.Errorf("próxima ordem %d", o)
	}

	// Punição: suspensão por admin endurece (token_versao+1); revogar recalcula.
	var tv0 int
	db.Raw("SELECT token_versao FROM usuarios WHERE id = ?", b.ID).Scan(&tv0)
	fim := agora.Add(48 * time.Hour)
	p := &models.Punicao{IDDenuncia: &d1.ID, IDUsuario: b.ID, Tipo: models.PunicaoSuspensao, Escopo: models.EscopoGlobal,
		Fim: &fim, AplicadaPorAdmin: &adm.ID, Justificativa: "suspenso por conduta"}
	if err := pun.Aplicar(nil, p); err != nil {
		t.Fatal(err)
	}
	// relógio atual: inicio = NOW(3) do banco, posterior ao `agora` do começo do teste (lento com -race)
	est, endureceu, err := pun.RecalcularEstadoConta(nil, b.ID, time.Now().Add(time.Second))
	if err != nil || !endureceu || est.SuspensoAte == nil || est.SuspensoAte.Before(fim.Add(-time.Second)) {
		t.Errorf("recalcular: %+v %v %v", est, endureceu, err)
	}
	var tv1 int
	db.Raw("SELECT token_versao FROM usuarios WHERE id = ?", b.ID).Scan(&tv1)
	if tv1 != tv0+1 {
		t.Errorf("token_versao %d -> %d", tv0, tv1)
	}
	esperar(t, api(t, "GET", "/api/carteira", b.Token, nil), 401) // sessão derrubada
	if ok, _ := pun.Revogar(nil, p.ID, adm.ID, "recurso aceito", agora); !ok {
		t.Errorf("revogar")
	}
	if est, endureceu, _ := pun.RecalcularEstadoConta(nil, b.ID, agora.Add(time.Second)); endureceu || est.SuspensoAte != nil {
		t.Errorf("após revogar: %+v %v", est, endureceu)
	}
	// Escopo global sem admin viola chk_punicoes_aplicador.
	if err := pun.Aplicar(nil, &models.Punicao{IDUsuario: b.ID, Tipo: models.PunicaoAdvertencia, Escopo: models.EscopoGlobal,
		AplicadaPor: &a.ID, Justificativa: "x"}); err == nil {
		t.Errorf("advertência global aplicada por usuário deveria violar o CHECK")
	}
	rc := &models.Recurso{IDPunicao: p.ID, Texto: strings.Repeat("r", 30)}
	if err := pun.CriarRecurso(nil, rc); err != nil {
		t.Fatal(err)
	}
	if err := pun.CriarRecurso(nil, &models.Recurso{IDPunicao: p.ID, Texto: "outro"}); !repositories.EhDuplicata(err) {
		t.Errorf("segundo recurso deveria dar 1062: %v", err)
	}
	if ok, _ := pun.JulgarRecurso(nil, rc.ID, models.RecursoDeferido, adm.ID, "ok", agora); !ok {
		t.Errorf("julgar recurso")
	}

	// Evento: CHECK de ator.
	if err := evt.Registrar(nil, &models.ModeracaoEvento{IDAdmin: &adm.ID, AtorPapel: models.AtorAdmin, Acao: "punicao_aplicada",
		Entidade: models.EntidadePunicao, IDEntidade: p.ID}); err != nil {
		t.Errorf("evento admin: %v", err)
	}
	if err := evt.Registrar(nil, &models.ModeracaoEvento{IDAtor: &a.ID, AtorPapel: models.AtorAdmin, Acao: "x",
		Entidade: models.EntidadePunicao, IDEntidade: p.ID}); err == nil {
		t.Errorf("evento admin com id_ator de usuário deveria violar o CHECK")
	}

	// Pendentes, débito e abatimento sobre um desafio qualquer do usuário.
	var idDesafio uint
	execQA(t, db, `INSERT INTO desafios (titulo,categoria,valor_entrada_centavos,data_evento,status,vagas,posicoes_podio,id_criador,criado_em,atualizado_em)
		VALUES ('Moderação', 'geral', 0, NOW(3), 'encerrado', 10, 3, ?, NOW(3), NOW(3))`, a.ID)
	db.Raw("SELECT MAX(id) FROM desafios WHERE id_criador = ?", a.ID).Scan(&idDesafio)
	lote := []models.PagamentoPendente{
		{IDDesafio: idDesafio, IDUsuario: &a.ID, Tipo: models.PendentePremio, ValorCentavos: 1000, LiberarEm: agora.Add(-time.Minute)},
		{IDDesafio: idDesafio, Tipo: models.PendenteTaxaInscricao, ValorCentavos: 100, LiberarEm: agora.Add(-time.Minute)},
	}
	if err := pen.CriarLote(nil, lote); err != nil {
		t.Fatal(err)
	}
	if n, _ := pen.Reter(nil, idDesafio, d1.ID); n != 2 {
		t.Errorf("reter %d", n)
	}
	if n, _ := pen.Reabrir(nil, d1.ID); n != 2 {
		t.Errorf("reabrir %d", n)
	}
	if tot, _ := pen.TotaisDoUsuario(a.ID); tot.ALiberar != 1000 {
		t.Errorf("totais %+v", tot)
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		lib, err := pen.TravarLiberaveis(tx, idDesafio, agora)
		if err != nil || len(lib) != 2 {
			t.Errorf("liberáveis %d %v", len(lib), err)
		}
		for _, l := range lib {
			var idTx *uint
			if l.IDUsuario != nil {
				tr, err := cart.Creditar(tx, repositories.Lancamento{IDUsuario: *l.IDUsuario, Tipo: models.TipoPremioDesafio, ValorCentavos: l.ValorCentavos, IDDesafio: &idDesafio})
				if err != nil {
					return err
				}
				idTx = &tr.ID
			}
			if ok, err := pen.MarcarLiberado(tx, l.ID, idTx, agora); !ok || err != nil {
				t.Errorf("marcar liberado %v %v", ok, err)
			}
		}
		// débito de 1500 e abatimento até o crédito de 1000 que acabou de entrar
		if _, err := deb.Criar(tx, a.ID, idDesafio, 1500, "Anulação do desafio"); err != nil {
			return err
		}
		ab, err := deb.AbaterDebitos(tx, cart, a.ID, 1000, agora)
		if err != nil || len(ab) != 1 || ab[0].Valor != 1000 || ab[0].Quitado {
			t.Errorf("abatimento %+v %v", ab, err)
		}
		total, err := deb.TotalAberto(tx, a.ID)
		if err != nil || total != 500 {
			t.Errorf("débito aberto %d %v", total, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := pen.ExisteLiberado(nil, idDesafio); !ok {
		t.Errorf("deveria existir liberado")
	}
	if err := dm.RegistrarEstornoTaxa(nil, idDesafio, 100); err != nil {
		t.Errorf("estorno taxa: %v", err)
	}
	if err := dm.RegistrarEstornoTaxa(nil, idDesafio, 100); !repositories.EhDuplicata(err) {
		t.Errorf("segundo estorno de taxa deveria dar 1062: %v", err)
	}
	if ok, _ := dm.AnularResultado(nil, idDesafio, models.AnulacaoRelancarPodio, agora); !ok {
		t.Errorf("anular")
	}
	if ok, _ := dm.AnularResultado(nil, idDesafio, models.AnulacaoDevolverInscricoes, agora); ok {
		t.Errorf("anulação vigente única")
	}
	if ok, _ := dm.ReabrirParaRelancamento(nil, idDesafio); !ok {
		t.Errorf("reabrir para relançamento")
	}
	ledgerConfere(t, a)
}
