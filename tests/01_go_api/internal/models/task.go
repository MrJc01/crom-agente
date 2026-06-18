package models

import (
	"errors"
	"sync"
	"time"
)

// Task represents a task in the system
type Task struct {
	ID          int       `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Done        bool      `json:"done"`
	CreatedAt   time.Time `json:"created_at"`
}

// TaskStore manages tasks in memory
type TaskStore struct {
	tasks  []Task
	nextID int
	mu     sync.Mutex
}

// NewTaskStore creates a new TaskStore
func NewTaskStore() *TaskStore {
	return &TaskStore{
		tasks:  make([]Task, 0),
		nextID: 1,
	}
}

// GetAllTasks returns all tasks
func (ts *TaskStore) GetAllTasks() []Task {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	return ts.tasks
}

// GetTaskByID returns a task by its ID
func (ts *TaskStore) GetTaskByID(id int) (*Task, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	for i, task := range ts.tasks {
		if task.ID == id {
			return &ts.tasks[i], nil
		}
	}
	return nil, errors.New("task not found")
}

// AddTask adds a new task to the store
func (ts *TaskStore) AddTask(task Task) Task {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	task.ID = ts.nextID
	ts.nextID++
	task.CreatedAt = time.Now()
	ts.tasks = append(ts.tasks, task)
	return task
}

// UpdateTask updates an existing task
func (ts *TaskStore) UpdateTask(updatedTask Task) (*Task, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	for i, task := range ts.tasks {
		if task.ID == updatedTask.ID {
			ts.tasks[i].Title = updatedTask.Title
			ts.tasks[i].Description = updatedTask.Description
			ts.tasks[i].Done = updatedTask.Done
			return &ts.tasks[i], nil
		}
	}
	return nil, errors.New("task no found to update")
}

// DeleteTask removes a task by its ID
func (ts *TaskStore) DeleteTask(id int) error {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	for i, task := range ts.tasks {
		if task.ID == id {
			ts.tasks = append(ts.tasks[:i], ts.tasks[i+1:]...) // This should work if the ID exists
			return nil
		}
	}
	return errors.New("task not found to delete")
}
