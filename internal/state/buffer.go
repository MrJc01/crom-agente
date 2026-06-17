package state

import (
	"fmt"
	"strings"
	"sync"
)

// BufferUpdate representa uma atualização/patch de conteúdo enviada ao buffer ativo
type BufferUpdate struct {
	Path    string `json:"path"`
	Content string `json:"content"` // Conteúdo completo ou alterado
	Version int    `json:"version"`
}

// ActiveBuffer representa o estado em memória de um arquivo aberto/editado
type ActiveBuffer struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Version int    `json:"version"`
}

// BufferManager coordena a sincronização bidirecional do buffer ativo
type BufferManager struct {
	mu          sync.RWMutex
	buffers     map[string]*ActiveBuffer
	subscribers []chan BufferUpdate
}

// NewBufferManager cria uma nova instância do BufferManager
func NewBufferManager() *BufferManager {
	return &BufferManager{
		buffers: make(map[string]*ActiveBuffer),
	}
}

// UpdateBuffer atualiza o buffer em memória e notifica todos os inscritos
func (m *BufferManager) UpdateBuffer(path string, content string, version int) {
	m.mu.Lock()
	buf, ok := m.buffers[path]
	if !ok {
		buf = &ActiveBuffer{Path: path}
		m.buffers[path] = buf
	}

	// Evita retrocesso de versão
	if version >= buf.Version {
		buf.Content = content
		buf.Version = version
	}
	m.mu.Unlock()

	// Notifica inscritos de forma não-bloqueante
	update := BufferUpdate{Path: path, Content: content, Version: version}
	m.mu.RLock()
	for _, sub := range m.subscribers {
		select {
		case sub <- update:
		default:
			// Canal cheio, descarta ou ignora para não travar
		}
	}
	m.mu.RUnlock()
}

// GetBuffer retorna uma cópia do buffer ativo para leitura
func (m *BufferManager) GetBuffer(path string) (ActiveBuffer, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	buf, ok := m.buffers[path]
	if !ok {
		return ActiveBuffer{}, false
	}
	return *buf, true
}

// Subscribe cria um canal de leitura que recebe atualizações em tempo real
func (m *BufferManager) Subscribe() chan BufferUpdate {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch := make(chan BufferUpdate, 50)
	m.subscribers = append(m.subscribers, ch)
	return ch
}

// Unsubscribe remove o canal de inscrições
func (m *BufferManager) Unsubscribe(ch chan BufferUpdate) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, sub := range m.subscribers {
		if sub == ch {
			close(ch)
			m.subscribers = append(m.subscribers[:i], m.subscribers[i+1:]...)
			return
		}
	}
}

// ApplyPatch simula aplicação de deltas simples de busca/substituição no buffer
func (m *BufferManager) ApplyPatch(path string, target string, replacement string) error {
	m.mu.Lock()
	buf, ok := m.buffers[path]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("buffer para o arquivo '%s' não encontrado", path)
	}

	content := buf.Content
	count := strings.Count(content, target)
	if count == 0 {
		m.mu.Unlock()
		return fmt.Errorf("alvo do patch não encontrado no buffer")
	}
	if count > 1 {
		m.mu.Unlock()
		return fmt.Errorf("patch ambíguo: múltiplas ocorrências encontradas")
	}

	newContent := strings.Replace(content, target, replacement, 1)
	buf.Content = newContent
	buf.Version++
	m.mu.Unlock()

	// Notifica inscritos
	update := BufferUpdate{Path: path, Content: newContent, Version: buf.Version}
	m.mu.RLock()
	for _, sub := range m.subscribers {
		select {
		case sub <- update:
		default:
		}
	}
	m.mu.RUnlock()

	return nil
}
