package db

import (
	"testing"
)

func SetupDBInstance(t *testing.T) *DB {
	db, err := SetupD1DB()
	if err != nil {
		t.Fatalf("Failed to connect to database: %v", err)
	}

	// For SQLite, we don't need to drop tables as we use in-memory database
	// For D1, we drop the test table if it exists
	if err := db.Exec("DROP TABLE IF EXISTS tasks").Error; err != nil {
		// Ignore error for SQLite in-memory database
		t.Logf("Note: Could not drop test table (this is normal for in-memory databases): %v", err)
	}

	dbInstance := NewDB(db)
	if dbInstance.getConnection() == nil {
		t.Fatal("Expected DB connection to be non-nil")
	}
	return dbInstance
}

func TestCreateTask(t *testing.T) {
	dbInstance := SetupDBInstance(t)

	// Create a test task with a unique path
	virtualPath := "/test/create_file1.txt"

	// Clean up any existing task first
	_ = dbInstance.DeleteTask(virtualPath)

	task := &Task{
		VirtualPath: virtualPath,
		SrcPath:     "/source/file1.txt",
		Status:      Pending,
	}

	err := dbInstance.CreateTask(task)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// Verify the task was created
	if task.ID == 0 {
		t.Fatal("Expected task ID to be set after creation")
	}

	// Clean up
	_ = dbInstance.DeleteTask(task.VirtualPath)
}

func TestGetTask(t *testing.T) {
	dbInstance := SetupDBInstance(t)

	// Create a test task first
	originalTask := &Task{
		VirtualPath: "/test/file2.txt",
		SrcPath:     "/source/file2.txt",
		Status:      Uploading,
	}

	err := dbInstance.CreateTask(originalTask)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// Get the task
	retrievedTask, err := dbInstance.GetTask(originalTask.VirtualPath)
	if err != nil {
		t.Fatalf("Failed to get task: %v", err)
	}

	// Verify the retrieved task
	if retrievedTask.VirtualPath != originalTask.VirtualPath {
		t.Errorf("Expected VirtualPath %s, got %s", originalTask.VirtualPath, retrievedTask.VirtualPath)
	}
	if retrievedTask.SrcPath != originalTask.SrcPath {
		t.Errorf("Expected SrcPath %s, got %s", originalTask.SrcPath, retrievedTask.SrcPath)
	}
	if retrievedTask.Status != originalTask.Status {
		t.Errorf("Expected Status %d, got %d", originalTask.Status, retrievedTask.Status)
	}

	// Clean up
	_ = dbInstance.DeleteTask(originalTask.VirtualPath)
}

func TestUpdateTask(t *testing.T) {
	dbInstance := SetupDBInstance(t)

	// Create a test task first
	task := &Task{
		VirtualPath: "/test/file3.txt",
		SrcPath:     "/source/file3.txt",
		Status:      Pending,
	}

	err := dbInstance.CreateTask(task)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// Update the task
	task.Status = Uploaded
	task.SrcPath = "/source/updated_file3.txt"

	err = dbInstance.UpdateTask(task)
	if err != nil {
		t.Fatalf("Failed to update task: %v", err)
	}

	// Verify the update
	updatedTask, err := dbInstance.GetTask(task.VirtualPath)
	if err != nil {
		t.Fatalf("Failed to get updated task: %v", err)
	}

	if updatedTask.Status != Uploaded {
		t.Errorf("Expected Status %d, got %d", Uploaded, updatedTask.Status)
	}
	if updatedTask.SrcPath != "/source/updated_file3.txt" {
		t.Errorf("Expected SrcPath %s, got %s", "/source/updated_file3.txt", updatedTask.SrcPath)
	}

	// Clean up
	_ = dbInstance.DeleteTask(task.VirtualPath)
}

func TestDeleteTask(t *testing.T) {
	dbInstance := SetupDBInstance(t)

	// Create a test task first
	task := &Task{
		VirtualPath: "/test/file4.txt",
		SrcPath:     "/source/file4.txt",
		Status:      Pending,
	}

	err := dbInstance.CreateTask(task)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// Delete the task
	err = dbInstance.DeleteTask(task.VirtualPath)
	if err != nil {
		t.Fatalf("Failed to delete task: %v", err)
	}

	// Verify the task was deleted
	_, err = dbInstance.GetTask(task.VirtualPath)
	if err == nil {
		t.Fatal("Expected error when getting deleted task, but got none")
	}
}
