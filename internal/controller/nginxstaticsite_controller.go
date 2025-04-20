package controller

import (
    "context"
    //"fmt"

    appsv1 "k8s.io/api/apps/v1"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    //"k8s.io/apimachinery/pkg/types"
    "k8s.io/apimachinery/pkg/api/errors"
    "k8s.io/apimachinery/pkg/runtime"
    resource "k8s.io/apimachinery/pkg/api/resource"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"

    webv1alpha1 "github.com/m-nik/k8s-operator-task/api/v1alpha1"
)

type NginxStaticSiteReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *NginxStaticSiteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    logger := log.FromContext(ctx)

    var site webv1alpha1.NginxStaticSite
    if err := r.Get(ctx, req.NamespacedName, &site); err != nil {
        if errors.IsNotFound(err) {
            return ctrl.Result{}, nil
        }
        return ctrl.Result{}, err
    }

    site.Status.Phase = "Creating"
    r.Status().Update(ctx, &site)

    // PVC
    pvc := &corev1.PersistentVolumeClaim{
        ObjectMeta: metav1.ObjectMeta{
            Name:      site.Name + "-pvc",
            Namespace: site.Namespace,
        },
        Spec: corev1.PersistentVolumeClaimSpec{
            AccessModes: []corev1.PersistentVolumeAccessMode{
                corev1.ReadWriteOnce,
            },
            Resources: corev1.VolumeResourceRequirements{
                Requests: corev1.ResourceList{
                    corev1.ResourceStorage: resourceMustParse(site.Spec.StorageSize),
                },
            },
        },
    }
    if err := ctrl.SetControllerReference(&site, pvc, r.Scheme); err == nil {
        //_ = r.Create(ctx, pvc)
        if err := r.Create(ctx, pvc); err != nil {
            logger.Error(err, "failed to create PVC")
            return ctrl.Result{}, err
        }
    }

    // Deployment
    deploy := &appsv1.Deployment{
        ObjectMeta: metav1.ObjectMeta{
            Name:      site.Name + "-nginx",
            Namespace: site.Namespace,
        },
        Spec: appsv1.DeploymentSpec{
            Replicas: &site.Spec.Replicas,
            Selector: &metav1.LabelSelector{
                MatchLabels: map[string]string{"app": site.Name},
            },
            Template: corev1.PodTemplateSpec{
                ObjectMeta: metav1.ObjectMeta{
                    Labels: map[string]string{"app": site.Name},
                },
                Spec: corev1.PodSpec{
                    NodeSelector: site.Spec.NodeSelector,
                    Containers: []corev1.Container{
                        {
                            Name:  "nginx",
                            Image: "nginx:" + site.Spec.ImageVersion,
                            VolumeMounts: []corev1.VolumeMount{
                                {
                                    Name:      "static-content",
                                    MountPath: site.Spec.StaticFilePath,
                                },
                            },
                        },
                    },
                    Volumes: []corev1.Volume{
                        {
                            Name: "static-content",
                            VolumeSource: corev1.VolumeSource{
                                PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
                                    ClaimName: site.Name + "-pvc",
                                },
                            },
                        },
                    },
                },
            },
        },
    }
    if err := ctrl.SetControllerReference(&site, deploy, r.Scheme); err == nil {
        //_ = r.Create(ctx, deploy)
	if err := r.Create(ctx, deploy); err != nil {
            logger.Error(err, "failed to create deployment")
            return ctrl.Result{}, err
        }
    }

    site.Status.Phase = "Running"
    site.Status.ReadyReplicas = site.Spec.Replicas
    r.Status().Update(ctx, &site)

    logger.Info("Reconciled NginxStaticSite successfully", "name", site.Name)

    return ctrl.Result{}, nil
}

// helper to parse storage size like "1Gi"
func resourceMustParse(size string) resource.Quantity {
    q, _ := resource.ParseQuantity(size)
    return q
}

func (r *NginxStaticSiteReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&webv1alpha1.NginxStaticSite{}).
        Complete(r)
}

