package bryscad

import (
	"context"
	"errors"
	"net/http"

	"github.com/golgimed/mimic/internal/shared/lifecycle"
)

// collectionLifecycle: a collection starts PENDENTE and ends in exactly one
// terminal state. CONCLUIDO is reached through the Mimic-only /concluir
// helper; CANCELADO and REJEITADA through /cancelar and /rejeitar.
var collectionLifecycle = lifecycle.NewMachine("coleta", map[string][]string{
	"PENDENTE": {"CONCLUIDO", "CANCELADO", "REJEITADA"},
})

func newCollectionRunner(store *Store) *lifecycle.Runner {
	current := func(_ context.Context, chave string) (string, bool, error) {
		c, err := store.GetCollection(chave)
		if err != nil || c == nil {
			return "", false, err
		}
		return c.Situacao, true, nil
	}
	apply := func(_ context.Context, t lifecycle.Transition) error {
		var err error
		if t.To == "CONCLUIDO" {
			_, err = store.CompleteCollection(t.ID)
		} else {
			_, err = store.TransitionCollection(t.ID, t.To)
		}
		return err
	}
	return lifecycle.NewRunner(Name, collectionLifecycle, nil, current, apply)
}

// writeTransitionError maps a lifecycle error onto SCAD's error envelope.
// The 409 for a collection that already left PENDENTE is a Mimic
// assumption: BRy's documentation doesn't specify the response for acting
// on a finished collection.
func writeTransitionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, lifecycle.ErrNotFound):
		writeError(w, http.StatusNotFound, "coleta_nao_encontrada", "Coleta não encontrada.")
	case errors.Is(err, lifecycle.ErrInvalidTransition):
		writeError(w, http.StatusConflict, "coleta_situacao_invalida", "A coleta não está pendente.")
	default:
		writeError(w, http.StatusInternalServerError, "erro_interno", err.Error())
	}
}
