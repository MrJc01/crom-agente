import click
import json
import os
from rich.console import Console
from rich.table import Table
from datetime import datetime


# --- Configurações e Utilitários ---
CONFIG_DIR = os.path.join(os.path.expanduser("~"), ".config", "todo_cli")
TODO_FILE = os.path.join(CONFIG_DIR, "todo.json")
console = Console()

def _ensure_config_dir():
    os.makedirs(CONFIG_DIR, exist_ok=True)

def _load_tasks():
    _ensure_config_dir()
    if not os.path.exists(TODO_FILE):
        return []
    with open(TODO_FILE, 'r') as f:
        return json.load(f)

def _save_tasks(tasks):
    _ensure_config_dir()
    with open(TODO_FILE, 'w') as f:
        json.dump(tasks, f, indent=4)


# --- Comandos CLI ---
@click.group()
def todo():
    "A simple ToDo CLI application."
    pass


@todo.command()
@click.argument('title')
@click.option('--priority', default='medium', type=click.Choice(['low', 'medium', 'high'], case_sensitive=False), help='Priority of the task.')
def add(title, priority):
    "Adds a new task."
    tasks = _load_tasks()
    task_id = 1 if not tasks else max([t['id'] for t in tasks]) + 1
    new_task = {
        'id': task_id,
        'title': title,
        'description': '',  # Description is empty for now
        'priority': priority,
        'done': False,
        'created_at': datetime.now().isoformat()
    }
    tasks.append(new_task)
    _save_tasks(tasks)
    console.print(f"Task '[bold blue]{title}[/bold blue]' added with priority [bold]{priority}[/bold].")


@todo.command()
def list():
    "Lists all tasks."
    tasks = _load_tasks()
    if not tasks:
        console.print("[italic grey]No tasks found.[/italic grey]")
        return

    table = Table(title="[bold green]Your Tasks[/bold green]")
    table.add_column("ID", style="cyan", no_wrap=True)
    table.add_column("Title", style="magenta")
    table.add_column("Priority", style="dim")
    table.add_column("Status", justify="center")
    table.add_column("Created At", style="green")

    for task in tasks:
        status = "[green]✓ Done[/green]" if task['done'] else "[red]✗ Pending[/red]"
        priority_color = {'low': 'grey', 'medium': 'yellow', 'high': 'red'}.get(task['priority'], 'white')
        table.add_row(
            str(task['id']),
            task['title'],
            f"[{priority_color}]{task['priority']}[/{priority_color}]",
            status,
            task['created_at'].split('T')[0] # Just date
        )
    console.print(table)


@todo.command()
@click.argument('task_id', type=int)
def done(task_id):
    "Marks a task as done."
    tasks = _load_tasks()
    found = False
    for task in tasks:
        if task['id'] == task_id:
            task['done'] = True
            found = True
            break
    _save_tasks(tasks)
    if found:
        console.print(f"Task [bold green]#{task_id}[/bold green] marked as done.")
    else:
        console.print(f"[bold red]Error:[/bold red] Task with ID {task_id} not found.")


@todo.command()
@click.argument('task_id', type=int)
def remove(task_id):
    "Removes a task."
    tasks = _load_tasks()
    initial_count = len(tasks)
    tasks = [task for task in tasks if task['id'] != task_id]
    _save_tasks(tasks)
    if len(tasks) < initial_count:
        console.print(f"Task [bold red]#{task_id}[/bold red] removed.")
    else:
        console.print(f"[bold red]Error:[/bold red] Task with ID {task_id} not found.")


if __name__ == '__main__':
    todo()
