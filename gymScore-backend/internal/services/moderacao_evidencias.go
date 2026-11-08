package services

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"gymScore-backend/internal/models"
	"gymScore-backend/internal/repositories"

	"gorm.io/gorm"
)

// Evidências: imagens guardadas na pasta PRIVADA EVIDENCIAS_DIR (nunca sob /uploads), com nome
// aleatório de 128 bits; o tipo é decidido pelos magic bytes (nunca pela extensão/Content-Type do
// cliente) e o arquivo só é servido por rota autenticada a denunciante, academia julgadora e admin.

// mimesEvidencia são os tipos aceitos e a extensão gravada.
var mimesEvidencia = map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp"}

// DetectarImagem devolve o mime da imagem pelos magic bytes ("" se não for JPEG, PNG ou WebP).
func DetectarImagem(b []byte) string {
	switch {
	case len(b) >= 3 && b[0] == 0xFF && b[1] == 0xD8 && b[2] == 0xFF:
		return "image/jpeg"
	case len(b) >= 8 && bytes.Equal(b[:8], []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1A, '\n'}):
		return "image/png"
	case len(b) >= 12 && bytes.Equal(b[:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")):
		return "image/webp"
	}
	return ""
}

// ErroArquivoGrande é o 413 de evidência acima do limite.
func erroArquivoGrande(max int64) *ErroNegocio {
	return erro(http.StatusRequestEntityTooLarge, fmt.Sprintf("A imagem deve ter no máximo %d KB", max/1024))
}

func nomeAleatorio(ext string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b) + ext, nil
}

func (s *moderacaoService) EnviarEvidencia(userID, idDenuncia uint, conteudo []byte) (*models.EvidenciaResponse, error) {
	if len(conteudo) == 0 {
		return nil, erroRequisicao("Envie uma imagem (campo arquivo)")
	}
	if int64(len(conteudo)) > s.cfg.EvidenciaMaxBytes {
		return nil, erroArquivoGrande(s.cfg.EvidenciaMaxBytes)
	}
	mime := DetectarImagem(conteudo)
	if mime == "" {
		return nil, erroRequisicao("Formato não aceito: envie uma imagem JPEG, PNG ou WebP")
	}
	if err := os.MkdirAll(s.cfg.EvidenciasDir, 0o700); err != nil {
		return nil, err
	}
	nome, err := nomeAleatorio(mimesEvidencia[mime])
	if err != nil {
		return nil, err
	}
	caminho := filepath.Join(s.cfg.EvidenciasDir, nome)
	ev := &models.DenunciaEvidencia{IDDenuncia: idDenuncia, Caminho: nome, Mime: mime, TamanhoBytes: uint64(len(conteudo))}
	gravado := false
	err = s.db.Transaction(func(tx *gorm.DB) error {
		d, err := s.denuncias.Buscar(tx, idDenuncia, true) // trava a denúncia: serializa envios simultâneos
		if err != nil {
			return err
		}
		if d == nil || d.IDDenunciante != userID {
			return erroNaoEncontrado("Denúncia não encontrada")
		}
		if d.Status != models.DenunciaAberta {
			return erroConflito("Só é possível enviar evidências enquanto a denúncia está aberta")
		}
		ordem, err := s.denuncias.ProximaOrdemEvidencia(tx, idDenuncia)
		if err != nil {
			return err
		}
		if ordem == 0 {
			return erroConflito(fmt.Sprintf("A denúncia já tem %d evidências", models.DenunciaEvidenciasMaximo))
		}
		ev.Ordem = ordem
		if err := os.WriteFile(caminho, conteudo, 0o600); err != nil {
			return err
		}
		gravado = true
		if err := s.denuncias.AdicionarEvidencia(tx, ev); err != nil {
			if repositories.EhDuplicata(err) {
				return erroConflito("Evidência enviada ao mesmo tempo por outra requisição; tente de novo")
			}
			return err
		}
		return s.registrar(tx, atorUsuario(userID), EventoEvidenciaEnviada, models.EntidadeDenuncia, idDenuncia, d.IDDesafio, "", nil,
			map[string]any{"ordem": ordem, "mime": mime, "tamanho_bytes": len(conteudo)})
	})
	if err != nil {
		if gravado {
			_ = os.Remove(caminho) // a linha não foi gravada: não deixa arquivo órfão
		}
		return nil, err
	}
	return &models.EvidenciaResponse{Ordem: ev.Ordem, Mime: ev.Mime, TamanhoBytes: ev.TamanhoBytes, CriadoEm: ev.CriadoEm}, nil
}

// podeLerEvidencia aplica o sigilo: denunciante (a própria), academia julgadora (só enquanto a
// jurisdição for dela) e admin.
func (s *moderacaoService) podeLerEvidencia(papel PapelLeitor, idLeitor uint, d *models.Denuncia) (bool, error) {
	switch papel {
	case LeitorAdmin:
		return true, nil
	case LeitorDenunciante:
		return d.IDDenunciante == idLeitor, nil
	case LeitorAcademia:
		a, err := s.academias.BuscarPorUsuario(idLeitor)
		if err != nil || a == nil {
			return false, err
		}
		return d.Jurisdicao == models.JurisdicaoAcademia && d.IDAcademiaJulgadora != nil && *d.IDAcademiaJulgadora == a.ID, nil
	}
	return false, nil
}

func (s *moderacaoService) Evidencia(papel PapelLeitor, idLeitor, idDenuncia uint, ordem uint8) (*ArquivoEvidencia, error) {
	naoEncontrada := erroNaoEncontrado("Evidência não encontrada")
	d, err := s.denuncias.Buscar(nil, idDenuncia, false)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, naoEncontrada
	}
	if ok, err := s.podeLerEvidencia(papel, idLeitor, d); err != nil {
		return nil, err
	} else if !ok {
		return nil, naoEncontrada // 404 (não revela a existência)
	}
	for _, e := range d.Evidencias {
		if e.Ordem != ordem {
			continue
		}
		// Só o nome-base gravado por nós; nunca um caminho vindo de fora.
		conteudo, err := os.ReadFile(filepath.Join(s.cfg.EvidenciasDir, filepath.Base(e.Caminho)))
		if errors.Is(err, os.ErrNotExist) {
			return nil, naoEncontrada
		}
		if err != nil {
			return nil, err
		}
		return &ArquivoEvidencia{Conteudo: conteudo, Mime: e.Mime,
			Nome: fmt.Sprintf("denuncia-%d-evidencia-%d%s", d.ID, e.Ordem, mimesEvidencia[e.Mime])}, nil
	}
	return nil, naoEncontrada
}
