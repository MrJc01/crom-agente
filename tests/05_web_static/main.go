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

// Task representa o modelo de tarefa
type Task struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Done        bool      `json:"done"`
	CreatedAt   time.Time `json:"created_at"`
}

var (
	tasks = make(map[string]Task)
	mu    sync.Mutex
	nextID int = 1
)

func main() {
	http.HandleFunc("/api/tasks", tasksHandler)
	http.HandleFunc("/api/tasks/", taskByIDHandler)

	fmt.Println("Server listening on port 8080...")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func tasksHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case "GET":
		getTasks(w, r)
	case "POST":
		createTask(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func taskByIDHandler(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Path[len("/api/tasks/"):] // Extrai o ID da URL
	if id == "" {
		http.Error(w, "ID da tarefa não fornecido", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case "GET":
		getTask(w, r, id)
	case "PUT":
		updateTask(w, r, id)
	case "DELETE":
		deleteTask(w, r, id)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// getTasks retorna todas as tarefas
func getTasks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	mu.Lock()
	defer mu.Unlock()
	
	var allTasks []Task
	for _, task := range tasks {
		allTasks = append(allTasks, task)
	}

	json.NewEncoder(w).Encode(allTasks)
}

// createTask cria uma nova tarefa
func createTask(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var task Task
	if err := json.NewDecoder(r.Body).Decode(&task); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	mu.Lock()
	defer mu.Unlock()

	task.ID = strconv.Itoa(nextID)
	nextID++
	task.CreatedAt = time.Now()
	tasks[task.ID] = task

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(task)
}

// getTask busca uma tarefa por ID
func getTask(w http.ResponseWriter, r *http.Request, id string) {
	w.Header().Set("Content-Type", "application/json")
	mu.Lock()
	defer mu.Unlock()

	task, ok := tasks[id]
	if !ok {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}
	json.NewEncoder(w).Encode(task)
}

// updateTask atualiza uma tarefa existente
func updateTask(w http.ResponseWriter, r *http.Request, id string) {
	w.Header().Set("Content-Type", "application/json")
	mu.Lock()
	defer mu.Unlock()

	_, ok := tasks[id]
	if !ok {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	var updatedTask Task
	if err := json.NewDecoder(r.Body).Decode(&updatedTask); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	updatedTask.ID = id // Garante que o ID da tarefa não muda
	tasks[id] = updatedTask

	json.NewEncoder(w).Encode(updatedTask)
}

// deleteTask remove uma tarefa
func deleteTask(w http.ResponseWriter, r *http.Request, id string) {
	w.Header().Set("Content-Type", "application/json")
	mu.Lock()
	defer mu.Unlock()

	_, ok := tasks[id]
	if !ok {
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}

	delete(tasks, id)
	w.WriteHeader(http.StatusNoContent)
}
