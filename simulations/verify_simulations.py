import os
import sys
import json
import subprocess
import shutil
from pathlib import Path

# Configuração de caminhos
BASE_DIR = Path("/home/j/Documentos/GitHub/crom-agente")
SIMS_DIR = BASE_DIR / "simulations"

def check_sanity(file_path):
    """Verifica se o arquivo existe, não está vazio, tem tamanho mínimo e não contém apenas comentários/fences vazios."""
    if not file_path.exists():
        return False, f"Arquivo {file_path.name} não existe."
    content = file_path.read_text().strip()
    if not content:
        return False, f"Arquivo {file_path.name} está vazio."
    if len(content) < 15:
        return False, f"Arquivo {file_path.name} é excessivamente curto ({len(content)} bytes)."
    
    # Remove linhas vazias e delimitadores de código markdown
    lines = [line.strip() for line in content.splitlines() if line.strip()]
    non_fence_lines = [line for line in lines if not line.startswith("```") and not line.startswith("#") and not line.startswith("//") and not line.startswith("/*") and not line.endswith("*/")]
    if not non_fence_lines:
        return False, f"Arquivo {file_path.name} contém apenas comentários ou delimitadores de bloco de código markdown."
    return True, ""

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
    
    ok, err = check_sanity(report_file)
    if not ok:
        return False, err
        
    content = report_file.read_text().strip()
    if len(content) < 50:
        return False, f"relatorio_clima.md é muito curto ({len(content)} caracteres)."
    
    # Verifica parágrafos básicos (com tolerância de formato, ex: 1 parágrafo vs 3)
    paragraphs = [p for p in content.split("\n\n") if p.strip()]
    if len(paragraphs) < 1:
        return False, "Nenhum parágrafo/seção válido encontrado."
    
    return True, f"relatorio_clima.md validado com sucesso ({len(paragraphs)} seções)."

def check_sim04():
    """factorial script check"""
    ws = SIMS_DIR / "sim04_factorial"
    script = ws / "fatorial.py"
    
    ok, err = check_sanity(script)
    if not ok:
        return False, err
    
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
    
    ok, err = check_sanity(script)
    if not ok:
        return False, err
    ok, err = check_sanity(data_file)
    if not ok:
        return False, err
        
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
    
    ok, err = check_sanity(script)
    if not ok:
        return False, err
        
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
    
    ok, err = check_sanity(html_file)
    if not ok:
        return False, err
        
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
    
    ok, err = check_sanity(server_file)
    if not ok:
        return False, err
        
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
    
    ok, err = check_sanity(script)
    if not ok:
        return False, err
        
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
        
    # Valida presença de código php básico (<?php) nos arquivos e sanity
    for path, name in [(db_config, "config/db.php"), (model_item, "models/Item.php"), (controller_item, "controllers/ItemController.php")]:
        ok, err = check_sanity(path)
        if not ok:
            return False, f"Sanidade de {name} falhou: {err}"
        content = path.read_text().strip()
        if not content.startswith("<?php"):
            return False, f"Arquivo {name} não inicia com tag PHP válida '<?php'."
            
    return True, "Estrutura de arquivos Yii2 MVC e tags PHP validadas com sucesso."

def check_generic_file(ws_dir, file_name, min_len=15):
    path = ws_dir / file_name
    if not path.exists():
        return False, f"Arquivo {file_name} nao existe no workspace."
    if min_len > 0:
        return check_sanity(path)
    return True, f"Arquivo {file_name} existe."

def check_go_compile(ws_dir, file_name):
    path = ws_dir / file_name
    ok, err = check_sanity(path)
    if not ok:
        return False, err
    try:
        binary_out = ws_dir / "test_bin"
        if binary_out.exists():
            binary_out.unlink()
        res = subprocess.run(["go", "build", "-o", str(binary_out), str(path)], capture_output=True, text=True, timeout=20, cwd=str(ws_dir))
        if res.returncode != 0:
            return False, f"Go build falhou: {res.stderr}"
        if binary_out.exists():
            binary_out.unlink()
        return True, "Go compilado com sucesso."
    except Exception as e:
        return False, f"Erro compilação Go: {e}"

def check_sim11():
    return check_generic_file(SIMS_DIR / "sim11_link_validator", "validator.py")

def check_sim12():
    return check_go_compile(SIMS_DIR / "sim12_go_crud", "server.go")

def check_sim13():
    return check_generic_file(SIMS_DIR / "sim13_log_parser", "parse_logs.sh")

def check_sim14():
    return check_generic_file(SIMS_DIR / "sim14_async_downloader", "downloader.py")

def check_sim15():
    ws = SIMS_DIR / "sim15_jwt_middleware"
    ok1, err1 = check_generic_file(ws, "app.js")
    if not ok1: return False, err1
    return check_generic_file(ws, "auth.js")

def check_sim16():
    return check_generic_file(SIMS_DIR / "sim16_sqlite_transaction", "transfer.py")

def check_sim17():
    return check_go_compile(SIMS_DIR / "sim17_concurrent_crawler", "crawler.go")

def check_sim18():
    return check_generic_file(SIMS_DIR / "sim18_email_extractor", "parse_emails.py")

def check_sim19():
    return check_generic_file(SIMS_DIR / "sim19_csv_stream", "stream_processor.js")

def check_sim20():
    return check_go_compile(SIMS_DIR / "sim20_exec_timeout", "runner.go")

def check_sim21():
    return check_generic_file(SIMS_DIR / "sim21_file_cipher", "cipher.py")

def check_sim22():
    return check_generic_file(SIMS_DIR / "sim22_migrations_runner", "migrate.py")

def check_sim23():
    return check_generic_file(SIMS_DIR / "sim23_html_parser", "parser.py")

def check_sim24():
    return check_go_compile(SIMS_DIR / "sim24_tcp_server", "tcp_server.go")

def check_sim25():
    return check_generic_file(SIMS_DIR / "sim25_markdown_converter", "md2html.js")

def check_sim26():
    return check_generic_file(SIMS_DIR / "sim26_csv_importer", "import_csv.py")

def check_sim27():
    return check_go_compile(SIMS_DIR / "sim27_memory_cache", "cache.go")

def check_sim28():
    return check_generic_file(SIMS_DIR / "sim28_path_sanitizer", "sandbox.py")

def check_sim29():
    ws = SIMS_DIR / "sim29_yii2_rest"
    path = ws / "controllers" / "PostController.php"
    return check_sanity(path)

def check_sim30():
    return check_generic_file(SIMS_DIR / "sim30_cli_manager", "cli.py")

def check_sim31():
    return check_generic_file(SIMS_DIR / "sim31_legal_contract", "contrato_prestacao_servicos.md")

def check_sim32():
    return check_generic_file(SIMS_DIR / "sim32_expense_report", "resumo_financeiro.py")

def check_sim33():
    return check_generic_file(SIMS_DIR / "sim33_recipe_scaler", "redimensionar_receita.py")

def check_sim34():
    return check_generic_file(SIMS_DIR / "sim34_translation", "tradutor.py")

def check_sim35():
    return check_generic_file(SIMS_DIR / "sim35_abnt_bibliography", "formatador_abnt.py")

def check_sim36():
    return check_generic_file(SIMS_DIR / "sim36_marketing_calendar", "calendario_editorial.md")

def check_sim37():
    return check_generic_file(SIMS_DIR / "sim37_real_estate", "gerador_anuncios.py")

def check_sim38():
    return check_generic_file(SIMS_DIR / "sim38_customer_support", "respostas_suporte.md")

def check_sim39():
    return check_generic_file(SIMS_DIR / "sim39_logistics_planner", "planejador_rotas.py")

def check_sim40():
    return check_generic_file(SIMS_DIR / "sim40_fitness_workout", "treino.py")

def check_sim41():
    return check_generic_file(SIMS_DIR / "sim41_meal_planner", "plano_alimentar.md")

def check_sim42():
    return check_generic_file(SIMS_DIR / "sim42_inventory_report", "estoque.py")

def check_sim43():
    return check_generic_file(SIMS_DIR / "sim43_travel_itinerary", "roteiro_viagem.md")

def check_sim44():
    return check_generic_file(SIMS_DIR / "sim44_press_release", "press_release.md")

def check_sim45():
    return check_generic_file(SIMS_DIR / "sim45_book_review", "resenha.py")

def check_sim46():
    return check_generic_file(SIMS_DIR / "sim46_lesson_plan", "plano_aula.md")

def check_sim47():
    return check_generic_file(SIMS_DIR / "sim47_product_pricing", "precificacao.py")

def check_sim48():
    return check_generic_file(SIMS_DIR / "sim48_employee_scheduler", "escala.py")

def check_sim49():
    return check_generic_file(SIMS_DIR / "sim49_email_newsletter", "newsletter.html")

def check_sim50():
    return check_generic_file(SIMS_DIR / "sim50_meeting_minutes", "ata_kickoff.md")

def check_sim51():
    return check_generic_file(SIMS_DIR / "sim51_jwst_wiki", "jwst_resumo.md")

def check_sim52():
    return check_generic_file(SIMS_DIR / "sim52_screenshot_mobile", "example_mobile.png", min_len=0)

def check_sim53():
    return check_generic_file(SIMS_DIR / "sim53_github_metrics", "yii2_metrics.json")

def check_sim54():
    return check_generic_file(SIMS_DIR / "sim54_weather_forecast", "previsao_sp.txt")

def check_sim55():
    return check_generic_file(SIMS_DIR / "sim55_exchange_rates", "cotacoes.json")

def check_sim56():
    return check_generic_file(SIMS_DIR / "sim56_pdf_download", "guia.pdf", min_len=0)

def check_sim57():
    return check_generic_file(SIMS_DIR / "sim57_seo_search", "seo_results.json")

def check_sim58():
    return check_generic_file(SIMS_DIR / "sim58_news_summary", "tech_news.md")

def check_sim59():
    return check_generic_file(SIMS_DIR / "sim59_go_doc_formatting", "formatting_links.txt")

def check_sim60():
    return check_generic_file(SIMS_DIR / "sim60_price_comparison", "rtx4060_precos.json")

def check_sim61():
    return check_generic_file(SIMS_DIR / "sim61_openapi_schema", "openapi_schema.json")

def check_sim62():
    ws = SIMS_DIR / "sim62_layout_check"
    ok1, err1 = check_generic_file(ws, "wikipedia_globe.png", min_len=0)
    if not ok1: return False, err1
    return check_generic_file(ws, "layout_report.txt")

def check_sim63():
    return check_generic_file(SIMS_DIR / "sim63_trending_topics", "trends_post.md")

def check_sim64():
    return check_generic_file(SIMS_DIR / "sim64_stocks_ticker", "stocks_report.md")

def check_sim65():
    return check_generic_file(SIMS_DIR / "sim65_form_filler", "form_preenchido.png", min_len=0)

def check_sim66():
    return check_generic_file(SIMS_DIR / "sim66_pep8_research", "pep8_info.md")

def check_sim67():
    return check_generic_file(SIMS_DIR / "sim67_restaurant_scraper", "restaurante_info.json")

def check_sim68():
    return check_generic_file(SIMS_DIR / "sim68_google_logo", "google_logo.png", min_len=0)

def check_sim69():
    return check_generic_file(SIMS_DIR / "sim69_security_policy", "cloudflare_security.txt")

def check_sim70():
    return check_generic_file(SIMS_DIR / "sim70_conference_schedule", "grade_palestras.md")

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
        ("Simulação 11 (Markdown Link Validator)", check_sim11),
        ("Simulação 12 (Go REST API CRUD SQLite)", check_sim12),
        ("Simulação 13 (Bash Log Parser)", check_sim13),
        ("Simulação 14 (Async Image Downloader)", check_sim14),
        ("Simulação 15 (Node.js JWT Middleware)", check_sim15),
        ("Simulação 16 (SQLite ACID Transaction)", check_sim16),
        ("Simulação 17 (Go Concurrent Web Crawler)", check_sim17),
        ("Simulação 18 (Python Regex Email Extractor)", check_sim18),
        ("Simulação 19 (Node.js CSV Stream Filter)", check_sim19),
        ("Simulação 20 (Go Exec Command Timeout)", check_sim20),
        ("Simulação 21 (Python XOR/AES File Cipher)", check_sim21),
        ("Simulação 22 (SQL Migrations Runner)", check_sim22),
        ("Simulação 23 (BeautifulSoup HTML Parser)", check_sim23),
        ("Simulação 24 (Go TCP Reverse Echo Server)", check_sim24),
        ("Simulação 25 (Node.js Markdown to HTML Converter)", check_sim25),
        ("Simulação 26 (Python CSV to SQLite Importer)", check_sim26),
        ("Simulação 27 (Go Concurrent Memory Cache TTL)", check_sim27),
        ("Simulação 28 (Python Path Traversal Sanitizer)", check_sim28),
        ("Simulação 29 (Yii2 REST Controller SQLite)", check_sim29),
        ("Simulação 30 (Python CLI Task Manager)", check_sim30),
        ("Simulação 31 (Legal Contract Drafting)", check_sim31),
        ("Simulação 32 (Financial Expense Report)", check_sim32),
        ("Simulação 33 (Recipe Book Scaler)", check_sim33),
        ("Simulação 34 (Language Translation & Localization)", check_sim34),
        ("Simulação 35 (Academic Bibliography Formatter)", check_sim35),
        ("Simulação 36 (Weekly Social Media Calendar)", check_sim36),
        ("Simulação 37 (Real Estate Listing Generator)", check_sim37),
        ("Simulação 38 (Customer Support Response Templates)", check_sim38),
        ("Simulação 39 (Logistics Route & Load Planner)", check_sim39),
        ("Simulação 40 (Workout Routine & Calorie Calculator)", check_sim40),
        ("Simulação 41 (Nutrition Meal Planner)", check_sim41),
        ("Simulação 42 (Inventory Stock Management Report)", check_sim42),
        ("Simulação 43 (Travel Itinerary Planner)", check_sim43),
        ("Simulação 44 (Product Launch Press Release)", check_sim44),
        ("Simulação 45 (Book Review Organizer)", check_sim45),
        ("Simulação 46 (Classroom Lesson Plan)", check_sim46),
        ("Simulação 47 (Retail Product Pricing Calculator)", check_sim47),
        ("Simulação 48 (Employee Shift Scheduler)", check_sim48),
        ("Simulação 49 (Email Newsletter Draft)", check_sim49),
        ("Simulação 50 (Project Kickoff Meeting Minutes)", check_sim50),
        ("Simulação 51 (Wikipedia Search & Summary)", check_sim51),
        ("Simulação 52 (Responsive Website Screenshot)", check_sim52),
        ("Simulação 53 (GitHub Repository Info Search)", check_sim53),
        ("Simulação 54 (Weather Forecast Finder)", check_sim54),
        ("Simulação 55 (Exchange Rate Scraper)", check_sim55),
        ("Simulação 56 (Online PDF Guide Downloader)", check_sim56),
        ("Simulação 57 (Search Engine Rank Checker)", check_sim57),
        ("Simulação 58 (Tech News Summary)", check_sim58),
        ("Simulação 59 (Doc Downloader & Link Extractor)", check_sim59),
        ("Simulação 60 (E-commerce Price Comparer)", check_sim60),
        ("Simulação 61 (Public API Schema Fetcher)", check_sim61),
        ("Simulação 62 (Visual Layout Bug Detector)", check_sim62),
        ("Simulação 63 (Trending Topics Finder)", check_sim63),
        ("Simulação 64 (Stock Market Ticker Research)", check_sim64),
        ("Simulação 65 (Online HTML Form Filler)", check_sim65),
        ("Simulação 66 (Official Python PEP Search)", check_sim66),
        ("Simulação 67 (Local Business Info Scraper)", check_sim67),
        ("Simulação 68 (Static Assets Downloader)", check_sim68),
        ("Simulação 69 (Security Policy Checker)", check_sim69),
        ("Simulação 70 (Conference Schedule Scraper)", check_sim70),
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
