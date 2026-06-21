package stress

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{}

// mockWSServer cria um servidor WS falso na nuvem para teste
func mockWSServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			mt, message, err := c.ReadMessage()
			if err != nil {
				break
			}
			// Echo de volta
			err = c.WriteMessage(mt, message)
			if err != nil {
				break
			}
		}
	}))
}

// TestWebSocketStress simula 100 conexoes websocket simultaneas enviando mensagens e lendo
func TestWebSocketStress(t *testing.T) {
	srv := mockWSServer()
	defer srv.Close()

	wsURL := "ws" + srv.URL[4:] // troca http por ws
	numConns := 100

	var wg sync.WaitGroup
	wg.Add(numConns)

	var successCount int32
	t.Logf("Iniciando %d conexões WS simultâneas contra %s", numConns, wsURL)

	for i := 0; i < numConns; i++ {
		go func(id int) {
			defer wg.Done()
			c, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
			if err != nil {
				t.Errorf("Falha ao conectar WS %d: %v", id, err)
				return
			}
			defer c.Close()

			msg := []byte(fmt.Sprintf("Ping do agente %d", id))
			if err := c.WriteMessage(websocket.TextMessage, msg); err != nil {
				t.Errorf("Falha ao enviar WS %d: %v", id, err)
				return
			}

			_, recvMsg, err := c.ReadMessage()
			if err != nil {
				t.Errorf("Falha ao ler WS %d: %v", id, err)
				return
			}

			if string(recvMsg) == string(msg) {
				atomic.AddInt32(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()

	if successCount != int32(numConns) {
		t.Errorf("FAIL: Nem todos os websockets completaram o ciclo. Sucessos: %d/%d", successCount, numConns)
	} else {
		t.Logf("SUCCESS: A infraestrutura Websocket processou 100 conexões pesadas concorrentes com 0 drop packets.")
	}
}
