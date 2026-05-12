# Kubernetes Example

Apply the manifests in this order:

```bash
kubectl apply -f deploy/k8s/namespace.yaml
kubectl apply -f deploy/k8s/collector-configmap.yaml
kubectl apply -f deploy/k8s/collector-deployment.yaml
kubectl apply -f deploy/k8s/collector-service.yaml
kubectl apply -f deploy/k8s/web-deployment.yaml
kubectl apply -f deploy/k8s/web-service.yaml
kubectl apply -f deploy/k8s/ingress.yaml
```

Before applying:

1. Replace the placeholder image names with your published collector and web images.
2. Adjust hostnames in `ingress.yaml`.
3. If you want durable trace storage, swap the collector `emptyDir` mount for a PVC and set `DATA_DIR`.
