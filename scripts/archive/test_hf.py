import requests
res = requests.get("https://huggingface.co/api/datasets?search=terminal-bench")
if res.ok:
    print([d["id"] for d in res.json()])
