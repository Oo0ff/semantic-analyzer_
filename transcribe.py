import sys
import json
import wave
import re
from vosk import Model, KaldiRecognizer, SetLogLevel

if len(sys.argv) < 2:
    print("Usage: python transcribe.py <audio.wav> [model_path]")
    sys.exit(1)

# Отключаем логи Vosk
SetLogLevel(-1)

# Принудительно устанавливаем UTF-8 для вывода
if hasattr(sys.stdout, 'reconfigure'):
    sys.stdout.reconfigure(encoding='utf-8')
else:
    import io
    sys.stdout = io.TextIOWrapper(sys.stdout.buffer, encoding='utf-8')

# Путь к модели
if len(sys.argv) >= 3:
    model_path = sys.argv[2]
else:
    model_path = "models/vosk-model-small-ru-0.22"

if not model_path:
    print("ERROR: model path is empty")
    sys.exit(1)

model = Model(model_path)
wf = wave.open(sys.argv[1], "rb")
if wf.getnchannels() != 1 or wf.getsampwidth() != 2 or wf.getframerate() not in (8000, 16000):
    print("Audio must be mono, 16-bit, 8/16 kHz WAV")
    sys.exit(1)

rec = KaldiRecognizer(model, wf.getframerate())
result_text = ""

while True:
    data = wf.readframes(4000)
    if len(data) == 0:
        break
    if rec.AcceptWaveform(data):
        res = json.loads(rec.Result())
        result_text += res.get("text", "") + " "

final = json.loads(rec.FinalResult())
result_text += final.get("text", "")

# Очистка текста: оставляем только буквы (русские и латинские), цифры, пробелы и основные знаки препинания
cleaned = re.sub(r'[^\w\s.,!?;:…«»\-–—@№#]', ' ', result_text, flags=re.UNICODE)
# Удаляем лишние пробелы
cleaned = re.sub(r'\s+', ' ', cleaned).strip()

print(cleaned)