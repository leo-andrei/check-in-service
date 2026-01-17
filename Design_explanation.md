# Event-Driven Check-In System: Architecture Summary

## Overview
This system decouples user actions (check-in/check-out) from third-party API calls using an event-driven architecture. It leverages queues, workers, and monitoring to ensure reliability, scalability, and observability.

## Architecture Diagram
See architecture.drawio for a visual diagram. Key components:
- **User Device (UI/Card Reader):** Initiates check-in/out requests.
- **API Service:** Receives requests, validates, and enqueues events.
- **Message Queue (RabbitMQ/SQS):** Buffers events for asynchronous processing.
- **Worker Service:** Consumes events, calls third-party APIs, handles retries and errors.
- **Third-Party API (Legacy System):** External system for recording actions.
- **Dead Letter Queue (DLQ):** Stores messages that repeatedly fail for manual review.
- **Monitoring & Tracing:** Prometheus, OpenTelemetry, Grafana for metrics, tracing, and alerting.

## Design explanation
- This system decouples user actions (check-in/check-out) from third-party API calls using an event-driven architecture. In order to do this, and not lose information from the user, I thought about saving immediately the check-in/out of the user and an event, inside a transaction (so we won't have inconsistencies afterwards). This ensures a quick response for the user, and all the processing is done asynchronously. 
All the events are saved in a separate table and are periodically checked and added to a queue (the check-out ones). I saved here all the events in case of a future audit or replay of events. This table should shrink in the future by adding a command to delete older entries (or simply archive/move older events).
- In case of errors, I thought of a couple of cases:
    * if the database fails: this usually is treated by having replicas on the server side to ensure the availability, or checks in the code if the database is healthy and use a circuit breaker in case is not. This case is not currently handled properly in the code, but an idea for this would be to persist the data somewhere else first. This can be another queue and another publisher that takes the events from this queue and follows the current process in the code.
    * if the transaction fails, nothing is saved. This ensures consistency in our data. The user will be provided with an error response and he will have to try again, or in case of a queue before the database, we will have to retry a couple of times and if errors are persisent, use a deadletter queue
    * in case the queues system is down, for the scenario we have in the code, the data will be persisted in the database and the publisher will try to put the events on the queue with a number of retries. In case of persistent errors, we have a circuit breaker implemented that should be applied here. For the scenario where we have a queue before the database and the queue system is down, we need to make a fallback if the system is down, try to save directly in the database.
    * in case the legacy API is down: the circuit breaker pattern prevents flooding the API during outages, pausing calls until recovery (this is implemented). We should also apply a rate limiter here to avoid exceeding third-party API quotas. 
    * in case the messages can't be consumed or have some errors, they are sent to a DLQ for further intervention

- The circuit breaker pattern is used in order not to flood our system with too many requests. It's used in the calls to the legacy recording system. Another usage would be when trying to publish to the queue system and the system is down. 
- The rate limiter should be used in a couple of ways: one at the API Gateway level to protect against abuse, DoS or accidental floods (reject request with 429 Too Many Requests). Another one would be at the service level (when adding to queues, when persisting to the database, when making calls to third parties, to ensure that all the external systems can handle the load).

- For monitoring and tracing:
    * Distributed tracing - OpenTelemetry. I added an example of usage with spans in the publishing to queues part of the code. I also need to ensure the context is propagated through the whole flow (API, queue, worker) and to use a unique trace ID for each message. This can be exported to a backend (Jaeger, Tempo, or stdout for local dev)
    * Structured logging - Zap. I added a few logs, but on the long term I need to make sure every significant action is logged, with the trace ID and message ID.
    * Metrics - Prometheus. This one I didn't have the time to integrate. It should expose metrics for queue depth, processing latency, success/failure counts and DLQ size. Grafana daskboards are very useful to watch especially the DLQ. Alerts would also be in place.

## The project is functional and has most of the critical things integrated, but there's also a couple of things that need to be added to be Production ready:
- Authentication - this one depends on how this API is used. If it's directly called by a card reader, a simple API key would suffice. If it's called from a UI, than a JWT auth would be recommended.
- Automated DLQ reprocessing tools. 
- Admin UI for monitoring and retrying failed messages.
- Command to delete outbox_events from the database (based on a period of time)
- Testing. We need unit tests, integration tests, end to end tests.
- Database migration from go package
- Middlewares for authentication, logging and metrics
- Secrets management
- API Documentation - OpenAPI/Swagger