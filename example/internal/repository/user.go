package repository

import (
	"context"

	"github.com/pemako/mk/database"
	"gorm.io/gorm"

	"github.com/pemako/mk/example/internal/model"
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

// List returns a page of users by offset and limit.
func (r *UserRepo) List(ctx context.Context, offset, limit int) ([]model.User, error) {
	var users []model.User
	err := database.DB("default").WithContext(ctx).Offset(offset).Limit(limit).Find(&users).Error
	return users, err
}

// Count returns the total number of users.
func (r *UserRepo) Count(ctx context.Context) (int64, error) {
	var count int64
	err := database.DB("default").WithContext(ctx).Model(&model.User{}).Count(&count).Error
	return count, err
}
