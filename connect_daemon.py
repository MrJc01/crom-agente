
import os
import sys
import requests
import socket
from urllib.parse import urlparse

def check_http_daemon(url, timeout=5):
    """
    Verifica a conectividade com um daemon HTTP/HTTPS.
    """
    try:
        response = requests.get(url, timeout=timeout)
        response.raise_for_status()  # Levanta um HTTPError para códigos de status 4xx/5xx
        print(f"Sucesso: Conexão HTTP/HTTPS com {url} bem-sucedida. Status: {response.status_code}")
        return True
    except requests.exceptions.RequestException as e:
        print(f"Falha: Não foi possível conectar ao daemon HTTP/HTTPS em {url}. Erro: {e}")
        return False

def check_tcp_daemon(host, port, timeout=5):
    """
    Verifica a conectividade com um daemon TCP bruto.
    """
    try:
        with socket.create_connection((host, port), timeout=timeout) as sock:
            print(f"Sucesso: Conexão TCP com {host}:{port} bem-sucedida.")
            return True
    except (socket.timeout, ConnectionRefusedError, OSError) as e:
        print(f"Falha: Não foi possível conectar ao daemon TCP em {host}:{port}. Erro: {e}")
        return False

def main():
    daemon_url = os.getenv("URL_DO_DAEMON")
    if not daemon_url:
        print("Erro: A variável de ambiente URL_DO_DAEMON não está definida.")
        sys.exit(1)

    # Assumindo HTTP por padrão, será ajustado com base na resposta do usuário
    # ou podemos tentar inferir (melhor aguardar input para robustez)
    # Por enquanto, farei uma pequena inferência provisória para ter algo funcionando.

    parsed_url = urlparse(daemon_url)
    if parsed_url.scheme in ["http", "https"]:
        check_http_daemon(daemon_url)
    elif parsed_url.scheme == "tcp":
        if not parsed_url.hostname or not parsed_url.port:
            print(f"Erro: URL TCP inválida. Formato esperado: tcp://host:port. Recebido: {daemon_url}")
            sys.exit(1)
        check_tcp_daemon(parsed_url.hostname, parsed_url.port)
    else:
        print(f"Erro: Esquema de URL não suportado: {parsed_url.scheme}. Suportado: http, https, tcp.")
        sys.exit(1)

if __name__ == "__main__":
    main()
