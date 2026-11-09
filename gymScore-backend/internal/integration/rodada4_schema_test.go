//go:build integration

package integration

// Migration 00020 (rodada 4): perfil = tipo de conta (instrutor vira modo da conta de atleta) e
// endereço estruturado da academia.

import (
	"context"
	"strings"
	"testing"

	"gymScore-backend/internal/repositories"
)

func TestMigracao_00020_PerfilEEndereco(t *testing.T) {
	d, sqlDB := bancoQA(t, "r4", 19)
	ctx := context.Background()
	p := providerQA(t, sqlDB)

	// Estado da 00019: conta perfil 'instrutor' com registro aprovado, outra com pedido pendente, um atleta
	// e uma academia com endereço em texto livre.
	instAprov := usuarioLegado(t, d, "instrutor")
	instPend := usuarioLegado(t, d, "instrutor")
	atleta := usuarioLegado(t, d, "atleta")
	contaAc := usuarioLegado(t, d, "academia")
	execQA(t, d, "INSERT INTO instrutores (id_usuario, cref, status) VALUES (?, '1-G/SP', 'aprovada'), (?, '2-G/SP', 'pendente')", instAprov, instPend)
	execQA(t, d, "INSERT INTO academias (id_usuario, razao_social, nome_fantasia, cnpj, endereco, status) VALUES (?, 'R', 'F', '11.111.111/0001-11', 'Rua Velha, 10, Centro', 'aprovada')", contaAc)

	if _, err := p.UpTo(ctx, 20); err != nil {
		t.Fatalf("UpTo 20: %v", err)
	}
	perfil := func(id uint) (s string) { d.Raw("SELECT perfil FROM usuarios WHERE id = ?", id).Scan(&s); return }
	for _, id := range []uint{instAprov, instPend, atleta} {
		if pf := perfil(id); pf != "atleta" {
			t.Errorf("usuário %d com perfil %q após a 00020", id, pf)
		}
	}
	if perfil(contaAc) != "academia" {
		t.Errorf("conta de academia mudou de perfil")
	}
	var nInst int64
	d.Raw("SELECT COUNT(*) FROM instrutores").Scan(&nInst)
	if nInst != 2 {
		t.Errorf("registros de instrutor: %d", nInst)
	}
	var end string
	d.Raw("SELECT endereco FROM academias WHERE id_usuario = ?", contaAc).Scan(&end)
	if end != "Rua Velha, 10, Centro" {
		t.Errorf("endereço antigo alterado: %q", end)
	}
	if res := d.Exec("UPDATE usuarios SET perfil = 'instrutor' WHERE id = ?", atleta); res.Error == nil {
		t.Errorf("perfil 'instrutor' não deveria mais ser aceito")
	}

	// CHECKs do endereço.
	for _, q := range []string{
		"UPDATE academias SET cep = '1234567' WHERE id_usuario = ?",  // 7 dígitos
		"UPDATE academias SET cep = '1234567a' WHERE id_usuario = ?", // não dígito
		"UPDATE academias SET uf = 'XX' WHERE id_usuario = ?",
		"UPDATE academias SET uf = 'sp' WHERE id_usuario = ?",
	} {
		if err := d.Exec(q, contaAc).Error; err == nil || !strings.Contains(err.Error(), "3819") {
			t.Errorf("%s deveria violar o CHECK: %v", q, err)
		}
	}
	execQA(t, d, `UPDATE academias SET cep = '01310100', logradouro = 'Avenida Paulista', numero = '1000', complemento = 'Sala 1',
		bairro = 'Bela Vista', cidade = 'São Paulo', uf = 'SP', endereco = ? WHERE id_usuario = ?`,
		strings.Repeat("x", 400), contaAc)

	// PapeisDoUsuario.
	repo := repositories.NovoUsuarioRepository(d)
	pa, err := repo.PapeisDoUsuario(instAprov)
	if err != nil || pa == nil || pa.Conta != "atleta" || !pa.InstrutorAprovado() || len(pa.ModosDisponiveis()) != 2 {
		t.Errorf("papéis do instrutor aprovado: %+v %v", pa, err)
	}
	pp, _ := repo.PapeisDoUsuario(instPend)
	if pp == nil || pp.InstrutorStatus != "pendente" || pp.InstrutorAprovado() || len(pp.ModosDisponiveis()) != 1 {
		t.Errorf("papéis do instrutor pendente: %+v", pp)
	}
	pc, _ := repo.PapeisDoUsuario(contaAc)
	if pc == nil || pc.Conta != "academia" || pc.AcademiaID == 0 || pc.AcademiaStatus != "aprovada" || pc.ModosDisponiveis() != nil {
		t.Errorf("papéis da academia: %+v", pc)
	}
	if px, err := repo.PapeisDoUsuario(999999); px != nil || err != nil {
		t.Errorf("usuário inexistente: %+v %v", px, err)
	}

	// Down: volta ao perfil exclusivo; endereco > 255 mantém VARCHAR(500) (não trunca).
	if _, err := p.DownTo(ctx, 19); err != nil {
		t.Fatalf("Down 20: %v", err)
	}
	if perfil(instAprov) != "instrutor" || perfil(instPend) != "instrutor" || perfil(atleta) != "atleta" || perfil(contaAc) != "academia" {
		t.Errorf("Down não restaurou os perfis")
	}
	if colunaExiste(t, d, "academias", "cep") || colunaExiste(t, d, "academias", "uf") {
		t.Errorf("colunas do endereço sobraram após o Down")
	}
	var tam int64
	d.Raw("SELECT CHARACTER_MAXIMUM_LENGTH FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'academias' AND column_name = 'endereco'").Scan(&tam)
	if tam != 500 {
		t.Errorf("endereco com %d caracteres não deveria ter sido estreitado", tam)
	}
	// Up de novo e Up reaplicado sobre o schema já migrado (versão 20 apagada do goose_db_version).
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("Up de novo: %v", err)
	}
	execQA(t, d, "DELETE FROM goose_db_version WHERE version_id = 20")
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("Up reaplicado sobre o schema migrado: %v", err)
	}
	if versaoGoose(t, d) != versaoFinal || perfil(instAprov) != "atleta" || !colunaExiste(t, d, "academias", "uf") {
		t.Errorf("estado após Up de novo incorreto")
	}

	// Conta de academia com registro de instrutor (incoerente): a 00020 aborta.
	if _, err := p.DownTo(ctx, 19); err != nil {
		t.Fatalf("Down 20 (2): %v", err)
	}
	execQA(t, d, "INSERT INTO instrutores (id_usuario, cref) VALUES (?, '3-G/SP')", contaAc)
	if _, err := p.Up(ctx); err == nil || !strings.Contains(err.Error(), "00020") {
		t.Errorf("Up deveria abortar com academia + instrutor: %v", err)
	}
}
