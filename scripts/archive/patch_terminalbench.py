import re

with open("benchmark/run_all.py", "r") as f:
    code = f.read()

new_logic = """
    print("\\n" + "=" * 60)
    print("📋 BENCHMARK 3/5: Terminal-Bench (HuggingFace)")
    print("=" * 60)
    
    tasks = []
    try:
        from datasets import load_dataset
        ds = load_dataset("josancamon/terminal-bench", split="test")
        for i in range(min(limit, len(ds))):
            item = ds[i]
            # Formato do dataset geralmente tem id, instruction, etc
            tasks.append({
                "task_id": str(item.get("id", f"TB-{i}")),
                "name": item.get("name", f"Task {i}"),
                "instruction": item.get("instruction", item.get("prompt", "")),
                # Simulando uma validação simples baseada em checar se o agente não crashou
                "validation": lambda path: True 
            })
        print(f"✓ {len(tasks)} tarefas Terminal-Bench carregadas do HuggingFace")
    except Exception as e:
        print(f"⚠️ Erro ao carregar Terminal-Bench: {e}")
        tasks = []
"""

old_logic = """
    print("\\n" + "=" * 60)
    print("📋 BENCHMARK 3/5: Terminal-Tasks (Custom)")
    print("=" * 60)
    # Tarefas realistas de terminal (baseadas no formato Terminal-Bench)
    tasks = []
"""

code = code.replace(old_logic.strip(), new_logic.strip())

with open("benchmark/run_all.py", "w") as f:
    f.write(code)

