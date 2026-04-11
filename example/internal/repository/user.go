package repository

import (
	"context"

	"github.com/pemako/mk/database"
	"github.com/pemako/mk/example/internal/model"
	"gorm.io/gorm"
)

type UserRepo struct{}

func (r *UserRepo) Create(ctx context.Context, u *model.User) error {
	return database.DB("default").WithContext(ctx).Create(u).Error
}

func (r *UserRepo) FindByID(ctx context.Context, id uint) (*model.User, error) {
	var u model.User
	err := database.DB("default").WithContext(ctx).First(&u, id).Error
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return &u, err
}

func (r *UserRepo) List(ctx context.Context) ([]model.User, error) {
	var users []model.User
	err := database.DB("default").WithContext(ctx).Find(&users).Error
	return users, err
}
