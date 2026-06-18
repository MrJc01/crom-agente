package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"01_go_api/internal/models"
)

// ErrorResponse representa a estrutura de erro padronizada
type ErrorResponse struct {
	Error string `json:"error"`
}

// TaskHandler gerencia as requisições HTTP para as tarefas
type TaskHandler struct {
	TaskStore *models.TaskStore
}

// NewTaskHandler cria um novo TaskHandler
func NewTaskHandler(taskStore *models.TaskStore) *TaskHandler {
	return &TaskHandler{
		TaskStore: taskStore,
	}
}

// HealthCheck retora o status da API
func (th *TaskHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "service": "task-api"})
}

// HandleTasks lida com as requisições GET, POST, PUT e DELETE para /api/tasks
func (th *TaskHandler) HandleTasks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		th.GetTasks(w, r)
	case http.MethodPost:
		th.CreateTask(w, r)
	case http.MethodPut:
		th.UpdateTask(w, r)
	case http.MethodDelete:
		th.DeleteTask(w, r)
	default:
		th.sendError(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// GetTasks retorna todas as tarefas
func (th *TaskHandler) GetTasks(w http.ResponseWriter, r *http.Request) {
	tasks := th.TaskStore.GetAllTasks()
	json.NewEncoder(w).Encode(tasks)
}

// CreateTask cria uma nova tarefa
func (th *TaskHandler) CreateTask(w http.ResponseWriter, r *http.Request) {
	var task models.Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		th.sendError(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	newTask := th.TaskStore.AddTask(task)
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(newTask)
}

// UpdateTask atualiza uma tarefa existente
func (th *TaskHandler) UpdateTask(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	id, err := strconv.Atoi(idStr)
	if err != nil || id == 0 {
		th.sendError(w, "invalid task ID", http.StatusBadRequest)
		return
	}

	var updatedTask models.Task
	if err := json.NewDecoder(r.Body).Decode(&updatedTask); err != nil {
		th.sendError(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	updatedTask.ID = id // Ensure the ID from the URL is used

	_, err = th.TaskStore.UpdateTask(updatedTask)
	if err != nil {
		th.sendError(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(updatedTask)
}

// DeleteTask remove uma tarefa
func (th *TaskHandler) DeleteTask(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/tasks/")
	id, err := strconv.Atoi(idStr)
	if err != nil || id == 0 {
		th.sendError(w, "invalid task ID", http.StatusBadRequest)
		return
	}

	if err := th.TaskStore.DeleteTask(id); err != nil {
		th.sendError(w, err.Error(), http.StatusNotFound)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (th *TaskHandler) sendError(w http.ResponseWriter, message string, statusCode int) {
	log.Printf("Error: %s", message)
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(ErrorResponse{Error: message})
}
