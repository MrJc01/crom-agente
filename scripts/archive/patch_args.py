import re

with open('benchmark/run_all.py', 'r') as f:
    content = f.read()

# Add to argparse
content = content.replace(
    '    parser.add_argument("--clear-progress", action="store_true", help="Limpa o progresso salvo antes de iniciar")',
    '    parser.add_argument("--clear-progress", action="store_true", help="Limpa o progresso salvo antes de iniciar")\n    parser.add_argument("--api-key", type=str, default="", help="API Key dinâmica (sobrepõe o .env)")\n    parser.add_argument("--temp", type=float, default=0.0, help="Temperatura do LLM")'
)

# Set api key and temp
content = content.replace(
    '    if args.clear_progress:',
    '    if args.api_key:\n        os.environ[f"{PROVIDER.upper()}_API_KEY"] = args.api_key\n        print("🔑 Usando API Key passada via flag.")\n    if args.temp > 0.0:\n        os.environ["CROM_TEMPERATURE"] = str(args.temp)\n        print(f"🌡️ Configurando Temperature para {args.temp}")\n\n    if args.clear_progress:'
)

with open('benchmark/run_all.py', 'w') as f:
    f.write(content)
