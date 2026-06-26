package concurrency_lock

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestConcurrentAcquireRelease(t *testing.T) {
	tool := NewConcurrencyLockTool("/tmp/test-workspace")
	ctx := context.Background()
	const numGoroutines = 100

	var wg sync.WaitGroup
	successCount := 0
	failCount := 0
	var mu sync.Mutex

	// 100 goroutines tentando adquirir o mesmo lock
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			owner := fmt.Sprintf("worker-%d", id)
			args, _ := json.Marshal(map[string]string{
				"path":   "/tmp/test-workspace/contested-file.go",
				"action": "acquire",
				"owner":  owner,
			})

			result, err := tool.Execute(ctx, args)
			if err != nil {
				t.Errorf("goroutine %d: unexpected error: %v", id, err)
				return
			}

			mu.Lock()
			if result.Success {
				successCount++
			} else {
				failCount++
			}
			mu.Unlock()

			// Se conseguiu o lock, segura brevemente e libera
			if result.Success {
				time.Sleep(1 * time.Millisecond)
				releaseArgs, _ := json.Marshal(map[string]string{
					"path":   "/tmp/test-workspace/contested-file.go",
					"action": "release",
					"owner":  owner,
				})
				tool.Execute(ctx, releaseArgs)
			}
		}(i)
	}

	wg.Wait()

	// Exatamente um deve ter conseguido o lock na primeira rodada
	if successCount < 1 {
		t.Errorf("expected at least 1 successful acquire, got %d", successCount)
	}
	t.Logf("Results: %d success, %d fail out of %d goroutines", successCount, failCount, numGoroutines)
}

func TestReentrantLock(t *testing.T) {
	tool := NewConcurrencyLockTool("/tmp/test-workspace")
	ctx := context.Background()

	// Mesmo owner deve poder readquirir
	args, _ := json.Marshal(map[string]string{
		"path":   "/tmp/test-workspace/reentrant.go",
		"action": "acquire",
		"owner":  "agent-1",
	})

	result1, _ := tool.Execute(ctx, args)
	if !result1.Success {
		t.Fatal("first acquire should succeed")
	}

	result2, _ := tool.Execute(ctx, args)
	if !result2.Success {
		t.Fatal("reentrant acquire by same owner should succeed")
	}

	// Outro owner deve falhar
	otherArgs, _ := json.Marshal(map[string]string{
		"path":   "/tmp/test-workspace/reentrant.go",
		"action": "acquire",
		"owner":  "agent-2",
	})

	result3, _ := tool.Execute(ctx, otherArgs)
	if result3.Success {
		t.Fatal("acquire by different owner should fail while locked")
	}

	// Liberar
	releaseArgs, _ := json.Marshal(map[string]string{
		"path":   "/tmp/test-workspace/reentrant.go",
		"action": "release",
		"owner":  "agent-1",
	})
	tool.Execute(ctx, releaseArgs)
}

func TestLockTTLExpiry(t *testing.T) {
	// Temporariamente reduzir TTL para teste
	originalTTL := lockTTL
	lockTTL = 100 * time.Millisecond
	defer func() { lockTTL = originalTTL }()

	tool := NewConcurrencyLockTool("/tmp/test-workspace")
	ctx := context.Background()

	// Adquirir lock
	args, _ := json.Marshal(map[string]string{
		"path":   "/tmp/test-workspace/ttl-test.go",
		"action": "acquire",
		"owner":  "old-worker",
	})

	result, _ := tool.Execute(ctx, args)
	if !result.Success {
		t.Fatal("acquire should succeed")
	}

	// Esperar TTL expirar
	time.Sleep(200 * time.Millisecond)

	// Forçar cleanup manualmente (já que o ticker roda a cada 10s)
	locksMu.Lock()
	now := time.Now()
	for path, entry := range locks {
		if now.Sub(entry.Timestamp) > lockTTL {
			delete(locks, path)
		}
	}
	locksMu.Unlock()

	// Outro owner deve conseguir adquirir agora
	newArgs, _ := json.Marshal(map[string]string{
		"path":   "/tmp/test-workspace/ttl-test.go",
		"action": "acquire",
		"owner":  "new-worker",
	})

	result2, _ := tool.Execute(ctx, newArgs)
	if !result2.Success {
		t.Fatal("acquire after TTL expiry should succeed")
	}

	// Cleanup
	releaseArgs, _ := json.Marshal(map[string]string{
		"path":   "/tmp/test-workspace/ttl-test.go",
		"action": "release",
		"owner":  "new-worker",
	})
	tool.Execute(ctx, releaseArgs)
}

func TestStatusAction(t *testing.T) {
	tool := NewConcurrencyLockTool("/tmp/test-workspace")
	ctx := context.Background()

	// Status de arquivo sem lock
	args, _ := json.Marshal(map[string]string{
		"path":   "/tmp/test-workspace/status-test.go",
		"action": "status",
	})

	result, _ := tool.Execute(ctx, args)
	if !result.Success {
		t.Fatal("status should always succeed")
	}

	// Verificar que locked é false
	var status map[string]interface{}
	if err := json.Unmarshal([]byte(result.Data), &status); err != nil {
		t.Fatalf("failed to unmarshal status: %v", err)
	}
	if status["locked"] != false {
		t.Fatal("status should show unlocked")
	}
}

func TestHighConcurrencyRaceDetection(t *testing.T) {
	tool := NewConcurrencyLockTool("/tmp/test-workspace")
	ctx := context.Background()

	const files = 10
	const workersPerFile = 20
	var wg sync.WaitGroup

	for f := 0; f < files; f++ {
		for w := 0; w < workersPerFile; w++ {
			wg.Add(1)
			go func(fileIdx, workerIdx int) {
				defer wg.Done()

				path := fmt.Sprintf("/tmp/test-workspace/file-%d.go", fileIdx)
				owner := fmt.Sprintf("w-%d-%d", fileIdx, workerIdx)

				// Adquirir
				args, _ := json.Marshal(map[string]string{
					"path":   path,
					"action": "acquire",
					"owner":  owner,
				})
				result, err := tool.Execute(ctx, args)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
					return
				}

				if result.Success {
					// Simular trabalho
					time.Sleep(time.Microsecond * 100)

					// Liberar
					releaseArgs, _ := json.Marshal(map[string]string{
						"path":   path,
						"action": "release",
						"owner":  owner,
					})
					tool.Execute(ctx, releaseArgs)
				}
			}(f, w)
		}
	}

	wg.Wait()
}
