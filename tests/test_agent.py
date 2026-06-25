#!/usr/bin/env python3
import sys
import json

def main():
    try:
        input_data = json.load(sys.stdin)
    except Exception as e:
        print(f"Error parsing json: {e}", file=sys.stderr)
        sys.exit(1)
        
    prompt = input_data.get("prompt", "")
    prior = input_data.get("prior_summary", "")
    
    result = {
        "success": True,
        "output": f"Python agent execution success! Received prompt: {prompt}",
        "context_summary": f"Python agent processed. Prior summary: {prior}"
    }
    
    print(json.dumps(result))

if __name__ == "__main__":
    main()
