# nginxstaticsite

### Important tip
Given that I did not have enough time to complete this task, the core of the work, including the operator being able to deploy and update the deployment based on the CRD, as well as create PVCs, has been done and tested. Creating and updating the service and ingress, as well as Prometheus metrics, are among the items that have not been tested or debugged.

[NginxStaticSite CRD yaml file](config/crd/bases/web.ictplus.ir_nginxstaticsites.yaml)

[RBAC roles and bindings](config/rbac)

### Install
- Dependencies:
  - go version v1.23.0+
  - docker version 17.03+.
  - kubectl version v1.11.3+.
  - kubebuilder version v4.5.2+.

- Clone the project:
```
git clone https://github.com/m-nik/k8s-nginx-operator.git
cd k8s-nginx-operator
```
- Install [NginxStaticSite CRD](config/crd/bases/web.ictplus.ir_nginxstaticsites.yaml)
```
kubectl apply -f config/crd/bases/web.ictplus.ir_nginxstaticsites.yaml
```
- Setting up the NginxStaticSite manifest and applying it: ([This example](config/samples/web_v1alpha1_nginxstaticsite.yaml))
```
kubectl apply -f config/samples/web_v1alpha1_nginxstaticsite.yaml
```
- Build, Push and deploy:
```
make docker-build docker-push deploy IMG=yourDockerRepo/nginx-operator:v0.1.13
```
##### OR
- Only deploy:
```
make deploy IMG=mojprogrammer/nginx-operator:v0.1.13
```
