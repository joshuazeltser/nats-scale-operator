# NATS JetStream Deployment Guide

This guide walks you through deploying NATS JetStream on Kubernetes using Helm and demonstrates how to publish and consume messages.

## Prerequisites

- Kubernetes cluster running
- Helm 3.x installed
- kubectl configured to access your cluster

## Deployment

### 1. Add NATS Helm Repository

```bash
helm repo add nats https://nats-io.github.io/k8s/helm/charts/
helm repo update
```

### 2. Create Namespace

```bash
kubectl create namespace nats
```

### 3. Deploy NATS with JetStream

Deploy NATS with JetStream enabled using a single command:

```bash
helm install nats nats/nats \
  --namespace nats \
  --set config.jetstream.enabled=true \
  --set config.jetstream.memStorage.enabled=true \
  --set config.jetstream.memStorage.size=1Gi \
  --set config.jetstream.fileStorage.enabled=true \
  --set config.jetstream.fileStorage.size=10Gi \
  --set config.jetstream.fileStorage.storageClassName=default \
  --set natsBox.enabled=true
```

Alternatively, create a `values.yaml` file for more control:

```yaml
config:
  jetstream:
    enabled: true
    memStorage:
      enabled: true
      size: 1Gi
    fileStorage:
      enabled: true
      size: 10Gi
      storageClassName: "default"

natsBox:
  enabled: true

auth:
  enabled: false

service:
  type: ClusterIP
  ports:
    client: 4222
    cluster: 6222
    monitor: 8222
```

Then deploy with:

```bash
helm install nats nats/nats --namespace nats -f values.yaml
```

### 4. Verify Deployment

```bash
kubectl get pods -n nats
kubectl get svc -n nats
```

You should see pods running including a `nats-box` pod which provides NATS CLI tools.

## Publishing and Consuming Messages

### Using the NATS Box Pod

The easiest way to interact with NATS is through the included nats-box pod:

```bash
# Connect to the nats-box pod
kubectl exec -n nats -it deployment/nats-box -- /bin/sh
```

### Create a Stream

Inside the nats-box pod, create a JetStream stream:

```bash
# Create a stream for order messages
nats stream add orders --subjects "orders.*" --storage file --replicas 1
```

### Create Consumers

Create consumers to process messages:

```bash
# Create a pull consumer (recommended)
nats consumer add orders order-processor --deliver all --ack explicit --max-deliver 1000

# Or create a push consumer (delivers to a subject outside the stream)
nats consumer add orders order-notifier --target notifications.orders --deliver all --ack explicit --max-deliver 1000
```

### Publish Messages

Send messages to your stream:

```bash
# Publish various order events
nats pub orders.received "Order #001: 2 laptops received"
nats pub orders.processing "Order #001: Processing payment"
nats pub orders.completed "Order #001: Shipped to customer"
nats pub orders.received "Order #002: 1 mouse received"
```

### Consume Messages

**Pull Consumer (Recommended):**

```bash
# Pull individual messages
nats consumer next orders order-processor

# Pull multiple messages at once
nats consumer next orders order-processor --count 5

# Pull messages continuously
while true; do
  nats consumer next orders order-processor --timeout 5s || break
done
```

**Push Consumer:**

```bash
# Subscribe to the delivery subject (in another terminal/session)
nats sub notifications.orders
```

### Monitor Your Setup

Check the status of your streams and consumers:

```bash
# View server information
nats server info

# List all streams
nats stream list

# Get detailed stream information
nats stream info orders

# List consumers for a stream
nats consumer list orders

# Get detailed consumer information
nats consumer info orders order-processor
```

## Accessing from Outside the Cluster

To access NATS from applications outside the cluster, you can:

### Port Forward (Development)

```bash
kubectl port-forward -n nats svc/nats 4222:4222
```

Then connect to `nats://localhost:4222` from your applications.

### LoadBalancer Service (Production)

Update the service type to LoadBalancer:

```bash
kubectl patch svc nats -n nats -p '{"spec":{"type":"LoadBalancer"}}'
```

Or include in your values.yaml:

```yaml
service:
  type: LoadBalancer
```

## Clean Up

To remove the NATS deployment:

```bash
helm uninstall nats --namespace nats
kubectl delete namespace nats
```

## Next Steps

- Explore [NATS JetStream documentation](https://docs.nats.io/jetstream) for advanced features
- Implement message deduplication, retention policies, and scaling
- Set up monitoring and alerting for your NATS deployment
- Configure authentication and authorization for production use