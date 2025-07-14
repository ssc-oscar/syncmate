package db

import (
	"gorm.io/gorm"
)

// DBOperation defines the interface for database operations
type DBOperation interface {
	// CreateTask creates a new task in the database.
	CreateTask(task *Task) error
	// GetTask retrieves a task by its ID.
	GetTask(virtualPath string) (*Task, error)
	// UpdateTask updates an existing task in the database.
	UpdateTask(task *Task) error
	// DeleteTask deletes a task by its ID.
	DeleteTask(virtualPath string) error
	// ListTasks retrieves all tasks with pagination.
	ListTasks(offset, limit int) ([]*Task, error)
	// CountTasks returns the total number of tasks in the database.
	CountTasks() (int64, error)
}

// DB is the concrete implementation of DBOperation
type DB struct {
	conn *gorm.DB
}

func (db *DB) CreateTask(task *Task) error {
	if err := db.conn.Create(task).Error; err != nil {
		return err
	}
	return nil
}

func (db *DB) GetTask(virtualPath string) (*Task, error) {
	var task Task
	if err := db.conn.First(&task, virtualPath).Error; err != nil {
		return nil, err
	}
	return &task, nil
}

func (db *DB) UpdateTask(task *Task) error {
	if err := db.conn.Save(task).Error; err != nil {
		return err
	}
	return nil
}

func (db *DB) DeleteTask(virtualPath string) error {
	if err := db.conn.Where("virtual_path = ?", virtualPath).Delete(&Task{}).Error; err != nil {
		return err
	}
	return nil
}

func (db *DB) ListTasks(offset, limit int) ([]*Task, error) {
	var tasks []*Task
	if err := db.conn.Offset(offset).Limit(limit).Find(&tasks).Error; err != nil {
		return nil, err
	}
	return tasks, nil
}

func (db *DB) CountTasks() (int64, error) {
	var count int64
	if err := db.conn.Model(&Task{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// NewDB creates a new DB instance with the given gorm.DB connection
func NewDB(conn *gorm.DB) *DB {
	if conn == nil {
		panic("gorm.DB connection cannot be nil")
	}
	if err := conn.AutoMigrate(&Task{}); err != nil {
		panic("failed to auto migrate Task model: " + err.Error())
	}
	return &DB{conn: conn}
}
