package models

import "time"

// Origem de um membro da equipe de instrutores do desafio (00021).
const (
	EquipeOrigemIndicado    = "indicado"    // posto pela academia na criação/edição (vinculado e aprovado)
	EquipeOrigemCandidatura = "candidatura" // candidatura aceita pela academia
)

// DesafioInstrutor é um membro da equipe de instrutores do desafio (tabela desafio_instrutores).
// UNIQUE(id_desafio, id_instrutor); no máximo 1 Responsavel por desafio (coluna gerada
// responsavel_desafio + UNIQUE no banco: violação = erro 1062). O tamanho máximo da equipe
// (desafios.instrutores_vagas) é validado na API. Ao trocar o responsável, desmarque o antigo
// ANTES de marcar o novo e atualize desafios.id_instrutor (espelho legado) na mesma transação.
type DesafioInstrutor struct {
	ID          uint      `gorm:"primaryKey;autoIncrement"`
	IDDesafio   uint      `gorm:"column:id_desafio;not null"`
	IDInstrutor uint      `gorm:"column:id_instrutor;not null"`
	Origem      string    `gorm:"column:origem;not null"`
	Responsavel bool      `gorm:"column:responsavel;not null;default:false"`
	CriadoEm    time.Time `gorm:"column:criado_em;autoCreateTime"`

	Instrutor *Instrutor `gorm:"foreignKey:IDInstrutor"`
}

func (DesafioInstrutor) TableName() string { return "desafio_instrutores" }

// Instrutores do desafio: limites da equipe (CHECK chk_desafios_instrutores_vagas, 00021).
const (
	InstrutoresVagasMin = 1
	InstrutoresVagasMax = 20
)

// Membro devolve o membro da equipe com esse instrutores.id (nil se não for da equipe).
// Exige Equipe carregada.
func (d *Desafio) Membro(idInstrutor uint) *DesafioInstrutor {
	if idInstrutor == 0 {
		return nil
	}
	for i := range d.Equipe {
		if d.Equipe[i].IDInstrutor == idInstrutor {
			return &d.Equipe[i]
		}
	}
	return nil
}

// ResponsavelEquipe devolve o membro responsável pelo pódio (nil se a equipe não tem responsável).
func (d *Desafio) ResponsavelEquipe() *DesafioInstrutor {
	for i := range d.Equipe {
		if d.Equipe[i].Responsavel {
			return &d.Equipe[i]
		}
	}
	return nil
}

// VagasInstrutorLivres é quantas vagas da equipe ainda estão livres (nunca negativo).
func (d *Desafio) VagasInstrutorLivres() int {
	if n := d.InstrutoresVagas - len(d.Equipe); n > 0 {
		return n
	}
	return 0
}
