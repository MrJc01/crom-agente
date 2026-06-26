import re
import sys

with open("benchmark/run_all.py", "r") as f:
    code = f.read()

# We need to find the run_swebench function
# And inject predictions.jsonl logic

predictions_logic = """
            if has_patch:
                try:
                    patch_content = (Path(tmpdir) / "fix.patch").read_text()
                    pred = {
                        "instance_id": iid,
                        "model_name_or_path": "crom-agente",
                        "model_patch": patch_content
                    }
                    pred_file = REPORTS_DIR / "predictions.jsonl"
                    with open(pred_file, "a") as pf:
                        import json
                        pf.write(json.dumps(pred) + "\\n")
                except Exception as e:
                    print(f"Error saving prediction for {iid}: {e}")
"""

# Replace the block where has_patch is defined and checked.
old_block = """            has_patch = (Path(tmpdir) / "fix.patch").exists()
            has_analysis = (Path(tmpdir) / "analise.md").exists()
            
            # Verificar se o patch é um diff válido (não apenas um arquivo vazio)
            valid_patch = False
            if has_patch:
                patch_content = (Path(tmpdir) / "fix.patch").read_text()
                valid_patch = ("---" in patch_content and "+++" in patch_content) or ("diff" in patch_content.lower())"""

new_block = old_block + predictions_logic

if "predictions.jsonl" not in code:
    code = code.replace(old_block, new_block)
    
    evaluation_logic = """
    # Run SWE-bench evaluation harness if possible
    pred_file = REPORTS_DIR / "predictions.jsonl"
    if pred_file.exists():
        print("\\n🚀 Iniciando avaliação nativa do SWE-bench a partir de predictions.jsonl...")
        import subprocess
        try:
            # Check if swebench is installed
            res = subprocess.run(["python3", "-m", "swebench.harness.run_evaluation", "--help"], capture_output=True)
            if res.returncode == 0:
                print("⏳ Executando swebench harness (isso pode demorar MUITO tempo para buildar as imagens docker)...")
                subprocess.Popen(
                    ["python3", "-m", "swebench.harness.run_evaluation", 
                     "--dataset_name", "princeton-nlp/SWE-bench_Lite", 
                     "--predictions_path", str(pred_file),
                     "--max_workers", str(workers),
                     "--run_id", "crom_agente_run"],
                    cwd=str(BASE_DIR)
                )
            else:
                print("⚠️ Pacote 'swebench' não está instalado no ambiente. Ignorando avaliação nativa.")
                print("   Instale com: pip install swebench docker")
        except Exception as e:
            print(f"⚠️ Erro ao tentar rodar SWE-bench harness: {e}")
            
    return {"benchmark": "swe-bench", "results": results}"""

    code = code.replace('    return {"benchmark": "swe-bench", "results": results}', evaluation_logic, 1) # replace only the first occurrence which is swebench

    with open("benchmark/run_all.py", "w") as f:
        f.write(code)

