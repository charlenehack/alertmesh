FROM python:3.12-slim

RUN pip install --no-cache-dir kubernetes requests

COPY watcher.py /app/watcher.py

WORKDIR /app
CMD ["python", "watcher.py"]