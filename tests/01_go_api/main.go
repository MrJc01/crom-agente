package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Task representa o modelo de dados para uma tarefa
type Task struct {
	ID          int       `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Done        bool      `json:"done"`
	CreatedAt   time.Time `json:"created_at"`
}

var (
	tasks    = make(map[int]Task)
	nextID   = 1
	tasksMux sync.Mutex // Mutex para proteger o acesso ao map de tasks
)

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/tasks", getTasksHandler)
	mux.HandleFunc("POST /api/tasks", createTaskHandler)
	mux.HandleFunc("GET /api/tasks/{id}", getTaskHandler)
	mux.HandleFunc("PUT /api/tasks/{id}", updateTaskHandler)
	mux.HandleFunc("DELETE /api/tasks/{id}", deleteTaskHandler)

	fmt.Println("Servidor iniciado na porta :8080")
	log.Fatal(http.ListenAndServe(":8080", mux))
}

// getTasksHandler lista todas as tarefas
func getTasksHandler(w http.ResponseWriter, r *http.Request) {
	tasksMux.Lock()
	defer tasksMux.Unlock()

	var allTasks []Task
	for _, task := range tasks {
		allTasks = append(allTasks, task)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(allTasks)
}

// createTaskHandler cria uma nova tarefa
func createTaskHandler(w http.ResponseWriter, r *http.Request) {
	tasksMux.Lock()
	defer tasksMux.Unlock()

	var newTask Task
	if err := json.NewDecoder(r.Body).Decode(&newTask); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	newTask.ID = nextID
	newTask.Done = false // Garante que a tarefa não é criada como concluída
	newTask.CreatedAt = time.Now()
	tasks[newTask.ID] = newTask
	nextID++

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(newTask)
}

// getTaskHandler busca uma tarefa por ID
func getTaskHandler(w http.ResponseWriter, r *http.Request) {
	tasksMux.Lock()
	defer tasksMux.Unlock()

	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "ID de tarefa inválido", http.StatusBadRequest)
		return
	}

	task, exists := tasks[id]
	if !exists {
		http.Error(w, "Tarefa não encontrada", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(task)
}

// updateTaskHandler atualiza uma tarefa existente
func updateTaskHandler(w http.ResponseWriter, r *http.Request) {
	tasksMux.Lock()
	defer tasksMux.Unlock()

	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "ID de tarefa inválido", http.StatusBadRequest)
		return
	}

	existingTask, exists := tasks[id]
	if !exists {
		http.Error(w, "Tarefa não encontrada", http.StatusNotFound)
		return
	}

	var updatedTask Task
	if err := json.NewDecoder(r.Body).Decode(&updatedTask); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Atualiza apenas os campos permitidos
	existingTask.Title = updatedTask.Title
	existingTask.Description = updatedTask.Description
	existingTask.Done = updatedTask.Done
	// O CreatedAt e ID não devem ser alterados via PUT

	tasks[id] = existingTask

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(existingTask)
}

// deleteTaskHandler remove uma tarefa
func deleteTaskHandler(w http.ResponseWriter, r *http.Request) {
	tasksMux.Lock()
	defer tasksMux.Unlock()

	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "ID de tarefa inválido", http.StatusBadRequest)
		return
	}

	_, exists := tasks[id]
	if !exists {
		http.Error(w, "Tarefa não encontrada", http.StatusNotFound)
		return
	}

	delete(tasks, id)
	w.WriteHeader(http.StatusNoContent)
}

