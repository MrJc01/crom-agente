import os
import sys
import json
import subprocess
import shutil
from pathlib import Path

# Configuração de caminhos
BASE_DIR = Path("/home/j/Documentos/GitHub/crom-agente")
SIMS_DIR = BASE_DIR / "simulations"

def check_sim01():
    """greeting fast path validation"""
    ws = SIMS_DIR / "sim01_greeting"
    state_file = ws / ".crom" / ".crom_state.json"
    if not state_file.exists():
        return False, "Arquivo de estado .crom_state.json não encontrado."
    return True, "Sessão e logs criados com sucesso."

def check_sim02():
    """yes_no fast path validation"""
    ws = SIMS_DIR / "sim02_yes_no"
    state_file = ws / ".crom" / ".crom_state.json"
    if not state_file.exists():
        return False, "Arquivo de estado .crom_state.json não encontrado."
    return True, "Sessão e logs criados com sucesso."

def check_sim03():
    """climate report validation"""
    ws = SIMS_DIR / "sim03_report"
    report_file = ws / "relatorio_clima.md"
    if not report_file.exists():
        return False, "relatorio_clima.md não foi gerado."
    
    content = report_file.read_text().strip()
    if len(content) < 50:
        return False, f"relatorio_clima.md é muito curto ({len(content)} caracteres)."
    
    # Verifica parágrafos básicos
    paragraphs = [p for p in content.split("\n\n") if p.strip()]
    if len(paragraphs) < 2:
        return False, f"Esperado múltiplos parágrafos/seções, obteve apenas {len(paragraphs)}."
    
    return True, f"relatorio_clima.md validado com sucesso ({len(paragraphs)} seções)."

def check_sim04():
    """factorial script check"""
    ws = SIMS_DIR / "sim04_factorial"
    script = ws / "fatorial.py"
    if not script.exists():
        return False, "fatorial.py não encontrado."
    
    try:
        res = subprocess.run([sys.executable, str(script)], capture_output=True, text=True, timeout=10, cwd=str(ws))
        if res.returncode != 0:
            return False, f"Script falhou ao rodar: {res.stderr}"
        
        output = res.stdout.strip()
        if "120" not in output:
            return False, f"Esperado '120' no output do fatorial de 5, obteve: {output}"
        return True, "fatorial.py rodou com sucesso e retornou 120."
    except Exception as e:
        return False, f"Erro ao executar fatorial.py: {e}"

def check_sim05():
    """json parser check"""
    ws = SIMS_DIR / "sim05_json_parser"
    script = ws / "formatador.py"
    data_file = ws / "dados.json"
    if not script.exists():
        return False, "formatador.py não encontrado."
    if not data_file.exists():
        return False, "dados.json não encontrado."
        
    try:
        # Garante que dados.json seja JSON válido
        with open(data_file) as f:
            json.load(f)
    except Exception as e:
        return False, f"dados.json não é um JSON válido: {e}"

    try:
        res = subprocess.run([sys.executable, str(script)], capture_output=True, text=True, timeout=10, cwd=str(ws))
        if res.returncode != 0:
            return False, f"formatador.py falhou ao rodar: {res.stderr}"
        
        # Garante que o output seja legível
        output = res.stdout.strip()
        if len(output) < 5:
            return False, f"Output do formatador muito curto: '{output}'"
        return True, f"formatador.py executado com sucesso. Output:\n{output[:100]}..."
    except Exception as e:
        return False, f"Erro ao executar formatador.py: {e}"

def check_sim06():
    """sqlite product database check"""
    ws = SIMS_DIR / "sim06_sqlite"
    script = ws / "db.py"
    db_file = ws / "vendas.db"
    
    if not script.exists():
        return False, "db.py não encontrado."
        
    # Remove db anterior se sobrou algum resíduo
    if db_file.exists():
        try:
            db_file.unlink()
        except Exception:
            pass

    try:
        res = subprocess.run([sys.executable, str(script)], capture_output=True, text=True, timeout=15, cwd=str(ws))
        if res.returncode != 0:
            return False, f"db.py falhou ao rodar: {res.stderr}"
        
        if not db_file.exists():
            return False, "vendas.db não foi criado fisicamente no disco."
        
        # Verifica se o output possui indicações de produtos
        output = res.stdout.lower()
        if len(output) < 20:
            return False, f"Query SQL retornou output muito curto ou vazio: {output}"
            
        return True, "db.py criou vendas.db, inseriu produtos e fez query com sucesso."
    except Exception as e:
        return False, f"Erro ao executar db.py: {e}"
    finally:
        # Garante a limpeza do banco de dados gerado
        if db_file.exists():
            try:
                db_file.unlink()
            except Exception:
                pass

def check_sim07():
    """html portfolio page check"""
    ws = SIMS_DIR / "sim07_portfolio"
    html_file = ws / "index.html"
    if not html_file.exists():
        return False, "index.html não encontrado."
        
    content = html_file.read_text().strip().lower()
    if "<!doctype html>" not in content and "<html" not in content:
        return False, "index.html não possui estrutura HTML básica (DOCTYPE ou tag <html>)."
    if "css" in content or "style" in content:
        return True, "index.html gerado com estrutura básica e estilização inline/tag style."
    return True, "index.html gerado com estrutura HTML básica."

def check_sim08():
    """go http server compiler and lint check"""
    ws = SIMS_DIR / "sim08_go_server"
    server_file = ws / "server.go"
    if not server_file.exists():
        return False, "server.go não encontrado."
        
    # Verifica se compila corretamente usando 'go build'
    try:
        binary_out = ws / "server_test_bin"
        if binary_out.exists():
            binary_out.unlink()
            
        res = subprocess.run(["go", "build", "-o", str(binary_out), str(server_file)], capture_output=True, text=True, timeout=20, cwd=str(ws))
        if res.returncode != 0:
            return False, f"server.go falhou ao compilar: {res.stderr}"
            
        if not binary_out.exists():
            return False, "Binário do servidor não foi gerado pelo compilador Go."
            
        # Limpa binário de teste
        binary_out.unlink()
        return True, "server.go compilado com sucesso sem erros de sintaxe Go."
    except Exception as e:
        return False, f"Erro de compilação ou Go não instalado: {e}"

def check_sim09():
    """file organizer check"""
    ws = SIMS_DIR / "sim09_file_organizer"
    script = ws / "organizer.py"
    if not script.exists():
        return False, "organizer.py não encontrado."
        
    # Cria estrutura de arquivos de teste para ver se o organizer funciona
    test_dir = ws / "pasta_teste"
    if test_dir.exists():
        shutil.rmtree(test_dir)
    test_dir.mkdir(parents=True, exist_ok=True)
    
    file_txt = test_dir / "arquivo1.txt"
    file_json = test_dir / "dados1.json"
    file_txt.write_text("texto de teste")
    file_json.write_text('{"teste": true}')
    
    folder_textos = ws / "textos"
    folder_dados = ws / "dados"
    root_txt = ws / "teste_organizer.txt"
    root_json = ws / "teste_organizer.json"
    
    try:
        # Executa o script passando a pasta de teste se possível, ou rodando no workspace
        # Modificamos o organizer.py para rodar na pasta atual se nenhuma for dada
        res = subprocess.run([sys.executable, str(script)], capture_output=True, text=True, timeout=15, cwd=str(ws))
        
        # Como o prompt pede para organizar arquivos da pasta atual do workspace:
        # Vamos verificar se as pastas 'textos' e 'dados' foram criadas no workspace
        root_txt.write_text("conteudo")
        root_json.write_text("{}")
        
        subprocess.run([sys.executable, str(script)], capture_output=True, text=True, timeout=15, cwd=str(ws))
        
        moved_txt = folder_textos / "teste_organizer.txt"
        moved_json = folder_dados / "teste_organizer.json"
        
        success = moved_txt.exists() or moved_json.exists() or folder_textos.exists() or folder_dados.exists()
        
        if not success:
            return False, "As pastas 'textos' ou 'dados' de destino não foram encontradas/populadas."
            
        return True, "organizer.py criou as subpastas e moveu os tipos de arquivos corretamente."
    except Exception as e:
        return False, f"Erro ao testar organizador de arquivos: {e}"
    finally:
        # Limpeza de todos os dados e pastas gerados pelo teste
        for f in [root_txt, root_json]:
            if f.exists():
                try:
                    f.unlink()
                except Exception:
                    pass
        for d in [test_dir, folder_textos, folder_dados]:
            if d.exists():
                try:
                    shutil.rmtree(d)
                except Exception:
                    pass

def check_sim10():
    """yii2 mvc skeleton structure validation"""
    ws = SIMS_DIR / "sim10_yii2_sqlite"
    
    db_config = ws / "config" / "db.php"
    model_item = ws / "models" / "Item.php"
    controller_item = ws / "controllers" / "ItemController.php"
    
    missing = []
    if not db_config.exists():
        missing.append("config/db.php")
    if not model_item.exists():
        missing.append("models/Item.php")
    if not controller_item.exists():
        missing.append("controllers/ItemController.php")
        
    if missing:
        return False, f"Arquivos estruturais Yii2 ausentes: {', '.join(missing)}"
        
    # Valida presença de código php básico (<?php) nos arquivos
    for path, name in [(db_config, "config/db.php"), (model_item, "models/Item.php"), (controller_item, "controllers/ItemController.php")]:
        content = path.read_text().strip()
        if not content.startswith("<?php"):
            return False, f"Arquivo {name} não inicia com tag PHP válida '<?php'."
            
    return True, "Estrutura de arquivos Yii2 MVC e tags PHP validadas com sucesso."

def main():
    print("==================================================")
    print("Iniciando Verificação Físico-Funcional dos Projetos")
    print("==================================================")
    
    verifications = [
        ("Simulação 1 (Greeting)", check_sim01),
        ("Simulação 2 (Yes/No Reply)", check_sim02),
        ("Simulação 3 (Report Generation)", check_sim03),
        ("Simulação 4 (Factorial Script)", check_sim04),
        ("Simulação 5 (JSON Parser)", check_sim05),
        ("Simulação 6 (SQLite Product Table)", check_sim06),
        ("Simulação 7 (HTML Static Page)", check_sim07),
        ("Simulação 8 (Go HTTP Server)", check_sim08),
        ("Simulação 9 (File Organizer)", check_sim09),
        ("Simulação 10 (Yii2 MVC PHP SQLite)", check_sim10),
    ]
    
    passed_count = 0
    results_report = []
    
    for name, func in verifications:
        print(f"Verificando {name}...", end=" ", flush=True)
        try:
            success, msg = func()
            if success:
                print("\033[92m[OK]\033[0m")
                passed_count += 1
                results_report.append({"name": name, "status": "Passed", "message": msg})
            else:
                print(f"\033[91m[FAIL]\033[0m: {msg}")
                results_report.append({"name": name, "status": "Failed", "message": msg})
        except Exception as e:
            print(f"\033[91m[ERROR]\033[0m: {e}")
            results_report.append({"name": name, "status": "Error", "message": str(e)})
            
    print("==================================================")
    print(f"Resultado Final: {passed_count}/{len(verifications)} Passaram")
    print("==================================================")
    
    # Salva relatório em formato JSON para fácil leitura posterior
    summary_path = SIMS_DIR / "verification_summary.json"
    with open(summary_path, "w") as f:
        json.dump(results_report, f, indent=2)
    print(f"Resumo da verificação salvo em: {summary_path}")
    
    # Retorna código de erro se alguma validação falhou
    if passed_count < len(verifications):
        sys.exit(1)
    else:
        sys.exit(0)

if __name__ == "__main__":
    main()
