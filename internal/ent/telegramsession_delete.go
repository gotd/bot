// Code generated by ent, DO NOT EDIT.

package ent

import (
	"context"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqlgraph"
	"entgo.io/ent/schema/field"
	"github.com/gotd/bot/internal/ent/predicate"
	"github.com/gotd/bot/internal/ent/telegramsession"
)

// TelegramSessionDelete is the builder for deleting a TelegramSession entity.
type TelegramSessionDelete struct {
	config
	hooks    []Hook
	mutation *TelegramSessionMutation
}

// Where appends a list predicates to the TelegramSessionDelete builder.
func (tsd *TelegramSessionDelete) Where(ps ...predicate.TelegramSession) *TelegramSessionDelete {
	tsd.mutation.Where(ps...)
	return tsd
}

// Exec executes the deletion query and returns how many vertices were deleted.
func (tsd *TelegramSessionDelete) Exec(ctx context.Context) (int, error) {
	return withHooks(ctx, tsd.sqlExec, tsd.mutation, tsd.hooks)
}

// ExecX is like Exec, but panics if an error occurs.
func (tsd *TelegramSessionDelete) ExecX(ctx context.Context) int {
	n, err := tsd.Exec(ctx)
	if err != nil {
		panic(err)
	}
	return n
}

func (tsd *TelegramSessionDelete) sqlExec(ctx context.Context) (int, error) {
	_spec := sqlgraph.NewDeleteSpec(telegramsession.Table, sqlgraph.NewFieldSpec(telegramsession.FieldID, field.TypeUUID))
	if ps := tsd.mutation.predicates; len(ps) > 0 {
		_spec.Predicate = func(selector *sql.Selector) {
			for i := range ps {
				ps[i](selector)
			}
		}
	}
	affected, err := sqlgraph.DeleteNodes(ctx, tsd.driver, _spec)
	if err != nil && sqlgraph.IsConstraintError(err) {
		err = &ConstraintError{msg: err.Error(), wrap: err}
	}
	tsd.mutation.done = true
	return affected, err
}

// TelegramSessionDeleteOne is the builder for deleting a single TelegramSession entity.
type TelegramSessionDeleteOne struct {
	tsd *TelegramSessionDelete
}

// Where appends a list predicates to the TelegramSessionDelete builder.
func (tsdo *TelegramSessionDeleteOne) Where(ps ...predicate.TelegramSession) *TelegramSessionDeleteOne {
	tsdo.tsd.mutation.Where(ps...)
	return tsdo
}

// Exec executes the deletion query.
func (tsdo *TelegramSessionDeleteOne) Exec(ctx context.Context) error {
	n, err := tsdo.tsd.Exec(ctx)
	switch {
	case err != nil:
		return err
	case n == 0:
		return &NotFoundError{telegramsession.Label}
	default:
		return nil
	}
}

// ExecX is like Exec, but panics if an error occurs.
func (tsdo *TelegramSessionDeleteOne) ExecX(ctx context.Context) {
	if err := tsdo.Exec(ctx); err != nil {
		panic(err)
	}
}