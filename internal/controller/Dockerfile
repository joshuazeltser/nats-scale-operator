FROM golang:1.24

# Install dependencies
RUN apt-get update && apt-get install -y unzip curl

# Install Ginkgo
RUN go install github.com/onsi/ginkgo/v2/ginkgo@v2.15.0

# Install kubebuilder envtest tools
RUN go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest && \
    setup-envtest use 1.29.0 -p /usr/local/kubebuilder

# Set up workspace
WORKDIR /workspace
COPY . .

# Set ENV var for envtest to find kube-apiserver and etcd
ENV KUBEBUILDER_ASSETS=/usr/local/kubebuilder/kubebuilder_1.29.0_linux_amd64/bin

# Run test
CMD ["ginkgo", "-v", "./controllers"]
