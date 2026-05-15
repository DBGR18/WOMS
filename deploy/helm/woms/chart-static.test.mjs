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

function imageTag(section) {
  const match = values.match(new RegExp(`${section}:\\n[\\s\\S]*?image:\\n[\\s\\S]*?tag:\\s+([^\\s]+)`));
  assert.ok(match, `missing ${section}.image.tag`);
  return match[1];
}

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
  assert.match(values, /bootstrapServers:\s+"kafka\.\{\{ \.Release\.Namespace \}\}\.svc\.cluster\.local:9092"/);
  assert.match(values, /lagThreshold:\s+"10"/);
  assert.match(values, /targetUtilization:\s+"70"/);
  assert.match(values, /gthulhu:[\s\S]*enabled:\s+false/);
  assert.match(values, /prometheusServerAddress:\s+"http:\/\/monitoring-kube-prometheus-prometheus\.monitoring:9090"/);
  assert.match(values, /metricName:\s+woms_worker_gthulhu_involuntary_ctx_switches_rate/);
  assert.match(values, /threshold:\s+"20"/);
  assert.match(values, /query:\s+\|-/);
  assert.match(values, /gthulhu_pod_involuntary_ctx_switches_total\{exported_namespace="\{\{ \.Release\.Namespace \}\}"/);
  assert.match(values, /pod_name=~"\{\{ include "woms\.fullname" \. \}\}-worker-\.\*"/);
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
  const apiTag = imageTag("api");
  assert.match(apiTag, /^v0\.1\.\d+$/);
  assert.equal(imageTag("worker"), apiTag);
  assert.equal(imageTag("web"), apiTag);
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
  assert.match(scaledObject, /bootstrapServers:\s+\{\{ tpl \.Values\.keda\.kafka\.bootstrapServers \. \| quote \}\}/);
  assert.match(scaledObject, /topic:\s+\{\{ \.Values\.keda\.kafka\.topic \| quote \}\}/);
  assert.match(scaledObject, /consumerGroup:\s+\{\{ \.Values\.keda\.kafka\.consumerGroup \| quote \}\}/);
  assert.match(scaledObject, /lagThreshold:\s+\{\{ \.Values\.keda\.kafka\.lagThreshold \| quote \}\}/);
  assert.match(scaledObject, /type:\s+cpu/);
  assert.match(scaledObject, /metricType:\s+Utilization/);
  assert.match(scaledObject, /if \.Values\.keda\.gthulhu\.enabled/);
  assert.match(scaledObject, /type:\s+prometheus/);
  assert.match(scaledObject, /serverAddress:\s+\{\{ \.Values\.keda\.gthulhu\.prometheusServerAddress \| quote \}\}/);
  assert.match(scaledObject, /metricName:\s+\{\{ \.Values\.keda\.gthulhu\.metricName \| quote \}\}/);
  assert.match(scaledObject, /query:\s+\{\{ tpl \.Values\.keda\.gthulhu\.query \. \| quote \}\}/);
  assert.match(scaledObject, /threshold:\s+\{\{ \.Values\.keda\.gthulhu\.threshold \| quote \}\}/);
});

test("Kafka topic hook creates the scheduling topic with enough partitions for HPA", () => {
  assert.match(values, /kafkaTopic:[\s\S]*repository:\s+docker\.io\/bitnamilegacy\/kafka/);
  assert.match(values, /kafkaTopic:[\s\S]*tag:\s+3\.7\.1-debian-12-r4/);
  assert.match(kafkaTopicJob, /kind:\s+Job/);
  assert.match(kafkaTopicJob, /helm\.sh\/hook/);
  assert.match(kafkaTopicJob, /activeDeadlineSeconds:\s+\{\{ \.Values\.kafkaTopic\.activeDeadlineSeconds \}\}/);
  assert.match(kafkaTopicJob, /bootstrap=\{\{ tpl \.Values\.keda\.kafka\.bootstrapServers \. \| quote \}\}/);
  assert.match(kafkaTopicJob, /kafka-topics\.sh/);
  assert.match(kafkaTopicJob, /max_attempts=\{\{ \.Values\.kafkaTopic\.wait\.maxAttempts \| int \}\}/);
  assert.match(kafkaTopicJob, /exit 1/);
  assert.match(kafkaTopicJob, /--create/);
  assert.match(kafkaTopicJob, /--if-not-exists/);
  assert.match(kafkaTopicJob, /--alter/);
  assert.match(kafkaTopicJob, /\$partitions = \(\.Values\.keda\.maxReplicaCount \| int\)/);
});

test("Bitnami dependency image overrides use the legacy repository for retained tags", () => {
  assert.match(values, /postgresql:[\s\S]*repository:\s+bitnamilegacy\/postgresql/);
  assert.match(values, /postgresql:[\s\S]*tag:\s+16\.4\.0-debian-12-r14/);
  assert.match(values, /redis:[\s\S]*repository:\s+bitnamilegacy\/redis/);
  assert.match(values, /redis:[\s\S]*tag:\s+7\.2\.5-debian-12-r4/);
  assert.match(values, /^kafka:\n(?:^[ \t]+[^\n]*\n)*?^[ \t]+image:\n(?:^[ \t]+[^\n]*\n)*?^[ \t]+repository:\s+bitnamilegacy\/kafka\s*$/m);
  assert.match(values, /^kafka:\n(?:^[ \t]+[^\n]*\n)*?^[ \t]+image:\n(?:^[ \t]+[^\n]*\n)*?^[ \t]+tag:\s+3\.7\.1-debian-12-r4\s*$/m);
});

test("Single-node Kafka defaults keep internal topics usable on a clean VM", () => {
  assert.match(values, /controller:[\s\S]*replicaCount:\s+1/);
  assert.match(values, /broker:[\s\S]*replicaCount:\s+0/);
  assert.match(values, /controller:[\s\S]*extraConfigYaml:[\s\S]*default\.replication\.factor:\s+1/);
  assert.match(values, /controller:[\s\S]*extraConfigYaml:[\s\S]*min\.insync\.replicas:\s+1/);
  assert.match(values, /controller:[\s\S]*extraConfigYaml:[\s\S]*offsets\.topic\.replication\.factor:\s+1/);
  assert.match(values, /controller:[\s\S]*extraConfigYaml:[\s\S]*transaction\.state\.log\.min\.isr:\s+1/);
  assert.match(values, /controller:[\s\S]*extraConfigYaml:[\s\S]*transaction\.state\.log\.replication\.factor:\s+1/);
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
  assert.match(workerDeployment, /value:\s+\{\{ tpl \.Values\.keda\.kafka\.bootstrapServers \. \| quote \}\}/);
  assert.match(workerDeployment, /name:\s+KAFKA_CONSUMER_GROUP/);
  assert.match(workerDeployment, /name:\s+DATABASE_URL/);
  assert.match(workerDeployment, /name:\s+WORKER_MIN_JOB_DURATION_MS/);
  assert.match(workerDeployment, /name:\s+WORKER_MAX_RETRIES/);
  assert.match(workerDeployment, /if not \.Values\.keda\.enabled/);
  assert.match(workerDeployment, /replicas:\s+\{\{ \.Values\.worker\.replicaCount \}\}/);
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
