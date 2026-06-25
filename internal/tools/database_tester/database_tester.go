package database_tester

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/crom/crom-agente/internal/tools"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	_ "modernc.org/sqlite"
)

//go:embed metadata.json
var metadataJSON []byte

var metadata tools.ToolMetadata

func init() {
	var err error
	metadata, err = tools.ParseMetadata(metadataJSON)
	if err != nil {
		panic("falha ao carregar metadados de database_tester: " + err.Error())
	}
}

// DatabaseTesterTool testa credenciais e conexões com bancos populares (Postgres, MySQL, SQLite, MongoDB)
type DatabaseTesterTool struct {
	workspaceRoot string
}

// NewDatabaseTesterTool cria a ferramenta database_tester
func NewDatabaseTesterTool(workspaceRoot string) *DatabaseTesterTool {
	return &DatabaseTesterTool{workspaceRoot: workspaceRoot}
}

func (t *DatabaseTesterTool) ID() string { return metadata.ID }

func (t *DatabaseTesterTool) Description() string {
	return metadata.Description
}

func (t *DatabaseTesterTool) ParametersSchema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"type": {
				"type": "string",
				"enum": ["postgres", "mysql", "sqlite", "mongodb"],
				"description": "Tipo de banco de dados"
			},
			"dsn": {
				"type": "string",
				"description": "String de conexão (DSN / URI / Caminho do arquivo)"
			}
		},
		"required": ["type", "dsn"]
	}`)
}

func (t *DatabaseTesterTool) RequiresApproval() bool { return false }

func (t *DatabaseTesterTool) Execute(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	var input struct {
		Type string `json:"type"`
		DSN  string `json:"dsn"`
	}
	if err := json.Unmarshal(args, &input); err != nil {
		return tools.Result{Success: false, Error: "argumentos inválidos: " + err.Error()}, nil
	}

	dbType := strings.ToLower(input.Type)
	dsn := strings.TrimSpace(input.DSN)
	if dsn == "" {
		return tools.Result{Success: false, Error: "string de conexão (dsn) não pode ser vazia"}, nil
	}

	// Timeout de conexão: 5 segundos
	connectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	switch dbType {
	case "postgres":
		return t.testSQL(connectCtx, "postgres", dsn)
	case "mysql":
		return t.testSQL(connectCtx, "mysql", dsn)
	case "sqlite":
		// Resolver caminho com sandbox jail
		absPath, err := tools.ValidatePath(t.workspaceRoot, dsn, true)
		if err != nil {
			return tools.Result{Success: false, Error: "sqlite: " + err.Error()}, nil
		}
		return t.testSQLite(connectCtx, absPath)
	case "mongodb":
		return t.testMongoDB(connectCtx, dsn)
	default:
		return tools.Result{Success: false, Error: fmt.Sprintf("tipo de banco '%s' não suportado", dbType)}, nil
	}
}

func (t *DatabaseTesterTool) testSQL(ctx context.Context, driver, dsn string) (tools.Result, error) {
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("falha ao inicializar driver %s: %v", driver, err)}, nil
	}
	defer db.Close()

	// Ping com contexto de timeout
	errChan := make(chan error, 1)
	go func() {
		errChan <- db.Ping()
	}()

	select {
	case <-ctx.Done():
		return tools.Result{Success: false, Error: "timeout ao tentar conectar ao banco de dados SQL"}, nil
	case err := <-errChan:
		if err != nil {
			return tools.Result{Success: false, Error: fmt.Sprintf("falha na conexão/autenticação: %v", err)}, nil
		}
	}

	return tools.Result{Success: true, Data: fmt.Sprintf("Conexão com %s bem-sucedida!", driver)}, nil
}

func (t *DatabaseTesterTool) testSQLite(ctx context.Context, path string) (tools.Result, error) {
	// Verificar se arquivo é uma pasta
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		return tools.Result{Success: false, Error: "o caminho especificado é um diretório, não um arquivo SQLite"}, nil
	}

	// Testa abrindo usando o driver modernc.org/sqlite
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("falha ao carregar driver sqlite: %v", err)}, nil
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("falha ao acessar arquivo SQLite: %v", err)}, nil
	}

	// Executa uma query simples para validar
	var val int
	err = db.QueryRowContext(ctx, "SELECT 1").Scan(&val)
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("falha ao executar query de teste: %v", err)}, nil
	}

	return tools.Result{
		Success: true,
		Data:    fmt.Sprintf("Conexão SQLite estabelecida com sucesso. Arquivo: %s", path),
	}, nil
}

func (t *DatabaseTesterTool) testMongoDB(ctx context.Context, uri string) (tools.Result, error) {
	clientOpts := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		return tools.Result{Success: false, Error: fmt.Sprintf("falha ao configurar driver MongoDB: %v", err)}, nil
	}
	defer func() {
		_ = client.Disconnect(ctx)
	}()

	// Ping no admin db para verificar autenticação e conexão
	errChan := make(chan error, 1)
	go func() {
		errChan <- client.Ping(ctx, nil)
	}()

	select {
	case <-ctx.Done():
		return tools.Result{Success: false, Error: "timeout ao tentar conectar ao MongoDB"}, nil
	case err := <-errChan:
		if err != nil {
			return tools.Result{Success: false, Error: fmt.Sprintf("falha na conexão/autenticação MongoDB: %v", err)}, nil
		}
	}

	// Extrair informações do host da URI para omitir credenciais no retorno
	hostStr := "MongoDB Server"
	if u, err := url.Parse(uri); err == nil {
		hostStr = u.Host
	}

	return tools.Result{
		Success: true,
		Data:    fmt.Sprintf("Conexão MongoDB bem-sucedida com o host: %s", hostStr),
	}, nil
}
