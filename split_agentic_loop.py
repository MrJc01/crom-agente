import re
import os

with open('internal/loop/agentic_loop.go', 'r') as f:
    content = f.read()

# Define patterns to extract functions
def extract_func(content, func_name_pattern):
    match = re.search(r'^((?:func|const|var) ' + func_name_pattern + r'(?:.|\n)*?^})', content, re.MULTILINE)
    if match:
        return match.group(1)
    
    # Try finding multi-line consts
    match = re.search(r'^(const ' + func_name_pattern + r'(?:.|\n)*?^`)', content, re.MULTILINE)
    if match:
        return match.group(1)
    
    return ""

def extract_type(content, type_name):
    match = re.search(r'^(type ' + type_name + r'(?:.|\n)*?^})', content, re.MULTILINE)
    if match:
        return match.group(1)
    return ""

def extract_struct(content, struct_name):
    match = re.search(r'^(type ' + struct_name + r' struct(?:.|\n)*?^})', content, re.MULTILINE)
    if match:
        return match.group(1)
    return ""

# We will just write the files manually because regex extraction of Go code might fail on nested braces if not careful.
