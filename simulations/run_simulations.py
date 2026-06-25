import os
import sys
import json
import time
import subprocess
import shutil
from pathlib import Path

# Paths
BASE_DIR = Path("/home/j/Documentos/GitHub/crom-agente")
SIMS_DIR = BASE_DIR / "simulations"
BINARY_PATH = BASE_DIR / "build" / "crom-agente"

# Prompts and workspace paths for the 10 simulations
SIMULATIONS = [
    {
        "id": 1,
        "name": "Greeting (Fast Path)",
        "prompt": "Olá",
        "dir": "sim01_greeting"
    },
    {
        "id": 2,
        "name": "Yes/No Reply (Fast Path)",
        "prompt": "Sim",
        "dir": "sim02_yes_no"
    },
    {
        "id": 3,
        "name": "Report Generation (Basic)",
        "prompt": "Crie um arquivo chamado 'relatorio_clima.md' contendo um relatório simples sobre o clima no Brasil em 3 parágrafos.",
        "dir": "sim03_report"
    },
    {
        "id": 4,
        "name": "Factorial Script (Basic)",
        "prompt": "Escreva um script em Python chamado 'fatorial.py' que calcula o fatorial de 5 e imprime o resultado.",
        "dir": "sim04_factorial"
    },
    {
        "id": 5,
        "name": "JSON Parser (Medium)",
        "prompt": "Crie um script em Python chamado 'formatador.py' que lê um arquivo JSON 'dados.json' e imprime formatado. Adicione também um arquivo 'dados.json' com dados fictícios de teste.",
        "dir": "sim05_json_parser"
    },
    {
        "id": 6,
        "name": "SQLite Product Table (Medium)",
        "prompt": "Crie um script Python 'db.py' que cria uma tabela de produtos num banco SQLite local 'vendas.db', insere 3 itens e faz uma query listando-os no console.",
        "dir": "sim06_sqlite"
    },
    {
        "id": 7,
        "name": "HTML Static Page (Medium)",
        "prompt": "Gere uma página HTML 'index.html' elegante e minimalista com CSS inline estilizando um portfólio profissional de desenvolvedor.",
        "dir": "sim07_portfolio"
    },
    {
        "id": 8,
        "name": "Go HTTP Server (Advanced)",
        "prompt": "Crie um servidor web simples em Go em 'server.go' com um endpoint '/' que retorna a mensagem 'Hello, Crom!' em formato JSON. Adicione um comando terminal no seu plano para compilar/verificar o arquivo.",
        "dir": "sim08_go_server"
    },
    {
        "id": 9,
        "name": "File Organizer (Advanced)",
        "prompt": "Crie um script Python 'organizer.py' que organiza arquivos de uma pasta movendo arquivos com extensão .txt para a pasta 'textos' e .json para a pasta 'dados'.",
        "dir": "sim09_file_organizer"
    },
    {
        "id": 10,
        "name": "Yii2 MVC PHP SQLite (Complex)",
        "prompt": "Crie a estrutura de arquivos para um template de projeto PHP Yii2 MVC usando SQLite. Crie os arquivos 'config/db.php' com a conexão PDO SQLite, um model ActiveRecord em 'models/Item.php', e um controller em 'controllers/ItemController.php'.",
        "dir": "sim10_yii2_sqlite"
    },
    {
        "id": 11,
        "name": "Markdown Link Validator (Medium)",
        "prompt": "Crie um script em Python 'validator.py' que lê todos os arquivos `.md` no diretório atual, extrai todos os links no formato `[texto](url)` ou `[texto](path)` e verifica se os arquivos locais referenciados existem no disco.",
        "dir": "sim11_link_validator"
    },
    {
        "id": 12,
        "name": "Go REST API CRUD SQLite (Advanced)",
        "prompt": "Crie um servidor web em Go em 'server.go' que escuta na porta 8085 e implementa endpoints REST CRUD para uma entidade 'Task' (campos: id, title, done) conectando em um banco SQLite 'todo.db'. Não utilize ORMs (use raw sql/database/sql).",
        "dir": "sim12_go_crud"
    },
    {
        "id": 13,
        "name": "Bash Log Parser (Medium)",
        "prompt": "Crie um script bash 'parse_logs.sh' que lê um arquivo de log HTTP 'access.log' no diretório atual e imprime: (1) o número total de requisições, (2) o IP mais frequente, (3) a quantidade total de requisições que retornaram status 404.",
        "dir": "sim13_log_parser"
    },
    {
        "id": 14,
        "name": "Async Image Downloader (Advanced)",
        "prompt": "Crie um script Python 'downloader.py' que usa `asyncio` e `aiohttp` (ou `urllib` concorrente) para baixar de forma assíncrona uma lista de URLs de imagens definidas em um arquivo `urls.json` e as salva em uma pasta 'imagens/'.",
        "dir": "sim14_async_downloader"
    },
    {
        "id": 15,
        "name": "Node.js JWT Middleware (Advanced)",
        "prompt": "Crie um arquivo 'app.js' e um middleware 'auth.js' em Node.js (CommonJS ou ESM) contendo uma rota protegida `/dashboard` que requer um token JWT válido assinado com a chave 'segredo-secreto' passada no cabeçalho 'Authorization: Bearer <token>'.",
        "dir": "sim15_jwt_middleware"
    },
    {
        "id": 16,
        "name": "SQLite ACID Transaction (Complex)",
        "prompt": "Crie um script Python 'transfer.py' que conecta ao SQLite 'bank.db', cria a tabela 'contas' (id, saldo) se não existir, e executa uma transferência de 100 reais da conta 1 para a conta 2 de forma transacional segura (ACID), revertendo a transação (rollback) se a conta 1 não possuir saldo suficiente.",
        "dir": "sim16_sqlite_transaction"
    },
    {
        "id": 17,
        "name": "Go Concurrent Web Crawler (Complex)",
        "prompt": "Crie um programa Go em 'crawler.go' que percorre a estrutura de subpastas a partir da pasta atual e conta concorrentemente a ocorrência da palavra 'TODO' em todos os arquivos `.txt` usando goroutines e canais, imprimindo o total geral no console.",
        "dir": "sim17_concurrent_crawler"
    },
    {
        "id": 18,
        "name": "Python Regex Email Extractor (Medium)",
        "prompt": "Crie um script Python 'parse_emails.py' que lê um arquivo 'contacts.txt', encontra todos os endereços de email válidos usando expressões regulares e salva a lista única de emails ordenada alfabeticamente em 'emails.txt'.",
        "dir": "sim18_email_extractor"
    },
    {
        "id": 19,
        "name": "Node.js CSV Stream Filter (Advanced)",
        "prompt": "Crie um script Node.js 'stream_processor.js' que lê um arquivo CSV grande 'input.csv' (contendo colunas name, age, active) usando a API de streams do Node.js (`fs.createReadStream`), filtra as linhas onde age >= 18 e grava no arquivo 'output.csv' sem carregar todo o arquivo na memória.",
        "dir": "sim19_csv_stream"
    },
    {
        "id": 20,
        "name": "Go Exec Command Timeout (Advanced)",
        "prompt": "Crie um script Go em 'runner.go' que executa um comando do sistema recebido via argumento CLI (ex: `runner.go -cmd \"sleep 5\"`) usando o pacote `os/exec` e garante que ele seja interrompido (killed) com segurança se demorar mais de 2 segundos, imprimindo stdout ou timeout.",
        "dir": "sim20_exec_timeout"
    },
    {
        "id": 21,
        "name": "Python XOR/AES File Cipher (Advanced)",
        "prompt": "Crie um script Python 'cipher.py' que lê um arquivo 'secret.txt', faz a criptografia de seu conteúdo usando uma chave armazenada em 'key.bin' via cifra XOR ou AES (caso use a biblioteca padrão) e grava a saída em 'secret.enc'. O script deve suportar a flag `--decrypt`.",
        "dir": "sim21_file_cipher"
    },
    {
        "id": 22,
        "name": "SQL Migrations Runner (Complex)",
        "prompt": "Crie um script Python 'migrate.py' que lê todos os arquivos `.sql` dentro de uma pasta 'migrations/' (ex: '01_init.sql', '02_add_field.sql'), executa-os em ordem alfanumérica dentro de uma única transação SQLite no banco 'app.db' e mantém uma tabela de controle 'schema_migrations' para evitar re-execução.",
        "dir": "sim22_migrations_runner"
    },
    {
        "id": 23,
        "name": "BeautifulSoup HTML Parser (Medium)",
        "prompt": "Crie um script Python 'parser.py' usando `BeautifulSoup` ou a biblioteca padrão `html.parser` que lê um arquivo HTML local 'page.html', extrai todos os hiperlinks (`<a>`) e cabeçalhos (`<h1>` a `<h3>`) e salva essas informações estruturadas em um arquivo JSON 'extracted.json'.",
        "dir": "sim23_html_parser"
    },
    {
        "id": 24,
        "name": "Go TCP Reverse Echo Server (Advanced)",
        "prompt": "Crie um servidor TCP em Go em 'tcp_server.go' na porta 8089 que escuta conexões, aceita múltiplos clientes concorrentemente usando goroutines, lê strings enviadas por eles, inverte a string recebida (reverse string) e devolve a resposta ao cliente.",
        "dir": "sim24_tcp_server"
    },
    {
        "id": 25,
        "name": "Node.js Markdown to HTML Converter (Advanced)",
        "prompt": "Crie um script Node.js 'md2html.js' que lê um arquivo 'documento.md' e o converte para HTML estruturado (tags `p`, `h1`, `h2`, `ul`, `li`, `strong`) sem usar dependências externas, salvando em 'documento.html'.",
        "dir": "sim25_markdown_converter"
    },
    {
        "id": 26,
        "name": "Python CSV to SQLite Importer (Medium)",
        "prompt": "Crie um script Python 'import_csv.py' que lê um arquivo CSV 'usuarios.csv' (colunas name, email, role) e o importa para uma tabela 'usuarios' no banco SQLite 'data.db', tratando emails duplicados (ignorar ou atualizar).",
        "dir": "sim26_csv_importer"
    },
    {
        "id": 27,
        "name": "Go Concurrent Memory Cache TTL (Complex)",
        "prompt": "Implemente em Go em 'cache.go' um cache em memória thread-safe (`Cache`) que aceita chaves e valores com expiração (TTL). A struct deve ter métodos `Set(key string, val interface{}, ttl time.Duration)` e `Get(key string) (interface{}, bool)`. Escreva testes ou exemplo no main.",
        "dir": "sim27_memory_cache"
    },
    {
        "id": 28,
        "name": "Python Path Traversal Sanitizer (Advanced)",
        "prompt": "Crie um script Python 'sandbox.py' que aceita um caminho de arquivo dinâmico via argumento CLI, sanitiza-o de forma segura contra Path Traversal (evitando bypasses com `../` ou caminhos absolutos fora da área permitida) e lê o arquivo apenas se estiver estritamente contido dentro da subpasta local 'safe_dir/'.",
        "dir": "sim28_path_sanitizer"
    },
    {
        "id": 29,
        "name": "Yii2 REST Controller SQLite (Complex)",
        "prompt": "Crie a estrutura de arquivos Yii2 MVC e adicione um controller RESTful em 'controllers/PostController.php' que estende `yii\rest\ActiveController` e gerencia o model 'app\\models\\Post' em uma tabela SQLite configurada.",
        "dir": "sim29_yii2_rest"
    },
    {
        "id": 30,
        "name": "Python CLI Task Manager (Medium)",
        "prompt": "Crie um script Python 'cli.py' usando `argparse` que implementa um gerenciador de tarefas CLI com subcomandos `add \"titulo\"`, `list` e `done <id>` salvando o estado das tarefas em formato JSON em um arquivo 'tasks.json'.",
        "dir": "sim30_cli_manager"
    },
    {
        "id": 31,
        "name": "Legal Contract Drafting (Law/Legal)",
        "prompt": "Crie um arquivo 'contrato_prestacao_servicos.md' contendo um rascunho de contrato de prestação de serviços de consultoria de marketing de acordo com a legislação brasileira. O contrato deve incluir cláusulas de objeto, obrigações, preço, rescisão e foro, formatado profissionalmente em markdown.",
        "dir": "sim31_legal_contract"
    },
    {
        "id": 32,
        "name": "Financial Expense Report (Finance/Business)",
        "prompt": "Gere um arquivo CSV 'despesas_viagem.csv' com 10 despesas de viagem fictícias (colunas: Data, Categoria, Descrição, Valor). Crie um script Python 'resumo_financeiro.py' que lê o CSV, calcula o gasto total, a média por despesa, e gera um relatório formatado em 'resumo.txt'.",
        "dir": "sim32_expense_report"
    },
    {
        "id": 33,
        "name": "Recipe Book Scaler (Gastronomy)",
        "prompt": "Crie um arquivo JSON 'receitas.json' com 3 receitas culinárias (ingredientes e instruções). Escreva um script Python 'redimensionar_receita.py' que lê as receitas, pede um fator multiplicador via CLI (ou usa fator padrão 2 se não informado) e escreve uma versão escalada dos ingredientes das receitas em 'receitas_escaladas.txt'.",
        "dir": "sim33_recipe_scaler"
    },
    {
        "id": 34,
        "name": "Language Translation & Localization (Linguistics)",
        "prompt": "Crie um arquivo 'documento_pt.txt' com um parágrafo sobre a história do café no Brasil. Escreva um script Python 'tradutor.py' (ou use Go/Node) que lê o arquivo e gera versões traduzidas desse mesmo texto para inglês em 'documento_en.txt' e espanhol em 'documento_es.txt' utilizando substituições simples ou mock de tradução mantendo o sentido das frases principais.",
        "dir": "sim34_translation"
    },
    {
        "id": 35,
        "name": "Academic Bibliography Formatter (Education/ABNT)",
        "prompt": "Crie um arquivo JSON 'referencias.json' contendo dados de 3 livros (autor, título, editora, ano, cidade). Escreva um script Python 'formatador_abnt.py' que lê o JSON e gera a lista de referências bibliográficas no padrão ABNT (ex: SOBRENOME, Nome. *Título*. Cidade: Editora, Ano.) salva em 'bibliografia.txt'.",
        "dir": "sim35_abnt_bibliography"
    },
    {
        "id": 36,
        "name": "Weekly Social Media Calendar (Marketing)",
        "prompt": "Gere um arquivo Markdown 'calendario_editorial.md' contendo uma tabela com o plano de postagens de redes sociais de uma marca de café para 7 dias da semana (colunas: Dia, Rede Social, Tema do Post, Texto do Post, Hashtags sugeridas, Horário sugerido). O conteúdo deve ser criativo e engajador.",
        "dir": "sim36_marketing_calendar"
    },
    {
        "id": 37,
        "name": "Real Estate Listing Generator (Real Estate)",
        "prompt": "Crie um script Python 'gerador_anuncios.py' que lê dados de imóveis de um arquivo JSON 'imoveis.json' (tipo, bairro, quartos, preço, diferenciais) e gera descrições de anúncios de venda persuasivas voltadas para plataformas online, salvando a saída formatada em 'anuncios.txt'.",
        "dir": "sim37_real_estate"
    },
    {
        "id": 38,
        "name": "Customer Support Response Templates (Customer Support)",
        "prompt": "Crie um arquivo Markdown 'respostas_suporte.md' contendo templates de resposta por escrito profissionais e empáticos para 5 cenários comuns de reclamação de clientes: (1) atraso na entrega, (2) produto defeituoso, (3) cobrança indevida, (4) elogio, (5) solicitação de cancelamento.",
        "dir": "sim38_customer_support"
    },
    {
        "id": 39,
        "name": "Logistics Route & Load Planner (Supply Chain)",
        "prompt": "Crie um script Python 'planejador_rotas.py' que lê uma lista de entregas de 'entregas.json' (cliente, peso, cidade, distancia) e calcula a sequência ideal de entregas para maximizar a capacidade de carga de um veículo de 1000kg e ordenar as paradas pela menor distância total, escrevendo o manifesto de carga em 'manifesto.txt'.",
        "dir": "sim39_logistics_planner"
    },
    {
        "id": 40,
        "name": "Workout Routine & Calorie Calculator (Health/Fitness)",
        "prompt": "Crie um script Python 'treino.py' que recebe o peso corporal e o nível de atividade física do usuário e escreve um plano de treino semanal dividido em 4 dias em 'plano_treino.txt', calculando também uma estimativa de calorias diárias necessárias.",
        "dir": "sim40_fitness_workout"
    },
    {
        "id": 41,
        "name": "Nutrition Meal Planner (Health/Nutrition)",
        "prompt": "Gere um plano alimentar semanal de 7 dias para uma dieta de emagrecimento saudável (1800kcal) em 'plano_alimentar.md'. O arquivo deve incluir café da manhã, almoço, lanche da tarde e jantar, com porções estimadas e macros calculados (carboidratos, proteínas, gorduras).",
        "dir": "sim41_meal_planner"
    },
    {
        "id": 42,
        "name": "Inventory Stock Management Report (Retail)",
        "prompt": "Escreva um script Python 'estoque.py' que lê um estoque inicial em 'estoque.json' (produto, quantidade, preco_custo, preco_venda), aplica uma lista de vendas gravadas em 'vendas_dia.csv' e gera o relatório final 'balanco.txt' indicando quantidade restante, faturamento bruto e lucro do dia.",
        "dir": "sim42_inventory_report"
    },
    {
        "id": 43,
        "name": "Travel Itinerary Planner (Tourism/Hospitality)",
        "prompt": "Crie um arquivo Markdown 'roteiro_viagem.md' com um itinerário detalhado de 5 dias em Roma. O roteiro deve conter o que visitar de manhã, tarde e noite, recomendações de restaurantes locais clássicos e estimativa de custos de transporte e ingressos em Euros.",
        "dir": "sim43_travel_itinerary"
    },
    {
        "id": 44,
        "name": "Product Launch Press Release (Public Relations)",
        "prompt": "Escreva um comunicado de imprensa profissional (Press Release) em 'press_release.md' anunciando o lançamento de uma linha ecológica de copos térmicos biodegradáveis. Siga o formato padrão de jornalismo (Título, Subtítulo, Local, Data, Lide, Citações fictícias do CEO e Informações de Contato).",
        "dir": "sim44_press_release"
    },
    {
        "id": 45,
        "name": "Book Review Organizer (Literature/Arts)",
        "prompt": "Crie um script Python 'resenha.py' que lê uma resenha de livro em 'resenha_bruta.txt', faz a contagem de palavras, identifica os nomes dos personagens principais citados e gera uma ficha de leitura estruturada formatada em 'resenha_final.md'.",
        "dir": "sim45_book_review"
    },
    {
        "id": 46,
        "name": "Classroom Lesson Plan (Education/Teaching)",
        "prompt": "Gere um plano de aula de biologia de 50 minutos sobre 'Introdução à Genética Mendeliana' e salve em 'plano_aula.md'. O plano deve incluir objetivos de aprendizagem, recursos didáticos necessários, cronograma de atividades minuto a minuto e um conjunto de 3 questões de fixação.",
        "dir": "sim46_lesson_plan"
    },
    {
        "id": 47,
        "name": "Retail Product Pricing Calculator (Retail/Finance)",
        "prompt": "Crie um script Python 'precificacao.py' que calcula o preço de venda ideal de um produto com base em seu custo de fabricação, impostos (ex: 18%), margem de lucro desejada (ex: 30%) e custos de frete informados via JSON 'custos.json', salvando a memória de cálculo em 'preco_venda.txt'.",
        "dir": "sim47_product_pricing"
    },
    {
        "id": 48,
        "name": "Employee Shift Scheduler (Human Resources)",
        "prompt": "Crie um script Python 'escala.py' que monta uma escala semanal de trabalho (de segunda a domingo) para uma equipe de 5 atendentes em 'escala_trabalho.txt', garantindo que cada atendente tenha pelo menos 1 folga na semana e que sempre haja no mínimo 2 atendentes trabalhando por dia.",
        "dir": "sim48_employee_scheduler"
    },
    {
        "id": 49,
        "name": "Email Newsletter Draft (Content Writing)",
        "prompt": "Escreva uma newsletter de email promocional atraente em 'newsletter.html' com CSS básico integrado anunciando a Black Friday de uma livraria virtual. A newsletter deve incluir seções de ofertas principais, cupons de desconto e links para categorias populares de livros.",
        "dir": "sim49_email_newsletter"
    },
    {
        "id": 50,
        "name": "Project Kickoff Meeting Minutes (Business Administration)",
        "prompt": "Crie um arquivo Markdown 'ata_kickoff.md' detalhando a ata de uma reunião de início de projeto de um aplicativo de entregas de padaria local. A ata deve conter a pauta, resumo das discussões de cada participante, uma lista de tarefas atribuídas a cada membro da equipe e a data agendada para a próxima reunião de alinhamento.",
        "dir": "sim50_meeting_minutes"
    },
    {
        "id": 51,
        "name": "Wikipedia Search & Summary (Research)",
        "prompt": "Use o Pesquisador ou Scraper para buscar a página da Wikipédia sobre o 'Telescópio Espacial James Webb', extraia a data de lançamento, os principais espelhos e objetivos científicos e salve em um relatório estruturado 'jwst_resumo.md'.",
        "dir": "sim51_jwst_wiki"
    },
    {
        "id": 52,
        "name": "Responsive Website Screenshot (Browser/Visual)",
        "prompt": "Abra o site 'https://example.com' usando o browser, ajuste a resolução para celular (ex: 375x812), tire um print da tela inteira salvando em 'example_mobile.png' e verifique se o título principal está visível.",
        "dir": "sim52_screenshot_mobile"
    },
    {
        "id": 53,
        "name": "GitHub Repository Info Search (Research)",
        "prompt": "Acesse o site 'https://github.com/yiisoft/yii2' (ou pesquise sobre ele), descubra o número atual de estrelas (stars), forks e a versão da última release publicada, salvando essas métricas em 'yii2_metrics.json'.",
        "dir": "sim53_github_metrics"
    },
    {
        "id": 54,
        "name": "Weather Forecast Finder (Browser/Web)",
        "prompt": "Use o Browser ou Pesquisador para acessar um site de previsão do tempo (ex: ClimaTempo ou similar) para a cidade de São Paulo, obtenha a previsão para os próximos 3 dias (temperatura mínima/máxima e condição) e salve em 'previsao_sp.txt'.",
        "dir": "sim54_weather_forecast"
    },
    {
        "id": 55,
        "name": "Exchange Rate Scraper (Finance/Web)",
        "prompt": "Acesse um site de cotação financeira (ou pesquise na web) para obter o valor atual de conversão do Dólar (USD) e Euro (EUR) para Real (BRL). Crie um arquivo 'cotacoes.json' com os valores obtidos e a data/hora da consulta.",
        "dir": "sim55_exchange_rates"
    },
    {
        "id": 56,
        "name": "Online PDF Guide Downloader (Web)",
        "prompt": "Pesquise na internet por um manual ou guia em PDF público sobre o protocolo HTTP/2 (ou cheatsheet de comandos git). Baixe o arquivo PDF e salve-o no workspace como 'guia.pdf', verificando se ele possui um tamanho maior que 10KB.",
        "dir": "sim56_pdf_download"
    },
    {
        "id": 57,
        "name": "Search Engine Rank Checker (Marketing/SEO)",
        "prompt": "Use o browser para pesquisar por 'best open source LLM agent framework' no Google (ou buscador equivalente), encontre quais são os 3 primeiros links orgânicos que aparecem nos resultados e salve os títulos e URLs em 'seo_results.json'.",
        "dir": "sim57_seo_search"
    },
    {
        "id": 58,
        "name": "Tech News Summary (Research)",
        "prompt": "Visite um portal de notícias de tecnologia (ex: Hacker News ou TechCrunch), extraia o título das 5 principais notícias da página inicial e crie um rascunho de newsletter em 'tech_news.md' com uma breve introdução.",
        "dir": "sim58_news_summary"
    },
    {
        "id": 59,
        "name": "Doc Downloader & Link Extractor (Web/Doc)",
        "prompt": "Acesse la documentação oficial do Go sobre 'Effective Go' (https://go.dev/doc/effective_go), use o Scraper para converter a seção 'Formatting' em Markdown e extraia todos os links externos citados nessa seção, salvando em 'formatting_links.txt'.",
        "dir": "sim59_go_doc_formatting"
    },
    {
        "id": 60,
        "name": "E-commerce Price Comparer (Web/Shopping)",
        "prompt": "Use o browser para pesquisar e comparar o preço de uma 'Placa de Vídeo RTX 4060' em duas lojas online diferentes (ex: Kabum, Pichau ou via buscador de preços), salvando os links das ofertas e os valores encontrados em 'rtx4060_precos.json'.",
        "dir": "sim60_price_comparison"
    },
    {
        "id": 61,
        "name": "Public API Schema Fetcher (Research/API)",
        "prompt": "Pesquise na internet a URL pública do schema JSON de especificação da API do OpenAPI (Swagger). Faça o download do schema e salve em 'openapi_schema.json', garantindo que seja um JSON sintaticamente válido.",
        "dir": "sim61_openapi_schema"
    },
    {
        "id": 62,
        "name": "Visual Layout Bug Detector (Browser)",
        "prompt": "Navegue até a página 'https://www.wikipedia.org/', tire um print da área principal que contém as línguas (globe) salvando em 'wikipedia_globe.png' e escreva em um arquivo 'layout_report.txt' se a imagem principal está perfeitamente alinhada no centro horizontal.",
        "dir": "sim62_layout_check"
    },
    {
        "id": 63,
        "name": "Trending Topics Finder (Social Media)",
        "prompt": "Pesquise na web (ex: Google Trends ou portais de notícias) os 3 tópicos mais comentados (trending topics) no Brasil nas últimas 24 horas e escreva um post de blog curto em 'trends_post.md' comentando sobre a importância desses assuntos.",
        "dir": "sim63_trending_topics"
    },
    {
        "id": 64,
        "name": "Stock Market Ticker Research (Finance)",
        "prompt": "Use o buscador ou browser para consultar o preço de fechamento atual da ação da Apple (AAPL) e da Microsoft (MSFT) na bolsa de valores, salvando o ticker, preço e variação percentual do dia em 'stocks_report.md'.",
        "dir": "sim64_stocks_ticker"
    },
    {
        "id": 65,
        "name": "Online HTML Form Filler (Browser)",
        "prompt": "Use o Browser para acessar uma página de formulário de teste público (ex: formulário de contato do example ou ferramenta de testes) e simule o preenchimento dos campos Nome, Email e Mensagem com dados fictícios. Tire uma captura de tela do formulário preenchido antes do envio e salve em 'form_preenchido.png'.",
        "dir": "sim65_form_filler"
    },
    {
        "id": 66,
        "name": "Official Python PEP Search (Research)",
        "prompt": "Use o pesquisador web para descobrir os detalhes do PEP 8 (Style Guide for Python Code): quem são os autores, o ano de publicação original e o status atual. Crie um resumo dessa especificação em 'pep8_info.md'.",
        "dir": "sim66_pep8_research"
    },
    {
        "id": 67,
        "name": "Local Business Info Scraper (Browser)",
        "prompt": "Use o Browser para buscar uma padaria ou restaurante famoso no bairro dos Jardins em São Paulo. Extraia o nome, endereço completo, telefone de contato e horário de funcionamento, salvando as informações estruturadas em 'restaurante_info.json'.",
        "dir": "sim67_restaurant_scraper"
    },
    {
        "id": 68,
        "name": "Static Assets Downloader (Web)",
        "prompt": "Acesse o site 'https://www.google.com', encontre a URL da imagem oficial do logotipo (logo) do Google exibida na página inicial, faça o download do arquivo de imagem e salve-o como 'google_logo.png'.",
        "dir": "sim68_google_logo"
    },
    {
        "id": 69,
        "name": "Security Policy Checker (Research)",
        "prompt": "Acesse a página inicial do site 'https://www.cloudflare.com/' ou pesquise sobre as diretrizes de segurança deles. Localize a URL do arquivo de política de segurança 'security.txt' (normalmente em `/.well-known/security.txt`), baixe seu conteúdo e salve-o em 'cloudflare_security.txt'.",
        "dir": "sim69_security_policy"
    },
    {
        "id": 70,
        "name": "Conference Schedule Scraper (Research)",
        "prompt": "Busque a programação oficial (schedule) da última conferência 'GopherCon Brazil' ou 'PyCon US' na internet. Extraia o título de 3 palestras principais, os nomes dos palestrantes e os horários, gerando a grade de eventos em 'grade_palestras.md'.",
        "dir": "sim70_conference_schedule"
    }
]

def load_env_vars():
    env_file = Path("/home/j/.crom/.env")
    env = os.environ.copy()
    if env_file.exists():
        with open(env_file) as f:
            for line in f:
                if "=" in line and not line.strip().startswith("#"):
                    k, v = line.strip().split("=", 1)
                    env[k] = v
    return env

def get_session_stats(workspace_dir):
    crom_dir = workspace_dir / ".crom"
    state_file = crom_dir / ".crom_state.json"
    if state_file.exists():
        try:
            with open(state_file) as f:
                data = json.load(f)
                return {
                    "total_turns": data.get("total_turnos", 0),
                    "tokens_spent": data.get("tokens_gastos", 0),
                    "status": data.get("status_operacional", data.get("ultimo_status", "unknown")),
                    "cognitive_mode": data.get("modo_cognitive", data.get("modo_cognitivo", "unknown"))
                }
        except Exception as e:
            print(f"Error reading .crom_state.json: {e}")
            
    sessions_dir = crom_dir / "sessions"
    if sessions_dir.exists():
        sessions = sorted(list(sessions_dir.glob("session-*")), key=os.path.getmtime)
        if sessions:
            session_json = sessions[-1] / "session.json"
            if not session_json.exists():
                session_json = sessions[-1] / ".crom_state.json"
            if session_json.exists():
                try:
                    with open(session_json) as f:
                        data = json.load(f)
                        return {
                            "total_turns": data.get("total_turnos", 0),
                            "tokens_spent": data.get("tokens_gastos", 0),
                            "status": data.get("status_operacional", data.get("ultimo_status", "unknown")),
                            "cognitive_mode": data.get("modo_cognitive", data.get("modo_cognitivo", "unknown"))
                        }
                except Exception as e:
                    print(f"Error reading session file: {e}")
    return {}

import urllib.request
import urllib.error

def check_preflight(provider, env):
    print("\n==================================================")
    print("Realizando verificações pré-run (Preflight Checks)")
    print("==================================================")
    
    # 1. Check API Key
    key_name = None
    if provider == "openrouter":
        key_name = "OPENROUTER_API_KEY"
    elif provider == "openai":
        key_name = "OPENAI_API_KEY"
    elif provider == "gemini":
        key_name = "GEMINI_API_KEY"
    elif provider == "anthropic":
        key_name = "ANTHROPIC_API_KEY"

    if key_name:
        if key_name not in env or not env[key_name].strip():
            print(f"❌ ERRO: A variável de ambiente '{key_name}' não está definida ou está vazia.", file=sys.stderr)
            return False
        print(f"✓ Variável de ambiente '{key_name}' encontrada.")

    # 2. Check Network Connection/Reachability
    test_url = "https://openrouter.ai/api/v1/models" if provider == "openrouter" else "https://www.google.com"
    print(f"Testando conectividade de rede com {test_url}...")
    try:
        req = urllib.request.Request(
            test_url,
            headers={"User-Agent": "Cromia-Simulation-Preflight"}
        )
        with urllib.request.urlopen(req, timeout=5) as response:
            if response.status == 200:
                print("✓ Conectividade de rede estabelecida com sucesso.")
                return True
    except Exception as e:
        print(f"❌ ERRO: Falha ao conectar com o provedor/internet ({test_url}): {e}", file=sys.stderr)
        return False
    return True

def run_simulation(sim, env, provider, model, max_iterations, timeout=None):
    sim_dir = SIMS_DIR / sim["dir"]
    
    # Clean workspace folder if it exists
    if sim_dir.exists():
        shutil.rmtree(sim_dir)
    sim_dir.mkdir(parents=True, exist_ok=True)
    
    # Determina o timeout específico da simulação
    actual_timeout = timeout if timeout is not None else (180 if sim["id"] >= 8 else 120)
    
    print(f"\n==================================================")
    print(f"Iniciando Simulação {sim['id']}: {sim['name']}")
    print(f"Workspace: {sim_dir}")
    print(f"Prompt: {sim['prompt']}")
    print(f"Timeout: {actual_timeout}s")
    print(f"==================================================")
    
    cmd = [
        str(BINARY_PATH), "run", sim["prompt"],
        "--provider", provider,
        "--model", model,
        "--workspace", str(sim_dir),
        "--permission-mode", "total_access",
        "--max-iterations", str(max_iterations),
        "--disable-prompt-optimization"
    ]
    
    start_time = time.time()
    return_code = -1
    try:
        res = subprocess.run(cmd, env=env, cwd=str(sim_dir), capture_output=True, text=True, errors="replace", timeout=actual_timeout)
        elapsed = time.time() - start_time
        success = res.returncode == 0
        return_code = res.returncode
        output = res.stdout + "\n" + res.stderr
    except subprocess.TimeoutExpired as te:
        elapsed = time.time() - start_time
        success = False
        output = f"TIMEOUT EXPIRED: {te}\nSTDOUT: {te.stdout or ''}\nSTDERR: {te.stderr or ''}"
    
    print(f"Tempo decorrido: {elapsed:.2f}s")
    print(f"Código de retorno: {return_code if return_code != -1 else 'timeout'}")
    
    stats = get_session_stats(sim_dir)
    
    return {
        "id": sim["id"],
        "name": sim["name"],
        "prompt": sim["prompt"],
        "dir": sim["dir"],
        "elapsed_seconds": elapsed,
        "success": success,
        "total_turns": stats.get("total_turns", 0),
        "tokens_spent": stats.get("tokens_spent", 0),
        "status": stats.get("status", "unknown"),
        "cognitive_mode": stats.get("cognitive_mode", "unknown"),
        "output_snippet": output[-500:] if len(output) > 500 else output
    }

def main():
    import argparse
    parser = argparse.ArgumentParser(description="Executa as simulações de templates de projeto.")
    parser.add_argument("--model", type=str, default="meta-llama/llama-3.1-8b-instruct", help="Modelo de LLM a usar (separe por vírgula para rodar múltiplos)")
    parser.add_argument("--provider", type=str, default="openrouter", help="Provedor de LLM")
    parser.add_argument("--max-iterations", type=int, default=0, help="Limite máximo de iterações (0 = ilimitado)")
    parser.add_argument("--timeout", type=int, default=None, help="Tempo limite customizado (em segundos) para cada simulação")
    parser.add_argument("--ids", type=str, default=None, help="IDs específicos de simulações a rodar (ex: 1,2,3 ou 68-70)")
    parser.add_argument("--resume", action="store_true", help="Continua a partir de simulações que ainda não rodaram ou falharam no resumo anterior")
    args = parser.parse_args()

    SIMS_DIR.mkdir(parents=True, exist_ok=True)
    env = load_env_vars()
    env["CROM_PERMISSION_MODE"] = "total_access"
    
    # Executar verificações pré-run
    if not check_preflight(args.provider, env):
        print("\n❌ ERRO: Verificações pré-run falharam. Abortando execução.", file=sys.stderr)
        sys.exit(1)
    
    # Parse IDs to run
    target_ids = None
    if args.ids:
        target_ids = set()
        for part in args.ids.split(","):
            part = part.strip()
            if "-" in part:
                start, end = part.split("-")
                target_ids.update(range(int(start), int(end) + 1))
            else:
                target_ids.add(int(part))

    models = [m.strip() for m in args.model.split(",") if m.strip()]
    all_model_results = {}
    
    for idx, model in enumerate(models):
        print(f"\n==================================================")
        print(f"INICIANDO SIMULAÇÕES PARA O MODELO: {model} ({idx+1}/{len(models)})")
        print(f"==================================================")
        
        model_safe = model.replace("/", "_").replace(":", "_")
        summary_file = SIMS_DIR / f"simulations_summary_{model_safe}.json"
        
        # Carregar resultados existentes se resume ou ids ativados
        existing_runs = {}
        if summary_file.exists():
            try:
                with open(summary_file) as f:
                    for run in json.load(f):
                        existing_runs[run["id"]] = run
            except Exception as e:
                print(f"Aviso ao carregar summary_file existente: {e}")

        results = []
        for sim in SIMULATIONS:
            sim_id = sim["id"]
            
            # Decidir se deve rodar esta simulação
            should_run = True
            if target_ids is not None and sim_id not in target_ids:
                should_run = False
            elif args.resume and sim_id in existing_runs and existing_runs[sim_id].get("success", False):
                should_run = False
                
            if should_run:
                res = run_simulation(sim, env, args.provider, model, args.max_iterations, args.timeout)
                existing_runs[sim_id] = res
                time.sleep(2)
            
            # Adicionar o resultado ao estado atual (seja o novo rodado ou o existente/dummy)
            if sim_id in existing_runs:
                results.append(existing_runs[sim_id])
            else:
                results.append({
                    "id": sim_id,
                    "name": sim["name"],
                    "prompt": sim["prompt"],
                    "dir": sim["dir"],
                    "elapsed_seconds": 0.0,
                    "success": False,
                    "total_turns": 0,
                    "tokens_spent": 0,
                    "status": "not_run",
                    "cognitive_mode": "unknown",
                    "output_snippet": ""
                })
                
            # Gravar progresso imediato no JSON
            with open(summary_file, "w") as f:
                json.dump(results, f, indent=2)
                
            # Gerar relatório Markdown do progresso atual
            report_file = SIMS_DIR / f"simulations_report_{model_safe}.md"
            md = []
            md.append(f"# Relatório de Simulações do crom-agente via {args.provider.capitalize()}")
            md.append(f"\nData de execução: {time.strftime('%Y-%m-%d %H:%M:%S')}")
            md.append(f"Modelo utilizado: `{model}` via {args.provider}")
            md.append("\n## Tabela de Resultados das Simulações")
            md.append("\n| ID | Nome da Simulação | Status Final | Modo Cognitivo | Turnos | Tokens Gasto | Tempo (s) | Sucesso |")
            md.append("|---|---|---|---|---|---|---|---|")
            
            total_tokens = 0
            total_time = 0.0
            successful_runs = 0
            
            for r in results:
                succ_emoji = "✅" if r["success"] else "❌"
                md.append(f"| {r['id']} | {r['name']} | `{r['status']}` | `{r['cognitive_mode']}` | {r['total_turns']} | {r['tokens_spent']} | {r['elapsed_seconds']:.2f}s | {succ_emoji} |")
                total_tokens += r["tokens_spent"]
                total_time += r["elapsed_seconds"]
                if r["success"]:
                    successful_runs += 1
                    
            price_per_1m = 0.055 if "8b" in model.lower() or "9b" in model.lower() else 0.075
            md.append(f"\n### Métricas Consolidadas")
            md.append(f"- **Simulações Executadas/Registradas**: {len(results)}")
            md.append(f"- **Taxa de Sucesso**: {successful_runs}/{len(results)} ({successful_runs/len(results)*100:.1f}%)")
            md.append(f"- **Total de Tokens Consumidos**: {total_tokens}")
            md.append(f"- **Custo Estimado**: ${(total_tokens / 1000000) * price_per_1m:.6f} USD (baseado em ${price_per_1m} por 1M tokens)")
            md.append(f"- **Tempo Total de Execução**: {total_time:.2f} segundos")
            md.append(f"- **Média de Tempo por Simulação**: {total_time/len(results):.2f} segundos")
            
            with open(report_file, "w") as f:
                f.write("\n".join(md))
                
        print(f"\n✓ Relatório e resumo salvos para o modelo {model}")
        all_model_results[model] = results

    # Gerar relatório comparativo automático se houver múltiplos modelos
    if len(models) > 1:
        comp_report_file = SIMS_DIR / "simulations_comparative_report.md"
        comp_md = []
        comp_md.append("# Relatório Comparativo de Modelos (Simulações)")
        comp_md.append(f"\nData de execução: {time.strftime('%Y-%m-%d %H:%M:%S')}")
        comp_md.append(f"Provedor: `{args.provider}`\n")
        
        # Tabela resumo comparativa geral
        comp_md.append("## Resumo Geral dos Modelos\n")
        comp_md.append("| Modelo | Taxa de Sucesso | Tokens Totais | Tempo Total (s) | Média por Simulação (s) |")
        comp_md.append("|---|---|---|---|---|")
        for model in models:
            m_res = all_model_results[model]
            succ = sum(1 for r in m_res if r["success"])
            tok = sum(r["tokens_spent"] for r in m_res)
            t_total = sum(r["elapsed_seconds"] for r in m_res)
            comp_md.append(f"| `{model}` | {succ}/{len(m_res)} ({succ/len(m_res)*100:.1f}%) | {tok} | {t_total:.2f}s | {t_total/len(m_res):.2f}s |")
            
        # Detalhe por simulação
        comp_md.append("\n## Detalhamento Lado-a-Lado por Simulação\n")
        
        # Cabeçalho dinâmico para os modelos
        header_cols = ["ID", "Nome da Simulação"]
        sub_header = ["---|---"]
        for m in models:
            m_short = m.split("/")[-1]
            header_cols.extend([f"[{m_short}] Sucesso", f"[{m_short}] Turnos", f"[{m_short}] Tempo"])
            sub_header.extend(["---|---|---"])
        
        comp_md.append("| " + " | ".join(header_cols) + " |")
        comp_md.append("| " + " | ".join(sub_header) + " |")
        
        for idx in range(len(SIMULATIONS)):
            sim = SIMULATIONS[idx]
            row = [str(sim["id"]), sim["name"]]
            for model in models:
                res_sim = all_model_results[model][idx]
                succ_emoji = "✅" if res_sim["success"] else "❌"
                row.extend([succ_emoji, str(res_sim["total_turns"]), f"{res_sim['elapsed_seconds']:.1f}s"])
            comp_md.append("| " + " | ".join(row) + " |")
            
        with open(comp_report_file, "w") as f:
            f.write("\n".join(comp_md))
        print(f"\n==================================================")
        print(f"✓ Relatório Comparativo de Modelos gravado em: {comp_report_file}")
        print(f"==================================================")

if __name__ == "__main__":
    main()
