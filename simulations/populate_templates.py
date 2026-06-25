import os
from pathlib import Path

BASE_DIR = Path("/home/j/Documentos/GitHub/crom-agente")
SIMS_DIR = BASE_DIR / "simulations"

TEMPLATES = {
    "sim01_greeting": {
        "README.md": "# Simulação 01: Greeting\n\nEste diretório contém a simulação de uma interação inicial simples (saudação) com o crom-agente.\n\n* **Input**: 'Olá'\n* **Fast Path**: Ativado (intercepção instantânea)."
    },
    "sim02_yes_no": {
        "README.md": "# Simulação 02: Sim/Não\n\nEste diretório contém a simulação de uma resposta simples (Sim) de confirmação.\n\n* **Input**: 'Sim'\n* **Fast Path**: Ativado."
    },
    "sim03_report": {
        "relatorio_clima.md": """# Relatório sobre o Clima no Brasil

O Brasil, devido à sua vasta extensão territorial, apresenta uma grande diversidade de climas, variando desde o equatorial úmido na região Norte até o temperado no extremo Sul. A maior parte do país está situada na zona tropical, o que resulta em temperaturas predominantemente quentes e regimes de chuva sazonais bem definidos nas regiões centrais.

Na região Norte, o clima equatorial sob a influência da Floresta Amazônica traz chuvas abundantes durante o ano inteiro e temperaturas médias elevadas. Já no Nordeste, o clima semiárido predomina no sertão, com secas prolongadas e chuvas escassas, contrastando com o litoral úmido e as serras de clima mais ameno.

No Sul e Sudeste, o clima subtropical e o tropical de altitude propiciam invernos mais rigorosos, com ocorrência de geadas e, ocasionalmente, neve em regiões serranas. As estações do ano são mais demarcadas nessas áreas, influenciando diretamente a produção agrícola e o turismo local."""
    },
    "sim04_factorial": {
        "fatorial.py": """def factorial(n):
    if not isinstance(n, int) or n < 0:
        raise ValueError("O fatorial é definido apenas para inteiros não negativos.")
    if n == 0:
        return 1
    result = 1
    for i in range(1, n + 1):
        result *= i
    return result

if __name__ == '__main__':
    number = 5
    result = factorial(number)
    print(f"O fatorial de {number} é: {result}")
"""
    },
    "sim05_json_parser": {
        "dados.json": """{
  "nome": "Crom Agente",
  "versao": "1.2.0",
  "recursos": ["mTLS", "Cognitive Modes", "Intent Routing"],
  "ativo": true
}""",
        "formatador.py": """import json
from pathlib import Path

def format_json_file(filename):
    path = Path(filename)
    if not path.exists():
        print(f"Erro: Arquivo {filename} não encontrado.")
        return
    with open(path) as f:
        data = json.load(f)
    print(json.dumps(data, indent=4, ensure_ascii=False))

if __name__ == '__main__':
    format_json_file("dados.json")
"""
    },
    "sim06_sqlite": {
        "db.py": """import sqlite3

def init_db():
    conn = sqlite3.connect("vendas.db")
    cursor = conn.cursor()
    cursor.execute(\"\"\"
        CREATE TABLE IF NOT EXISTS produtos (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            nome TEXT NOT NULL,
            preco REAL NOT NULL,
            quantidade INTEGER NOT NULL
        )
    \"\"\")
    
    # Limpa e reinsere para teste
    cursor.execute("DELETE FROM produtos")
    produtos = [
        ("Notebook Crom", 4500.0, 10),
        ("Mouse Gamer", 150.0, 50),
        ("Teclado Mecânico", 350.0, 30)
    ]
    cursor.executemany("INSERT INTO produtos (nome, preco, quantidade) VALUES (?, ?, ?)", produtos)
    conn.commit()
    
    cursor.execute("SELECT * FROM produtos")
    for r in cursor.fetchall():
        print(f"ID: {r[0]} | Nome: {r[1]} | Preço: R${r[2]:.2f} | Quantidade: {r[3]}")
    
    conn.close()

if __name__ == '__main__':
    init_db()
"""
    },
    "sim07_portfolio": {
        "index.html": """<!DOCTYPE html>
<html lang="pt-BR">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Portfólio Profissional</title>
    <style>
        body {
            font-family: 'Outfit', 'Inter', sans-serif;
            background-color: #0d0f12;
            color: #c9d1d9;
            margin: 0;
            padding: 40px 20px;
            display: flex;
            justify-content: center;
        }
        .container {
            max-width: 600px;
            width: 100%;
        }
        h1 {
            color: #58a6ff;
            font-size: 2.5em;
            margin-bottom: 5px;
        }
        p.subtitle {
            color: #8b949e;
            font-size: 1.1em;
            margin-top: 0;
        }
        .section {
            margin-top: 30px;
            padding-top: 20px;
            border-top: 1px solid #21262d;
        }
        h2 {
            color: #388bfd;
            font-size: 1.5em;
        }
        .project-card {
            background-color: #161b22;
            border: 1px solid #30363d;
            border-radius: 6px;
            padding: 15px;
            margin-bottom: 15px;
        }
        .project-card h3 {
            margin-top: 0;
            color: #f0f6fc;
        }
    </style>
</head>
<body>
    <div class="container">
        <h1>Desenvolvedor de Elite</h1>
        <p class="subtitle">Especialista em Sistemas Distribuídos e Agentes Inteligentes</p>
        
        <div class="section">
            <h2>Sobre Mim</h2>
            <p>Construindo ferramentas de automação de código de alta performance, utilizando Go, Python e arquiteturas de multi-agentes autônomos.</p>
        </div>
        
        <div class="section">
            <h2>Projetos Recentes</h2>
            <div class="project-card">
                <h3>crom-agente</h3>
                <p>Um orquestrador de agentes local extremamente otimizado, com injeção em tempo real e controle dinâmico de estados cognitivos.</p>
            </div>
            <div class="project-card">
                <h3>Fast Path Intent Router</h3>
                <p>Algoritmo de desvio inteligente para mensagens curtas, reduzindo custos de API e latência para sub-segundos.</p>
            </div>
        </div>
    </div>
</body>
</html>"""
    },
    "sim08_go_server": {
        "server.go": """package main

import (
	"encoding/json"
	"log"
	"net/http"
)

type Response struct {
	Message string `json:"message"`
}

func helloHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	res := Response{Message: "Hello, Crom!"}
	json.NewEncoder(w).Encode(res)
}

func main() {
	http.HandleFunc("/", helloHandler)
	log.Println("Servidor iniciado em http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal(err)
	}
}
"""
    },
    "sim09_file_organizer": {
        "organizer.py": """import os
import shutil
from pathlib import Path

def organize_folder(folder_path="."):
    base = Path(folder_path)
    txt_dir = base / "textos"
    json_dir = base / "dados"
    
    # Garante os diretórios de destino
    txt_dir.mkdir(exist_ok=True)
    json_dir.mkdir(exist_ok=True)
    
    for item in base.iterdir():
        if item.is_file():
            if item.suffix == ".txt":
                shutil.move(str(item), str(txt_dir / item.name))
                print(f"Movido: {item.name} -> textos/")
            elif item.suffix == ".json" and item.name != ".crom_state.json" and item.name != "config.json":
                shutil.move(str(item), str(json_dir / item.name))
                print(f"Movido: {item.name} -> dados/")

if __name__ == '__main__':
    # Cria arquivos dummy para demonstração se não existirem
    with open("dummy1.txt", "w") as f: f.write("Dummy text")
    with open("dummy2.json", "w") as f: f.write('{"dummy": true}')
    organize_folder()
"""
    },
    "sim10_yii2_sqlite": {
        "config/db.php": """<?php
return [
    'class' => 'yii\\db\\Connection',
    'dsn' => 'sqlite:@app/db/vendas.db',
    'charset' => 'utf8',
];
""",
        "models/Item.php": """<?php
namespace app\\models;

use yii\\db\\ActiveRecord;

/**
 * Model ActiveRecord para a tabela 'items' do SQLite
 */
class Item extends ActiveRecord
{
    public static function tableName()
    {
        return 'items';
    }

    public function rules()
    {
        return [
            [['nome', 'preco'], 'required'],
            [['preco'], 'number'],
            [['quantidade'], 'integer'],
            [['nome'], 'string', 'max' => 255],
        ];
    }
}
""",
        "controllers/ItemController.php": """<?php
namespace app\\controllers;

use yii\\web\\Controller;
use app\\models\\Item;
use Yii;

/**
 * Controller MVC Yii2 para operações de CRUD de Itens
 */
class ItemController extends Controller
{
    public function actionIndex()
    {
        $items = Item::find()->all();
        return $this->render('index', [
            'items' => $items,
        ]);
    }

    public function actionCreate()
    {
        $model = new Item();
        if ($model->load(Yii::$app->request->post()) && $model->save()) {
            return $this->redirect(['index']);
        }
        return $this->render('create', [
            'model' => $model,
        ]);
    }
}
"""
    }
}

def main():
    for sim_name, files in TEMPLATES.items():
        sim_dir = SIMS_DIR / sim_name
        sim_dir.mkdir(parents=True, exist_ok=True)
        for filepath, content in files.items():
            full_path = sim_dir / filepath
            full_path.parent.mkdir(parents=True, exist_ok=True)
            with open(full_path, "w") as f:
                f.write(content)
            print(f"Escrito template: {sim_name}/{filepath}")

if __name__ == '__main__':
    main()
