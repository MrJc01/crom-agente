FROM python:3.9-slim-buster

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY connect_daemon.py .

CMD ["python", "connect_daemon.py"]
