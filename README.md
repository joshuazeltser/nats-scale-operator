# NATS Scale Operator

## Summary

A Kubernetes operator that provides automatic scaling of deployments based on NATS JetStream consumer queue depth. 
The operator monitors pending messages in NATS JetStream consumers and scales deployments up or down based on configurable thresholds, 
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

1. **Add the Helm repository:**
   ```bash
   helm repo add nats-scale-operator https://your-helm-repo-url
   helm repo update
   ```

2. **Install the operator:**
   ```bash
   helm install nats-scale-operator nats-scale-operator/nats-scale-operator \
     --namespace nats-scale-operator-system \
     --create-namespace
   ```

3. **Customize installation:**
   ```bash
   # Create values file
   cat > values.yaml << EOF
   image:
     repository: your-registry/nats-scale-operator
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

4. **Verify installation:**
   ```bash
   helm status nats-scale-operator -n nats-scale-operator-system
   kubectl get pods -n nats-scale-operator-system
   ```

5. **Upgrade the operator:**
   ```bash
   helm upgrade nats-scale-operator nats-scale-operator/nats-scale-operator \
     --namespace nats-scale-operator-system \
     --values values.yaml
   ```

6. **Uninstall:**
   ```bash
   helm uninstall nats-scale-operator -n nats-scale-operator-system
   ```

## How to Run Environment Tests

The project includes comprehensive test suites using Kubebuilder's envtest framework for integration testing.

### Prerequisites

- Go 1.21+
- Docker for kind/minikube testing
- Make

### Running Tests

1. **Run unit tests:**
   ```bash
   make test
   ```

2. **Run tests with coverage:**
   ```bash
   make test-coverage
   ```

3. **Run integration tests with envtest:**
   ```bash
   # This starts a local control plane for testing
   make envtest
   ```

4. **Run specific test suites:**
   ```bash
   # Controller tests only
   go test ./internal/controller/... -v
   
   # API tests only  
   go test ./api/... -v
   
   # Run with race detection
   go test -race ./...
   ```

5. **End-to-end testing:**
   ```bash
   # Build and test in minikube
   make e2e-test
   ```

### Test Configuration

Tests use the following environment:
- **Kubernetes Version**: 1.28+
- **Control Plane**: envtest (etcd + kube-apiserver)
- **Test Timeout**: 10 minutes
- **Parallel Execution**: Enabled for unit tests

### Test Coverage

The test suite covers:
- ✅ Controller reconciliation logic
- ✅ Custom resource validation
- ✅ NATS integration (mocked)
- ✅ Scaling behavior
- ✅ Error handling
- ✅ RBAC permissions

Run `make test-coverage` to generate detailed coverage reports.

---

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests for new functionality
5. Run the test suite: `make test`
6. Submit a pull request

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.