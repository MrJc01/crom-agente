import os
import tarfile
import tempfile
import io
import subprocess
from pathlib import Path

try:
    import docker
except ImportError:
    docker = None

class DockerSandbox:
    def __init__(self, image="python:3.10-slim", container_name=None, volumes=None, env=None):
        self.image = image
        self.container_name = container_name
        self.volumes = volumes or {}
        self.env = env or {}
        self.client = None
        self.container = None
        
        if docker is not None:
            try:
                self.client = docker.from_env()
            except Exception as e:
                print(f"⚠️ Não foi possível conectar ao Docker SDK: {e}. Usando CLI fallback.")
                self.client = None

    def start(self):
        """
        Inicia o container Docker e o mantém ativo em segundo plano.
        """
        # Se tiver o SDK do Docker funcional
        if self.client:
            try:
                # Prepara os volumes formatados para o SDK
                vol_map = {}
                for host_p, cont_p in self.volumes.items():
                    vol_map[os.path.abspath(host_p)] = {"bind": cont_p, "mode": "rw"}
                    
                self.container = self.client.containers.run(
                    self.image,
                    command="tail -f /dev/null",
                    name=self.container_name,
                    environment=self.env,
                    volumes=vol_map,
                    detach=True,
                    tty=True,
                    remove=True
                )
                return True
            except Exception as e:
                print(f"❌ Erro ao iniciar container via Docker SDK: {e}")
                self.container = None

        # Fallback via CLI do docker
        cmd = ["docker", "run", "-d", "--rm"]
        if self.container_name:
            cmd.extend(["--name", self.container_name])
        for host_p, cont_p in self.volumes.items():
            cmd.extend(["-v", f"{os.path.abspath(host_p)}:{cont_p}:rw"])
        for k, v in self.env.items():
            cmd.extend(["-e", f"{k}={v}"])
        cmd.extend([self.image, "tail", "-f", "/dev/null"])
        
        try:
            res = subprocess.run(cmd, capture_output=True, text=True, check=True)
            self.container_id = res.stdout.strip()
            return True
        except Exception as e:
            print(f"❌ Erro no fallback do docker CLI: {e}")
            return False

    def exec_run(self, cmd_args, workdir=None, env=None, timeout=None):
        """
        Executa um comando dentro do container ativo.
        """
        env_vars = self.env.copy()
        if env:
            env_vars.update(env)
            
        if self.client and self.container:
            try:
                # Docker SDK não aceita timeout direto no exec_run de forma simples,
                # então usamos sockets ou executamos em subprocesso CLI se necessário timeout.
                # Para comandos simples:
                exit_code, output = self.container.exec_run(
                    cmd_args,
                    workdir=workdir,
                    environment=env_vars,
                    demux=False
                )
                return exit_code, output.decode("utf-8", errors="replace")
            except Exception as e:
                print(f"Erro exec_run via SDK: {e}")
                
        # Fallback via CLI docker exec
        container_ref = self.container.id if self.container else self.container_id
        cmd = ["docker", "exec"]
        if workdir:
            cmd.extend(["-w", workdir])
        for k, v in env_vars.items():
            cmd.extend(["-e", f"{k}={v}"])
        cmd.append(container_ref)
        
        if isinstance(cmd_args, list):
            cmd.extend(cmd_args)
        else:
            cmd.append(cmd_args)
            
        try:
            res = subprocess.run(cmd, capture_output=True, text=True, errors="replace", timeout=timeout)
            return res.returncode, res.stdout + "\n" + res.stderr
        except subprocess.TimeoutExpired as te:
            return -1, f"TIMEOUT EXPIRED inside container: {te.stdout or ''}\n{te.stderr or ''}"
        except Exception as e:
            return -1, f"Erro ao executar comando CLI no container: {e}"

    def copy_to_container(self, local_path, container_path):
        """
        Copia um arquivo ou diretório local para dentro do container.
        """
        local_path = Path(local_path)
        container_ref = self.container.id if self.container else self.container_id
        
        # Cria um tarball em memória
        tar_stream = io.BytesIO()
        with tarfile.open(fileobj=tar_stream, mode='w') as tar:
            tar.add(local_path, arcname=local_path.name)
        tar_stream.seek(0)
        
        if self.client and self.container:
            try:
                self.container.put_archive(os.path.dirname(container_path), tar_stream.read())
                return True
            except Exception as e:
                print(f"Erro put_archive: {e}")
                
        # Fallback via docker cp usando arquivo temporário
        with tempfile.NamedTemporaryFile(suffix=".tar", delete=False) as tmp:
            tmp.write(tar_stream.read())
            tmp_path = tmp.name
            
        try:
            # docker cp precisa extrair o tarball no destino
            cmd = ["docker", "cp", tmp_path, f"{container_ref}:{container_path}"]
            subprocess.run(cmd, check=True, capture_output=True)
            os.unlink(tmp_path)
            return True
        except Exception as e:
            if os.path.exists(tmp_path):
                os.unlink(tmp_path)
            print(f"Erro docker cp: {e}")
            return False

    def copy_from_container(self, container_path, local_path):
        """
        Copia um arquivo de dentro do container para a máquina local.
        """
        container_ref = self.container.id if self.container else self.container_id
        
        if self.client and self.container:
            try:
                bits, stat = self.container.get_archive(container_path)
                tar_stream = io.BytesIO()
                for chunk in bits:
                    tar_stream.write(chunk)
                tar_stream.seek(0)
                
                with tarfile.open(fileobj=tar_stream) as tar:
                    tar.extractall(path=os.path.dirname(local_path))
                return True
            except Exception as e:
                print(f"Erro get_archive: {e}")
                
        # Fallback via docker cp CLI
        try:
            cmd = ["docker", "cp", f"{container_ref}:{container_path}", local_path]
            subprocess.run(cmd, check=True, capture_output=True)
            return True
        except Exception as e:
            print(f"Erro docker cp from container: {e}")
            return False

    def stop(self):
        """
        Para e remove o container.
        """
        if self.client and self.container:
            try:
                self.container.stop(timeout=2)
                self.container = None
                return True
            except Exception:
                pass
                
        # Fallback CLI
        container_ref = getattr(self, 'container_id', None)
        if not container_ref and self.container:
            container_ref = self.container.id
            
        if container_ref:
            try:
                subprocess.run(["docker", "kill", container_ref], capture_output=True)
                return True
            except Exception:
                pass
        return False
