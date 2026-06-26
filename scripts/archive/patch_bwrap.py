import re

with open("internal/tools/terminal_command/terminal_command.go", "r") as f:
    code = f.read()

helper = """
func buildSandboxedCommand(ctx context.Context, cmdName string, cmdArgs []string, workspace string, jailDir string) *exec.Cmd {
    if jailDir == "" {
        c := exec.CommandContext(ctx, cmdName, cmdArgs...)
        c.Dir = workspace
        return c
    }

    // Tenta encontrar o bwrap
    bwrapPath, err := exec.LookPath("bwrap")
    if err == nil && bwrapPath != "" {
        // Usa bwrap
        bwrapArgs := []string{
            "--ro-bind", "/", "/",
            "--dev", "/dev",
            "--proc", "/proc",
            "--tmpfs", "/tmp",
            "--bind", workspace, workspace,
            "--unshare-pid",
            "--unshare-ipc",
            "--unshare-uts",
            "--unshare-cgroup",
            "--chdir", workspace,
        }
        bwrapArgs = append(bwrapArgs, cmdName)
        bwrapArgs = append(bwrapArgs, cmdArgs...)
        
        c := exec.CommandContext(ctx, bwrapPath, bwrapArgs...)
        c.Dir = workspace
        return c
    }

    // Fallback: não usa jail se não tem privilégios
    c := exec.CommandContext(ctx, cmdName, cmdArgs...)
    c.Dir = workspace
    return c
}
"""

if "func buildSandboxedCommand" not in code:
    code = code + "\n" + helper

# Patch foreground cmd
fg_old = """
        c := exec.CommandContext(ctx, cmdName, cmdArgs...)
        c.Dir = t.workspaceRoot

        if t.jailDir != "" {
                c.SysProcAttr = &syscall.SysProcAttr{
                        Chroot: t.jailDir,
                }
        }
"""
fg_new = """
        c := buildSandboxedCommand(ctx, cmdName, cmdArgs, t.workspaceRoot, t.jailDir)
"""
code = code.replace(fg_old, fg_new)

# Patch background cmd
bg_old = """
                c := exec.CommandContext(bgCtx, cmdName, cmdArgs...)
                c.Dir = t.workspaceRoot
                c.SysProcAttr = &syscall.SysProcAttr{
                        Setpgid: true, // Executa em um grupo de processos separado para isolar sinais do processo pai
                }

                if t.jailDir != "" {
                        c.SysProcAttr.Chroot = t.jailDir // Isolar execução com chroot
                }
"""
bg_new = """
                c := buildSandboxedCommand(bgCtx, cmdName, cmdArgs, t.workspaceRoot, t.jailDir)
                c.SysProcAttr = &syscall.SysProcAttr{
                        Setpgid: true, // Executa em um grupo de processos separado para isolar sinais do processo pai
                }
"""
code = code.replace(bg_old, bg_new)

with open("internal/tools/terminal_command/terminal_command.go", "w") as f:
    f.write(code)

