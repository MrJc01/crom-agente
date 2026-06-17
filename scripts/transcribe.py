#!/usr/bin/env python3
import sys
import os
import urllib.request
import zipfile
import json
import wave

# Force stdout to UTF-8
if hasattr(sys.stdout, "reconfigure"):
    sys.stdout.reconfigure(encoding='utf-8')
if hasattr(sys.stderr, "reconfigure"):
    sys.stderr.reconfigure(encoding='utf-8')

MODEL_URL = "https://alphacephei.com/vosk/models/vosk-model-small-pt-0.3.zip"
CROM_DIR = os.path.expanduser("~/.crom")
MODEL_DIR = os.path.join(CROM_DIR, "vosk-model-small-pt-0.3")
ZIP_PATH = os.path.join(CROM_DIR, "vosk-model-small-pt-0.3.zip")

def main():
    if len(sys.argv) < 2:
        print("Usage: transcribe.py <wav_path>")
        sys.exit(1)
        
    wav_path = sys.argv[1]
    
    # 1. Check Vosk library
    try:
        import vosk
    except ImportError:
        print("Biblioteca Vosk nao instalada. Por favor, instale executando: pip install vosk", file=sys.stderr)
        sys.exit(1)
        
    # 2. Ensure model exists
    if not os.path.exists(MODEL_DIR):
        print(f"Baixando modelo Vosk Portugues (31MB)...", file=sys.stderr)
        try:
            os.makedirs(CROM_DIR, exist_ok=True)
            urllib.request.urlretrieve(MODEL_URL, ZIP_PATH)
            print("Descompactando modelo...", file=sys.stderr)
            with zipfile.ZipFile(ZIP_PATH, 'r') as zip_ref:
                zip_ref.extractall(CROM_DIR)
            os.remove(ZIP_PATH)
            print("Modelo offline pronto!", file=sys.stderr)
        except Exception as e:
            print(f"Erro ao baixar/extrair modelo Vosk: {e}", file=sys.stderr)
            sys.exit(1)
            
    # 3. Open WAV and transcribe
    try:
        from vosk import Model, KaldiRecognizer
        
        wf = wave.open(wav_path, "rb")
        if wf.getnchannels() != 1 or wf.getsampwidth() != 2 or wf.getcomptype() != "NONE":
            print("Formato WAV incorreto. Deve ser PCM mono 16-bit.", file=sys.stderr)
            sys.exit(1)
            
        model = Model(MODEL_DIR)
        rec = KaldiRecognizer(model, wf.getframerate())
        rec.SetWords(False)
        
        text_results = []
        while True:
            data = wf.readframes(4000)
            if len(data) == 0:
                break
            if rec.AcceptWaveform(data):
                res = json.loads(rec.Result())
                if res.get("text"):
                    text_results.append(res["text"])
                    
        res = json.loads(rec.FinalResult())
        if res.get("text"):
            text_results.append(res["text"])
            
        final_text = " ".join(text_results).strip()
        print(final_text)
        
    except Exception as e:
        print(f"Erro na transcricao Vosk: {e}", file=sys.stderr)
        sys.exit(1)

if __name__ == "__main__":
    main()
