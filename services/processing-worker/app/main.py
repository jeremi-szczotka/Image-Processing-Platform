"""Entry point — pika consumer loop."""
import json
import logging
import time
import psycopg2
import pika
from app import config
from app.storage import get_minio_client
from app.worker import process_message

logging.basicConfig(
    level=getattr(logging, config.LOG_LEVEL, logging.INFO),
    format='{"time":"%(asctime)s","level":"%(levelname)s","msg":"%(message)s"}',
)
log = logging.getLogger(__name__)

_RETRY_DELAY = 5  # seconds between reconnect attempts


def _callback(ch, method, _properties, body):
    payload = json.loads(body)
    job_id = payload.get("job_id", "?")
    log.info("received job %s", job_id)

    conn = psycopg2.connect(config.DATABASE_URL)
    s3 = get_minio_client(config.MINIO_ENDPOINT, config.MINIO_ACCESS_KEY, config.MINIO_SECRET_KEY, config.MINIO_USE_SSL)

    try:
        process_message(payload, conn, s3)
        ch.basic_ack(delivery_tag=method.delivery_tag)
    except Exception as exc:
        log.error("job %s failed permanently: %s", job_id, exc)
        # Job status already set to FAILED by process_message; discard the message.
        ch.basic_nack(delivery_tag=method.delivery_tag, requeue=False)
    finally:
        conn.close()


def main():
    log.info("processing-worker starting")
    while True:
        try:
            params = pika.URLParameters(config.RABBITMQ_URL)
            connection = pika.BlockingConnection(params)
            channel = connection.channel()
            channel.queue_declare(queue=config.QUEUE_NAME, durable=True)
            # Take exactly one message at a time — ensures fair distribution across replicas.
            channel.basic_qos(prefetch_count=1)
            channel.basic_consume(queue=config.QUEUE_NAME, on_message_callback=_callback)
            log.info("connected to RabbitMQ, waiting for jobs on queue %r", config.QUEUE_NAME)
            channel.start_consuming()
        except pika.exceptions.AMQPConnectionError as exc:
            log.warning("RabbitMQ connection lost (%s), retrying in %ds", exc, _RETRY_DELAY)
            time.sleep(_RETRY_DELAY)


if __name__ == "__main__":
    main()
