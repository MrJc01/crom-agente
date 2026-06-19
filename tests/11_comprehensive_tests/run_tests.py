#!/usr/bin/env python3
import os
import sys
import json
import subprocess
import threading
import shutil
import time
from http.server import HTTPServer, BaseHTTPRequestHandler

PORT = 9099
CROM_BIN = os.path.abspath("../.bin/crom-agente")
WORKSPACE = os.path.abspath("./temp_workspace")

# Mock responses mapping
def get_mock_response(prompt, assistant_count):
    if "Hello.txt" in prompt:  # Test 3: write_file
        if assistant_count == 0:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Vou criar o arquivo Hello.txt",
                    "tool_calls": [{
                        "id": "tc_write",
                        "type": "function",
                        "function": {
                            "name": "write_file",
                            "arguments": {"path": "Hello.txt", "content": "Olá!"}
                        }
                    }]
                },
                "done": True
            }
        else:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Arquivo Hello.txt criado com sucesso."
                },
                "done": True
            }

    if "config.json" in prompt:  # Test 4: read_file
        if assistant_count == 0:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Vou ler config.json",
                    "tool_calls": [{
                        "id": "tc_read",
                        "type": "function",
                        "function": {
                            "name": "read_file",
                            "arguments": {"path": "config.json"}
                        }
                    }]
                },
                "done": True
            }
        else:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Leitura concluída."
                },
                "done": True
            }

    if "code.py" in prompt:  # Test 5: diff_replace
        if assistant_count == 0:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Vou substituir o conteúdo em code.py",
                    "tool_calls": [{
                        "id": "tc_diff",
                        "type": "function",
                        "function": {
                            "name": "diff_replace",
                            "arguments": {"path": "code.py", "target_content": "old_content", "replacement_content": "new_content"}
                        }
                    }]
                },
                "done": True
            }
        else:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Substituição concluída."
                },
                "done": True
            }

    if "obsoleto.log" in prompt:  # Test 6: delete_file
        if assistant_count == 0:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Vou deletar o arquivo obsoleto.log",
                    "tool_calls": [{
                        "id": "tc_delete",
                        "type": "function",
                        "function": {
                            "name": "delete_file",
                            "arguments": {"path": "obsoleto.log"}
                        }
                    }]
                },
                "done": True
            }
        else:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Arquivo deletado."
                },
                "done": True
            }

    if "src" in prompt:  # Test 7: tree
        if assistant_count == 0:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Vou listar a árvore de diretórios",
                    "tool_calls": [{
                        "id": "tc_tree",
                        "type": "function",
                        "function": {
                            "name": "tree",
                            "arguments": {"path": "."}
                        }
                    }]
                },
                "done": True
            }
        else:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Listagem concluída."
                },
                "done": True
            }

    if "TODO" in prompt:  # Test 8: grep_search
        if assistant_count == 0:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Vou buscar por TODO",
                    "tool_calls": [{
                        "id": "tc_grep",
                        "type": "function",
                        "function": {
                            "name": "grep_search",
                            "arguments": {"SearchPath": ".", "Query": "TODO"}
                        }
                    }]
                },
                "done": True
            }
        else:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Busca concluída."
                },
                "done": True
            }

    if "status do git" in prompt:  # Test 9: git_status
        if assistant_count == 0:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Vou rodar git status",
                    "tool_calls": [{
                        "id": "tc_git_status",
                        "type": "function",
                        "function": {
                            "name": "git_status",
                            "arguments": {}
                        }
                    }]
                },
                "done": True
            }
        else:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Status verificado."
                },
                "done": True
            }

    if "comite" in prompt:  # Test 10: git_commit
        if assistant_count == 0:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Vou adicionar",
                    "tool_calls": [{
                        "id": "tc_git_add",
                        "type": "function",
                        "function": {
                            "name": "git_add",
                            "arguments": {"paths": ["Hello.txt"]}
                        }
                    }]
                },
                "done": True
            }
        elif assistant_count == 1:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Vou comitar",
                    "tool_calls": [{
                        "id": "tc_git_commit",
                        "type": "function",
                        "function": {
                            "name": "git_commit",
                            "arguments": {"message": "feat(test): Commit de teste"}
                        }
                    }]
                },
                "done": True
            }
        else:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Comitado com sucesso."
                },
                "done": True
            }

    if "branch" in prompt:  # Test 11: git_branch
        if assistant_count == 0:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Vou criar a branch",
                    "tool_calls": [{
                        "id": "tc_git_branch",
                        "type": "function",
                        "function": {
                            "name": "git_branch",
                            "arguments": {"action": "create", "name": "feature-abc"}
                        }
                    }]
                },
                "done": True
            }
        else:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Branch criada."
                },
                "done": True
            }

    if "health" in prompt:  # Test 12: http_client
        if assistant_count == 0:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Vou fazer a requisição GET",
                    "tool_calls": [{
                        "id": "tc_http",
                        "type": "function",
                        "function": {
                            "name": "http_client",
                            "arguments": {"url": "http://localhost:9099/health", "method": "GET"}
                        }
                    }]
                },
                "done": True
            }
        else:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Requisição concluída."
                },
                "done": True
            }

    if "HTML" in prompt:  # Test 13: scraper
        if assistant_count == 0:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Vou raspar a página",
                    "tool_calls": [{
                        "id": "tc_scraper",
                        "type": "function",
                        "function": {
                            "name": "scraper",
                            "arguments": {"url": "http://localhost:9099/html-page"}
                        }
                    }]
                },
                "done": True
            }
        else:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Raspagem concluída."
                },
                "done": True
            }

    if "screenshot" in prompt:  # Test 14: browser_action
        if assistant_count == 0:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Vou tirar um screenshot",
                    "tool_calls": [{
                        "id": "tc_browser",
                        "type": "function",
                        "function": {
                            "name": "browser_action",
                            "arguments": {"action": "screenshot", "path": "print.png", "url": "http://localhost:9099/html-page"}
                        }
                    }]
                },
                "done": True
            }
        else:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Screenshot concluído."
                },
                "done": True
            }

    if "complexa" in prompt:  # Test 15: complexity_reducer
        if assistant_count == 0:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Vou simplificar a função",
                    "tool_calls": [{
                        "id": "tc_reducer",
                        "type": "function",
                        "function": {
                            "name": "complexity_reducer",
                            "arguments": {"path": "code.py"}
                        }
                    }]
                },
                "done": True
            }
        else:
            return {
                "message": {
                    "role": "assistant",
                    "content": "Código simplificado."
                },
                "done": True
            }

    return {
        "message": {
            "role": "assistant",
            "content": "Tarefa concluída."
        },
        "done": True
    }

class MockHTTPHandler(BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        pass  # Suppress request logging

    def do_GET(self):
        if self.path == "/health":
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(b'{"status":"ok"}')
        elif self.path == "/html-page":
            self.send_response(200)
            self.send_header("Content-Type", "text/html; charset=utf-8")
            self.end_headers()
            self.wfile.write(b'<html><body><h1>Hello HTML Page</h1></body></html>')
        else:
            self.send_response(404)
            self.end_headers()

    def do_POST(self):
        if self.path == "/api/chat":
            content_length = int(self.headers['Content-Length'])
            post_data = self.rfile.read(content_length)
            req = json.loads(post_data.decode('utf-8'))
            messages = req.get('messages', [])
            
            user_msg = next((m for m in reversed(messages) if m.get('role') == 'user'), None)
            prompt = user_msg['content'] if user_msg else ""
            assistant_count = sum(1 for m in messages if m.get('role') == 'assistant')
            
            resp = get_mock_response(prompt, assistant_count)
            
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps(resp).encode('utf-8'))
        else:
            self.send_response(404)
            self.end_headers()

def start_mock_server():
    server = HTTPServer(('localhost', PORT), MockHTTPHandler)
    server_thread = threading.Thread(target=server.serve_forever, daemon=True)
    server_thread.start()
    return server

# Helper to run the agent CLI
def run_agent(task_prompt):
    env = os.environ.copy()
    env["OLLAMA_HOST"] = f"http://localhost:{PORT}"
    
    args = [
        CROM_BIN,
        "run", task_prompt,
        "--workspace", WORKSPACE,
        "--storage", os.path.join(WORKSPACE, ".crom"),
        "--provider", "ollama",
        "--model", "llama3",
        "--permission-mode", "total_access",
        "--max-iterations", "5",
        "--timeout", "10",
        "--disable-prompt-optimization"
    ]
    
    res = subprocess.run(args, capture_output=True, text=True, env=env)
    return res

# 15 tests list
results = []

def run_test(num, name, setup_fn, task_prompt, assertion_fn):
    print(f"▶️ Test {num}: {name}...", end="", flush=True)
    
    # 1. Clean workspace
    if os.path.exists(WORKSPACE):
        shutil.rmtree(WORKSPACE)
    os.makedirs(WORKSPACE)
    os.makedirs(os.path.join(WORKSPACE, ".crom"))
    
    # 2. Run custom test setup
    if setup_fn:
        setup_fn()
        
    # 3. Execute agent/CLI
    res = None
    if task_prompt:
        res = run_agent(task_prompt)
    
    # 4. Check assertion
    success, details = assertion_fn(res)
    
    if success:
        print(" \033[0;32m[PASS]\033[0m")
        results.append({"test": num, "name": name, "status": "PASS"})
    else:
        print(" \033[0;31m[FAIL]\033[0m")
        print(f"   Details: {details}")
        if res and res.stdout:
            print("   --- stdout ---")
            print(res.stdout)
        if res and res.stderr:
            print("   --- stderr ---")
            print(res.stderr)
        results.append({"test": num, "name": name, "status": "FAIL", "reason": details})

# Test Assertions & Setups
# 1. Version Help
def setup_t1():
    pass
def assert_t1(res):
    # Direct execution of --help instead of standard run
    args = [CROM_BIN, "--help"]
    res_help = subprocess.run(args, capture_output=True, text=True, env=os.environ)
    if "crom-agente" in res_help.stdout and res_help.returncode == 0:
        return True, ""
    return False, "Help output or exit code invalid"

# 2. State Command
def setup_t2():
    state_file = os.path.join(WORKSPACE, ".crom", ".crom_state.json")
    with open(state_file, "w") as f:
        json.dump({
            "status": "idle",
            "ultimo_status": "finished",
            "tarefa_em_andamento": "Test Task",
            "tokens_gastos": 100,
            "total_turnos": 5,
            "diretorio_atual": WORKSPACE
        }, f)
def assert_t2(res):
    args = [CROM_BIN, "state", "--workspace", WORKSPACE, "--storage", os.path.join(WORKSPACE, ".crom")]
    res_state = subprocess.run(args, capture_output=True, text=True, env=os.environ)
    output = res_state.stdout + res_state.stderr
    if "finished" in output and "Test Task" in output:
        return True, ""
    return False, f"State command output invalid. stdout: {res_state.stdout!r}, stderr: {res_state.stderr!r}, code: {res_state.returncode}"

# 3. write_file
def setup_t3():
    pass
def assert_t3(res):
    target = os.path.join(WORKSPACE, "Hello.txt")
    if os.path.exists(target):
        with open(target, "r") as f:
            content = f.read()
        if content == "Olá!":
            return True, ""
        return False, f"File hello.txt has content: {content}"
    return False, "File Hello.txt was not created"

# 4. read_file
def setup_t4():
    with open(os.path.join(WORKSPACE, "config.json"), "w") as f:
        f.write('{"status": "test"}')
def assert_t4(res):
    if res.returncode == 0:
        return True, ""
    return False, "Agent failed to read file"

# 5. diff_replace
def setup_t5():
    with open(os.path.join(WORKSPACE, "code.py"), "w") as f:
        f.write("old_content\n")
def assert_t5(res):
    target = os.path.join(WORKSPACE, "code.py")
    with open(target, "r") as f:
        content = f.read()
    if "new_content" in content:
        return True, ""
    return False, f"File code.py was not modified: {content}"

# 6. delete_file
def setup_t6():
    with open(os.path.join(WORKSPACE, "obsoleto.log"), "w") as f:
        f.write("logs\n")
def assert_t6(res):
    target = os.path.join(WORKSPACE, "obsoleto.log")
    if not os.path.exists(target):
        return True, ""
    return False, "File obsoleto.log still exists"

# 7. tree
def setup_t7():
    os.makedirs(os.path.join(WORKSPACE, "src"))
    with open(os.path.join(WORKSPACE, "src/main.go"), "w") as f:
        f.write("package main\n")
def assert_t7(res):
    if res.returncode == 0:
        return True, ""
    return False, "tree scan failed"

# 8. grep_search
def setup_t8():
    with open(os.path.join(WORKSPACE, "notes.txt"), "w") as f:
        f.write("TODO: fix bugs\n")
def assert_t8(res):
    if res.returncode == 0:
        return True, ""
    return False, "grep_search failed"

# 9. git_status
def setup_t9():
    subprocess.run(["git", "init"], cwd=WORKSPACE, capture_output=True)
    subprocess.run(["git", "config", "user.name", "Test User"], cwd=WORKSPACE, capture_output=True)
    subprocess.run(["git", "config", "user.email", "test@example.com"], cwd=WORKSPACE, capture_output=True)
def assert_t9(res):
    if res.returncode == 0:
        return True, ""
    return False, "git_status failed"

# 10. git_commit
def setup_t10():
    subprocess.run(["git", "init"], cwd=WORKSPACE, capture_output=True)
    subprocess.run(["git", "config", "user.name", "Test User"], cwd=WORKSPACE, capture_output=True)
    subprocess.run(["git", "config", "user.email", "test@example.com"], cwd=WORKSPACE, capture_output=True)
    with open(os.path.join(WORKSPACE, "Hello.txt"), "w") as f:
        f.write("text\n")
def assert_t10(res):
    # Verify git log to ensure commit was done
    git_log = subprocess.run(["git", "log", "--oneline"], cwd=WORKSPACE, capture_output=True, text=True, env=os.environ)
    if "Commit de teste" in git_log.stdout:
        return True, ""
    return False, f"Git commit not found in log: {git_log.stdout}"

# 11. git_branch
def setup_t11():
    subprocess.run(["git", "init"], cwd=WORKSPACE, capture_output=True)
    subprocess.run(["git", "config", "user.name", "Test User"], cwd=WORKSPACE, capture_output=True)
    subprocess.run(["git", "config", "user.email", "test@example.com"], cwd=WORKSPACE, capture_output=True)
    with open(os.path.join(WORKSPACE, "a.txt"), "w") as f:
        f.write("text\n")
    subprocess.run(["git", "add", "."], cwd=WORKSPACE, capture_output=True)
    subprocess.run(["git", "commit", "-m", "feat(init): initial commit"], cwd=WORKSPACE, capture_output=True)
def assert_t11(res):
    git_branches = subprocess.run(["git", "branch", "--list"], cwd=WORKSPACE, capture_output=True, text=True, env=os.environ)
    if "feature-abc" in git_branches.stdout:
        return True, ""
    return False, f"Git branches: {git_branches.stdout}"

# 12. http_client
def setup_t12():
    pass
def assert_t12(res):
    if res.returncode == 0:
        return True, ""
    return False, "http_client request failed"

# 13. scraper
def setup_t13():
    pass
def assert_t13(res):
    if res.returncode == 0:
        return True, ""
    return False, "scraper request failed"

# 14. browser_action
def setup_t14():
    pass
def assert_t14(res):
    if res.returncode == 0:
        return True, ""
    return False, "browser_action screenshot failed"

# 15. complexity_reducer
def setup_t15():
    with open(os.path.join(WORKSPACE, "code.py"), "w") as f:
        f.write("def complex_function(x):\n  # too many loops\n  return x\n")
def assert_t15(res):
    if res.returncode == 0:
        return True, ""
    return False, "complexity_reducer failed"

def main():
    print("🚀 Starting local Mock HTTP Server...")
    server = start_mock_server()
    print("🟢 Mock HTTP Server running on http://localhost:9099")
    print(f"🟢 Target Binary: {CROM_BIN}")
    print(f"🟢 Workspace Path: {WORKSPACE}")
    print("--------------------------------------------------")
    
    # Run tests
    run_test(1, "CLI Help & Version Info", setup_t1, None, assert_t1)
    run_test(2, "CLI State Inspection", setup_t2, None, assert_t2)
    run_test(3, "File Creation (write_file)", setup_t3, "Crie o arquivo Hello.txt com o conteúdo Olá!", assert_t3)
    run_test(4, "File Reading (read_file)", setup_t4, "Leia o conteúdo de config.json e retorne", assert_t4)
    run_test(5, "Diff Replacement (diff_replace)", setup_t5, "Substitua o conteúdo antigo por novo no arquivo code.py", assert_t5)
    run_test(6, "File Deletion (delete_file)", setup_t6, "Remova o arquivo obsoleto.log", assert_t6)
    run_test(7, "Directory Tree Scan (tree)", setup_t7, "Liste o diretório src", assert_t7)
    run_test(8, "Content Search (grep_search)", setup_t8, "Procure por TODO em todos os arquivos", assert_t8)
    run_test(9, "Git Init & Status (git_status)", setup_t9, "Verifique o status do git no repositório", assert_t9)
    run_test(10, "Git Commit (git_add / git_commit)", setup_t10, "Adicione e comite as alterações do arquivo", assert_t10)
    run_test(11, "Git Branching (git_branch)", setup_t11, "Crie a branch feature-abc no git", assert_t11)
    run_test(12, "HTTP Client requests (http_client)", setup_t12, "Faça uma requisição GET para http://localhost:9099/health", assert_t12)
    run_test(13, "Page Scraping (scraper)", setup_t13, "Extraia o texto da página HTML no scraper", assert_t13)
    run_test(14, "Browser Automation (browser_action)", setup_t14, "Tire um screenshot da página http://localhost:9099/html-page", assert_t14)
    run_test(15, "Coding Helper (complexity_reducer)", setup_t15, "Simplifique a função complexa em code.py", assert_t15)

    print("--------------------------------------------------")
    print("📊 Test Summary:")
    passed_count = sum(1 for r in results if r["status"] == "PASS")
    failed_count = sum(1 for r in results if r["status"] == "FAIL")
    print(f"   Passed: {passed_count} / {len(results)}")
    print(f"   Failed: {failed_count} / {len(results)}")
    
    # Save results to json
    with open("test_results.json", "w") as f:
        json.dump(results, f, indent=2)
        
    # Shutdown server
    server.shutdown()
    
    if failed_count > 0:
        sys.exit(1)
    else:
        sys.exit(0)

if __name__ == "__main__":
    main()
