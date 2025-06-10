# NATS Scale Operator

## Summary

A Kubernetes operator that provides automatic scaling of deployments based on NATS JetStream stream queue length. 
The operator monitors the number of messages in a stream and scales deployments up or down based on configurable thresholds, 
enabling efficient auto-scaling for message-driven workloads.

## Description

The NATS Scale Operator extends Kubernetes with a custom `AppScaler` resource that automatically adjusts the replica count 
of deployments based on NATS JetStream queue metrics. This is particularly useful for:

- **Message Processing Workloads**: Scale workers based on message queue backlog
- **Event-Driven Applications**: Respond to varying event volumes automatically  
- **Cost Optimization**: Scale down during low traffic periods
- **Performance Management**: Scale up to handle traffic spikes


## How to Deploy Using Minikube

### Prerequisites

- Minikube running with sufficient resources
- kubectl configured for your minikube cluster
- Docker daemon accessible

### Steps

1. **Start Minikube and configure Docker environment:**
   ```bash
   minikube start --memory=4096 --cpus=2
   eval $(minikube docker-env)
   ```

2. **Clone and build the operator:**
   ```bash
   git clone <your-repo-url>
   cd nats-scale-operator
   make generate
   make manifests
   make docker-build IMG=autoscaler:dev
   ```

3. **Deploy the operator:**
   ```bash
   # Install CRDs
   make install
   
   # Deploy operator
   make deploy IMG=autoscaler:dev
   ```

4. **Verify deployment:**
   ```bash
   kubectl get pods -n nats-scale-operator-system
   kubectl logs -n nats-scale-operator-system deployment/nats-scale-operator-controller-manager -f
   ```

5. **Create an AppScaler resource:**
   ```bash
   kubectl apply -f config/samples/autoscale_v1_appscaler.yaml
   ```

6. **Monitoring API:**
   There is a monitoring api which returns recent scaling history (up to 100 events).
   ```bash
   kubectl port-forward <operator pod> 8080:8080 -n scale-operator-system
   curl http://localhost:8080/scaling-history
   ```
   
### Example CRD:
```
apiVersion: autoscale.example.com/v1
kind: AppScaler
metadata:
  name: scale-sample
  namespace: default
spec:
  deploymentName: sample-worker
  namespace: default
  natsMonitoringUrl: http://nats.nats.svc.cluster.local:8222
  stream: mystream
  minReplicas: 1
  maxReplicas: 5
  scaleUpThreshold: 10
  scaleDownThreshold: 2
  pollIntervalSeconds: 15
```

## NATS Deployment and Setup

For detailed instructions on deploying NATS JetStream and configuring streams/consumers, see:

**[NATS Setup Guide](./docs/NATS_SETUP.md)**

This guide covers:
- NATS server deployment in Kubernetes
- JetStream configuration
- Stream and consumer creation
- Testing and verification steps

## How to Deploy Using Helm

### Prerequisites

- Helm v3+ installed
- Access to a Kubernetes cluster

### Steps

1. **Navigate to helm directory:**
   ```bash
   cd dist/chart
   ```

2**Customize & Install installation:**
   ```bash
   # Create values file
   cat > values.yaml << EOF
   image:
     tag: latest
   
   resources:
     limits:
       memory: 128Mi
       cpu: 100m
     requests:
       memory: 64Mi
       cpu: 50m
   
   replicaCount: 1
   EOF
   
   # Install with custom values
   helm install nats-scale-operator nats-scale-operator/nats-scale-operator \
     --namespace nats-scale-operator-system \
     --create-namespace \
     --values values.yaml
   ```

3. **Verify installation:**
   ```bash
   helm status nats-scale-operator -n nats-scale-operator-system
   kubectl get pods -n nats-scale-operator-system
   ```

4. **Upgrade the operator:**
   ```bash
   helm upgrade nats-scale-operator nats-scale-operator/nats-scale-operator \
     --namespace nats-scale-operator-system \
     --values values.yaml
   ```

5. **Uninstall:**
   ```bash
   helm uninstall nats-scale-operator -n nats-scale-operator-system
   ```

## How to Run Environment Tests

The project includes comprehensive test suites using Kubebuilder's envtest framework for integration testing.

### Running Tests

1. Build test docker
   ```bash
    docker build . -t autoscaler-tests:latest -f Dockerfile.test
   ```
2. Run tests
   ```bash
   docker run autoscaler-tests:latest
   ```
## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
