package rdb

import (
	"log"

	"gorm.io/gorm"
)

// AutoMigrate runs database migrations for the given models
func AutoMigrate(models ...interface{}) error {
	if GormDB == nil {
		return gorm.ErrInvalidDB
	}

	log.Println("Running auto-migrations...")
	err := GormDB.AutoMigrate(models...)
	if err != nil {
		log.Printf("Auto-migration failed: %v", err)
		return err
	}

	log.Println("Auto-migration completed successfully")
	return nil
}

func Create(model interface{}) error {
	if GormDB == nil {
		return gorm.ErrInvalidDB
	}

	log.Println("Running create...")
	err := GormDB.Create(model).Error
	if err != nil {
		log.Printf("Create failed: %v", err)
		return err
	}

	log.Println("Create completed successfully")
	return nil
}
