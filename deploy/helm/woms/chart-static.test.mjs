import test from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";

const values = readFileSync(new URL("./values.yaml", import.meta.url), "utf8");
const chart = readFileSync(new URL("./Chart.yaml", import.meta.url), "utf8");
const scaledObject = readFileSync(new URL("./templates/keda-scaledobject.yaml", import.meta.url), "utf8");
const apiDeployment = readFileSync(new URL("./templates/api-deployment.yaml", import.meta.url), "utf8");
const workerDeployment = readFileSync(new URL("./templates/worker-deployment.yaml", import.meta.url), "utf8");
const webDeployment = readFileSync(new URL("./templates/web-deployment.yaml", import.meta.url), "utf8");
const services = readFileSync(new URL("./templates/services.yaml", import.meta.url), "utf8");
const kafkaTopicJob = readFileSync(new URL("./templates/kafka-topic-job.yaml", import.meta.url), "utf8");
const secret = readFileSync(new URL("./templates/secret.yaml", import.meta.url), "utf8");
const notes = readFileSync(new URL("./templates/NOTES.txt", import.meta.url), "utf8");

test("Helm values keep async scheduling and HPA demo defaults wired", () => {
  assert.match(values, /store:\s+postgres/);
  assert.match(values, /databaseUrl:\s+postgres:\/\/woms:woms@postgres:5432\/woms\?sslmode=disable/);
  assert.match(values, /redisAddr:\s+redis-master:6379/);
  assert.match(values, /kafkaBrokers:\s+kafka:9092/);
  assert.match(values, /scheduleTopic:\s+woms\.schedule\.jobs/);
  assert.match(values, /kafkaPublishEnabled:\s+"true"/);
  assert.match(values, /minJobDurationMs:\s+"0"/);
  assert.match(values, /maxRetries:\s+"3"/);
  assert.match(values, /consumerGroup:\s+woms-scheduler-workers/);
  assert.match(values, /lagThreshold:\s+"10"/);
  assert.match(values, /targetUtilization:\s+"70"/);
});

test("Helm chart deploys required platform dependencies by default", () => {
  assert.match(chart, /name:\s+postgresql/);
  assert.match(chart, /condition:\s+postgresql\.enabled/);
  assert.match(chart, /name:\s+redis/);
  assert.match(chart, /condition:\s+redis\.enabled/);
  assert.match(chart, /name:\s+kafka/);
  assert.match(chart, /condition:\s+kafka\.enabled/);
  assert.match(values, /postgresql:[\s\S]*enabled:\s+true/);
  assert.match(values, /fullnameOverride:\s+postgres/);
  assert.match(values, /redis:[\s\S]*enabled:\s+true/);
  assert.match(values, /fullnameOverride:\s+redis/);
  assert.match(values, /kafka:[\s\S]*enabled:\s+true/);
  assert.match(values, /fullnameOverride:\s+kafka/);
});

test("Default Docker image tags use v-prefixed release tags", () => {
  assert.match(values, /^imageRegistry:\s+docker\.io\/d11nn/m);
  assert.match(values, /woms-api[\s\S]*tag:\s+v0\.1\.22/);
  assert.match(values, /woms-scheduler-worker[\s\S]*tag:\s+v0\.1\.22/);
  assert.match(values, /woms-web[\s\S]*tag:\s+v0\.1\.22/);
  assert.match(apiDeployment, /include "woms\.image"/);
  assert.match(workerDeployment, /include "woms\.image"/);
  assert.match(webDeployment, /include "woms\.image"/);
});

test("KEDA ScaledObject template points at scheduler worker backlog", () => {
  assert.match(scaledObject, /kind:\s+ScaledObject/);
  assert.match(scaledObject, /horizontalPodAutoscalerConfig:/);
  assert.match(scaledObject, /name:\s+\{\{ include "woms\.fullname" \. \}\}-worker-hpa/);
  assert.match(scaledObject, /scaleTargetRef:[\s\S]*name:\s+\{\{ include "woms\.fullname" \. \}\}-worker/);
  assert.match(scaledObject, /type:\s+kafka/);
  assert.match(scaledObject, /topic:\s+\{\{ \.Values\.keda\.kafka\.topic \| quote \}\}/);
  assert.match(scaledObject, /consumerGroup:\s+\{\{ \.Values\.keda\.kafka\.consumerGroup \| quote \}\}/);
  assert.match(scaledObject, /lagThreshold:\s+\{\{ \.Values\.keda\.kafka\.lagThreshold \| quote \}\}/);
  assert.match(scaledObject, /type:\s+cpu/);
  assert.match(scaledObject, /metricType:\s+Utilization/);
});

test("Kafka topic hook creates the scheduling topic with enough partitions for HPA", () => {
  assert.match(kafkaTopicJob, /kind:\s+Job/);
  assert.match(kafkaTopicJob, /helm\.sh\/hook/);
  assert.match(kafkaTopicJob, /activeDeadlineSeconds:\s+\{\{ \.Values\.kafkaTopic\.activeDeadlineSeconds \}\}/);
  assert.match(kafkaTopicJob, /kafka-topics\.sh/);
  assert.match(kafkaTopicJob, /max_attempts=\{\{ \.Values\.kafkaTopic\.wait\.maxAttempts \| int \}\}/);
  assert.match(kafkaTopicJob, /exit 1/);
  assert.match(kafkaTopicJob, /--create/);
  assert.match(kafkaTopicJob, /--if-not-exists/);
  assert.match(kafkaTopicJob, /--alter/);
  assert.match(kafkaTopicJob, /\$partitions = \(\.Values\.keda\.maxReplicaCount \| int\)/);
});

test("API JWT secret is generated when unset and documented for retrieval", () => {
  assert.match(values, /jwtSecret:\s+""/);
  assert.match(secret, /lookup "v1" "Secret"/);
  assert.match(secret, /randAlphaNum 64/);
  assert.match(notes, /generated or reused a JWT secret/);
  assert.match(notes, /kubectl get secret/);
});

test("API and worker deployments expose PostgreSQL, Kafka, and retry env", () => {
  assert.match(apiDeployment, /name:\s+API_STORE/);
  assert.match(apiDeployment, /name:\s+DATABASE_URL/);
  assert.match(apiDeployment, /name:\s+KAFKA_SCHEDULE_TOPIC/);
  assert.match(apiDeployment, /name:\s+KAFKA_PUBLISH_ENABLED/);
  assert.match(workerDeployment, /name:\s+KAFKA_SCHEDULE_TOPIC/);
  assert.match(workerDeployment, /name:\s+KAFKA_CONSUMER_GROUP/);
  assert.match(workerDeployment, /name:\s+DATABASE_URL/);
  assert.match(workerDeployment, /name:\s+WORKER_MIN_JOB_DURATION_MS/);
  assert.match(workerDeployment, /name:\s+WORKER_MAX_RETRIES/);
});

test("Web deployment is runnable without manual securityContext patches", () => {
  assert.doesNotMatch(services, /name:\s+api\s*\n/);
  assert.match(services, /name:\s+\{\{ include "woms\.fullname" \. \}\}-api/);
  assert.match(webDeployment, /name:\s+API_UPSTREAM/);
  assert.match(webDeployment, /value:\s+\{\{ printf "%s-api:8080" \(include "woms\.fullname" \.\) \| quote \}\}/);
  assert.match(webDeployment, /fsGroup:\s+101/);
  assert.match(webDeployment, /runAsNonRoot:\s+true/);
  assert.match(webDeployment, /runAsUser:\s+101/);
  assert.match(webDeployment, /readOnlyRootFilesystem:\s+true/);
  assert.match(webDeployment, /mountPath:\s+\/etc\/nginx\/conf\.d/);
  assert.match(webDeployment, /mountPath:\s+\/var\/cache\/nginx/);
  assert.match(webDeployment, /mountPath:\s+\/var\/run/);
  assert.match(webDeployment, /mountPath:\s+\/tmp/);
});
