package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFS_SandboxViolation(t *testing.T) {
	ws := t.TempDir()

	outsideFile := filepath.Join(filepath.Dir(ws), "outside_secret.txt")
	os.WriteFile(outsideFile, []byte("secret"), 0644)
	defer os.Remove(outsideFile)

	ctx := context.Background()

	readTool := NewReadFileTool(ws, true)
	argsRead, _ := json.Marshal(map[string]interface{}{"path": "../outside_secret.txt"})
	resRead, _ := readTool.Execute(ctx, argsRead)
	if resRead.Success {
		t.Error("expected ReadFile to fail on path traversal")
	} else if !strings.Contains(resRead.Error, "fora do sandbox") {
		t.Errorf("expected sandbox error, got %v", resRead.Error)
	}

	writeTool := NewWriteFileTool(ws, true)
	argsWrite, _ := json.Marshal(map[string]interface{}{
		"path":    "../outside_secret.txt",
		"content": "hacked",
	})
	resWrite, _ := writeTool.Execute(ctx, argsWrite)
	if resWrite.Success {
		t.Error("expected WriteFile to fail on path traversal")
	}

	deleteTool := NewDeleteFileTool(ws, true)
	argsDelete, _ := json.Marshal(map[string]interface{}{
		"path": "../outside_secret.txt",
	})
	resDelete, _ := deleteTool.Execute(ctx, argsDelete)
	if resDelete.Success {
		t.Error("expected DeleteFile to fail on path traversal")
	}
}

func TestFS_ValidOperations(t *testing.T) {
	ws := t.TempDir()
	ctx := context.Background()

	writeTool := NewWriteFileTool(ws, true)
	argsWrite, _ := json.Marshal(map[string]interface{}{
		"path":    "test.txt",
		"content": "hello world",
	})
	resWrite, _ := writeTool.Execute(ctx, argsWrite)
	if !resWrite.Success {
		t.Fatalf("WriteFile failed: %v", resWrite.Error)
	}

	readTool := NewReadFileTool(ws, true)
	argsRead, _ := json.Marshal(map[string]interface{}{
		"path": "test.txt",
	})
	resRead, _ := readTool.Execute(ctx, argsRead)
	if !resRead.Success {
		t.Fatalf("ReadFile failed: %v", resRead.Error)
	}
	if resRead.Data != "hello world" {
		t.Errorf("Expected 'hello world', got '%s'", resRead.Data)
	}

	renameTool := NewRenameFileTool(ws, true)
	argsRename, _ := json.Marshal(map[string]interface{}{
		"src_path":  "test.txt",
		"dest_path": "renamed.txt",
	})
	resRename, _ := renameTool.Execute(ctx, argsRename)
	if !resRename.Success {
		t.Fatalf("RenameFile failed: %v", resRename.Error)
	}

	argsReadRenamed, _ := json.Marshal(map[string]interface{}{
		"path": "renamed.txt",
	})
	resReadRenamed, _ := readTool.Execute(ctx, argsReadRenamed)
	if !resReadRenamed.Success {
		t.Fatalf("ReadFile on renamed file failed: %v", resReadRenamed.Error)
	}

	deleteTool := NewDeleteFileTool(ws, true)
	argsDelete, _ := json.Marshal(map[string]interface{}{
		"path": "renamed.txt",
	})
	resDelete, _ := deleteTool.Execute(ctx, argsDelete)
	if !resDelete.Success {
		t.Fatalf("DeleteFile failed: %v", resDelete.Error)
	}
}
