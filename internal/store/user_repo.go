package store

import (
	"context"

	"github.com/pscheid92/secretli/internal/model"
)

type UserRepo interface {
	Create(ctx context.Context, user *model.User) error
	GetByEmail(ctx context.Context, email string) (*model.User, error)
	GetByID(ctx context.Context, id int64) (*model.User, error)
}
