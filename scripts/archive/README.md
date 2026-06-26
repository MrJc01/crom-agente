# Arquivos Scratch (Archive)

Estes scripts foram criados durante o desenvolvimento e depuração do motor central do `crom-agente`. Eles foram movidos para esta pasta de arquivo para não poluir a raiz do projeto.

- `split_agentic_loop.py`: Script utilizado para prototipar a quebra do loop ReAct gigante em funções menores.
- `connect_daemon.py` / `test_daemon_connection.py`: Scripts usados para testar a comunicação gRPC e TCP com o daemon em background.
- `patch_tests.py`: Utilizado para testes manuais de aplicação de patches (diff_replace).
- `update_execute.py`: Rascunho para reescrever o arquivo principal de loop de execução em Go.
- `solucao.py`: Arquivo genérico gerado pelo agente durante simulações passadas.
