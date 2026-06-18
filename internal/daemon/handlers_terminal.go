package daemon

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

type terminalListener struct {
	ch   chan []byte
	once sync.Once
}

// TerminalSession holds interactive PTY details
type TerminalSession struct {
	ID        string
	Cmd       *exec.Cmd
	PTY       *os.File
	buffer    []byte
	mu        sync.Mutex
	listeners map[*terminalListener]bool
	closed    bool
}

func NewTerminalSession(id string) (*TerminalSession, error) {
	cmd := exec.Command("bash")
	homeDir, _ := os.UserHomeDir()
	if homeDir != "" {
		cmd.Dir = homeDir
	}
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	f, err := pty.Start(cmd)
	if err != nil {
		cmd = exec.Command("sh")
		if homeDir != "" {
			cmd.Dir = homeDir
		}
		cmd.Env = append(os.Environ(), "TERM=xterm-256color")
		f, err = pty.Start(cmd)
		if err != nil {
			return nil, err
		}
	}

	session := &TerminalSession{
		ID:        id,
		Cmd:       cmd,
		PTY:       f,
		listeners: make(map[*terminalListener]bool),
	}

	go func() {
		defer session.Close()
		buf := make([]byte, 2048)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				session.mu.Lock()
				session.buffer = append(session.buffer, chunk...)
				if len(session.buffer) > 50000 {
					session.buffer = session.buffer[len(session.buffer)-50000:]
				}
				for listener := range session.listeners {
					select {
					case listener.ch <- chunk:
					default:
					}
				}
				session.mu.Unlock()
			}
			if err != nil {
				break
			}
		}
	}()

	return session, nil
}

func (s *TerminalSession) Write(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return fmt.Errorf("terminal session closed")
	}
	_, err := s.PTY.Write(data)
	return err
}

func (s *TerminalSession) Close() {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.mu.Unlock()

	_ = s.PTY.Close()
	if s.Cmd.Process != nil {
		_ = s.Cmd.Process.Kill()
	}

	s.mu.Lock()
	for listener := range s.listeners {
		listener.once.Do(func() {
			close(listener.ch)
		})
	}
	s.listeners = make(map[*terminalListener]bool)
	s.mu.Unlock()
}

func (s *APIServer) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[APIServer] websocket upgrade error: %v", err)
		return
	}
	defer conn.Close()

	termID := r.URL.Query().Get("id")
	if termID == "" {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\nErro: Terminal ID obrigatorio\r\n"))
		return
	}

	s.terminalSessionsMu.Lock()
	session, exists := s.terminalSessions[termID]
	if !exists || session.closed {
		var err error
		session, err = NewTerminalSession(termID)
		if err != nil {
			s.terminalSessionsMu.Unlock()
			_ = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("\r\nErro ao criar sessao de terminal: %v\r\n", err)))
			return
		}
		s.terminalSessions[termID] = session
	}
	s.terminalSessionsMu.Unlock()

	outChan := make(chan []byte, 100)
	listener := &terminalListener{ch: outChan}
	session.mu.Lock()
	session.listeners[listener] = true
	history := make([]byte, len(session.buffer))
	copy(history, session.buffer)
	session.mu.Unlock()

	if len(history) > 0 {
		_ = conn.WriteMessage(websocket.BinaryMessage, history)
	} else {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("\r\n--- Terminal Crom Agent Iniciado ---\r\n"))
	}

	doneChan := make(chan struct{})
	go func() {
		defer close(doneChan)
		for data := range outChan {
			err := conn.WriteMessage(websocket.BinaryMessage, data)
			if err != nil {
				return
			}
		}
	}()

	for {
		mt, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if mt == websocket.TextMessage || mt == websocket.BinaryMessage {
			_ = session.Write(msg)
		}
	}

	session.mu.Lock()
	delete(session.listeners, listener)
	session.mu.Unlock()
	
	listener.once.Do(func() {
		close(outChan)
	})

	<-doneChan
}

func (s *APIServer) handleTerminalList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-Token")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if !s.authorize(w, r) {
		return
	}

	s.terminalSessionsMu.Lock()
	ids := []string{}
	for id, sess := range s.terminalSessions {
		if !sess.closed {
			ids = append(ids, id)
		}
	}
	s.terminalSessionsMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ids)
}

func (s *APIServer) handleTerminalClose(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-Token")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}
	if !s.authorize(w, r) {
		return
	}

	termID := r.URL.Query().Get("id")
	if termID == "" {
		http.Error(w, "id obrigatorio", http.StatusBadRequest)
		return
	}

	s.terminalSessionsMu.Lock()
	session, exists := s.terminalSessions[termID]
	if exists {
		session.Close()
		delete(s.terminalSessions, termID)
	}
	s.terminalSessionsMu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"success":true}`))
}
