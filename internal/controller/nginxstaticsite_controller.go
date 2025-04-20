package controller

import (
    "context"

    appsv1 "k8s.io/api/apps/v1"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/api/errors"
    "k8s.io/apimachinery/pkg/runtime"
    resource "k8s.io/apimachinery/pkg/api/resource"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/log"
    networkingv1 "k8s.io/api/networking/v1"
    webv1alpha1 "github.com/m-nik/k8s-operator-task/api/v1alpha1"
    intstr "k8s.io/apimachinery/pkg/util/intstr"

)

type NginxStaticSiteReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// Finalizer
const finalizerName = "nginxstaticsite.finalizers.ictplus.ir"



// Reconcile
func (r *NginxStaticSiteReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    logger := log.FromContext(ctx)

    var site webv1alpha1.NginxStaticSite
    if err := r.Get(ctx, req.NamespacedName, &site); err != nil {
        if errors.IsNotFound(err) {
            return ctrl.Result{}, nil
        }
        return ctrl.Result{}, err
    }


    // Handling deletion and finalizer
    if site.ObjectMeta.DeletionTimestamp.IsZero() {
        // Add finalizer
        if !controllerutil.ContainsFinalizer(&site, finalizerName) {
            controllerutil.AddFinalizer(&site, finalizerName)
            if err := r.Update(ctx, &site); err != nil {
                site.Status.Phase = "Failed"
                r.Status().Update(ctx, &site)
                return ctrl.Result{}, err
            }
        }
    } else {
        // Deleting
        logger.Info("Cleaning up resources for deleted NginxStaticSite", "name", site.Name)
    
        // Delete Resources
        _ = r.Delete(ctx, &appsv1.Deployment{
            ObjectMeta: metav1.ObjectMeta{Name: site.Name + "-nginx", Namespace: site.Namespace},
        })
        _ = r.Delete(ctx, &corev1.Service{
            ObjectMeta: metav1.ObjectMeta{Name: site.Name + "-svc", Namespace: site.Namespace},
        })
        _ = r.Delete(ctx, &networkingv1.Ingress{
            ObjectMeta: metav1.ObjectMeta{Name: site.Name + "-ing", Namespace: site.Namespace},
        })
        _ = r.Delete(ctx, &corev1.PersistentVolumeClaim{
            ObjectMeta: metav1.ObjectMeta{Name: site.Name + "-pvc", Namespace: site.Namespace},
        })
    
        // Remove finalizer
        controllerutil.RemoveFinalizer(&site, finalizerName)
        if err := r.Update(ctx, &site); err != nil {
            return ctrl.Result{}, err
        }
    
        return ctrl.Result{}, nil
    }
    





    site.Status.Phase = "Creating"
    r.Status().Update(ctx, &site)



    // ===== PVC =====
    // ===============
    pvc := &corev1.PersistentVolumeClaim{}
    pvcName := site.Name + "-pvc"
    err := r.Get(ctx, client.ObjectKey{Name: pvcName, Namespace: site.Namespace}, pvc)
    
    desiredSize := resourceMustParse(site.Spec.StorageSize)
    
    if err != nil && errors.IsNotFound(err) {
        // PVC not exists, create
        pvc = &corev1.PersistentVolumeClaim{
            ObjectMeta: metav1.ObjectMeta{
                Name:      pvcName,
                Namespace: site.Namespace,
            },
            Spec: corev1.PersistentVolumeClaimSpec{
                AccessModes: []corev1.PersistentVolumeAccessMode{
                    corev1.ReadWriteOnce,
                },
                Resources: corev1.VolumeResourceRequirements{
                    Requests: corev1.ResourceList{
                        corev1.ResourceStorage: desiredSize,
                    },
                },
            },
        }
    
        if err := ctrl.SetControllerReference(&site, pvc, r.Scheme); err == nil {
            if err := r.Create(ctx, pvc); err != nil {
                logger.Error(err, "failed to create PVC")
                site.Status.Phase = "Failed"
                r.Status().Update(ctx, &site)
                return ctrl.Result{}, err
            }
            logger.Info("Created PVC", "name", pvcName)
        }
    } else if err == nil {
        // PVC exists, update
        currentSize := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
    
        if desiredSize.Cmp(currentSize) > 0 {
            pvc.Spec.Resources.Requests[corev1.ResourceStorage] = desiredSize
            if err := r.Update(ctx, pvc); err != nil {
                logger.Error(err, "failed to resize PVC")
                site.Status.Phase = "Failed"
                r.Status().Update(ctx, &site)
                return ctrl.Result{}, err
            }
            logger.Info("Resized PVC", "name", pvcName)
        }
    } else {
        // Unexpected error
        logger.Error(err, "failed to get PVC")
        site.Status.Phase = "Failed"
        r.Status().Update(ctx, &site)
        return ctrl.Result{}, err
    }





    // = Deployment ==
    // ===============
    existingDeploy := &appsv1.Deployment{}
    err = r.Get(ctx, client.ObjectKey{
        Name:      site.Name + "-nginx",
        Namespace: site.Namespace,
    }, existingDeploy)
    
    if err != nil && errors.IsNotFound(err) {
        // Deployment not exists, create
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
            if err := r.Create(ctx, deploy); err != nil {
                logger.Error(err, "failed to create deployment")
                site.Status.Phase = "Failed"
                r.Status().Update(ctx, &site)
                return ctrl.Result{}, err
            }
        }
    } else if err == nil {
        // Deployment exists, update
        updated := false
    
        if *existingDeploy.Spec.Replicas != site.Spec.Replicas {
            existingDeploy.Spec.Replicas = &site.Spec.Replicas
            updated = true
        }
    
        container := &existingDeploy.Spec.Template.Spec.Containers[0]
        desiredImage := "nginx:" + site.Spec.ImageVersion
        if container.Image != desiredImage {
            container.Image = desiredImage
            updated = true
        }
    
        if container.VolumeMounts[0].MountPath != site.Spec.StaticFilePath {
            container.VolumeMounts[0].MountPath = site.Spec.StaticFilePath
            updated = true
        }
    
        if updated {
            if err := r.Update(ctx, existingDeploy); err != nil {
                logger.Error(err, "failed to update deployment")
                site.Status.Phase = "Failed"
                r.Status().Update(ctx, &site)
                return ctrl.Result{}, err
            }
            logger.Info("Updated deployment successfully", "name", site.Name)
        }
    } else {
        site.Status.Phase = "Failed"
        r.Status().Update(ctx, &site)
        return ctrl.Result{}, err
    }

    // === Service ===
    // ===============
    svc := &corev1.Service{}
    svcName := site.Name + "-svc"
    err = r.Get(ctx, client.ObjectKey{Name: svcName, Namespace: site.Namespace}, svc)
    
    if err != nil && errors.IsNotFound(err) {
        svc = &corev1.Service{
            ObjectMeta: metav1.ObjectMeta{
                Name:      svcName,
                Namespace: site.Namespace,
            },
            Spec: corev1.ServiceSpec{
                Selector: map[string]string{"app": site.Name},
                Ports: []corev1.ServicePort{
                    {
                        Port:     80,
                        Protocol: corev1.ProtocolTCP,
                        TargetPort: intstr.FromInt(80),
                    },
                },
                Type: corev1.ServiceTypeClusterIP,
            },
        }
        if err := ctrl.SetControllerReference(&site, svc, r.Scheme); err == nil {
            if err := r.Create(ctx, svc); err != nil {
                logger.Error(err, "failed to create service")
                site.Status.Phase = "Failed"
                r.Status().Update(ctx, &site)
                return ctrl.Result{}, err
            }
        }
    } else if err == nil {
        updated := false
        if svc.Spec.Type != corev1.ServiceTypeClusterIP {
            svc.Spec.Type = corev1.ServiceTypeClusterIP
            updated = true
        }
        if updated {
            if err := r.Update(ctx, svc); err != nil {
                logger.Error(err, "failed to update service")
                site.Status.Phase = "Failed"
                r.Status().Update(ctx, &site)
                return ctrl.Result{}, err
            }
        }
    } else {
        site.Status.Phase = "Failed"
        r.Status().Update(ctx, &site)
        return ctrl.Result{}, err
    }


    // === Ingress ===
    // ===============
    ing := &networkingv1.Ingress{}
    ingName := site.Name + "-ing"
    pathPrefix := "/" + site.Name
    
    err = r.Get(ctx, client.ObjectKey{Name: ingName, Namespace: site.Namespace}, ing)
    if err != nil && errors.IsNotFound(err) {
        ing = &networkingv1.Ingress{
            ObjectMeta: metav1.ObjectMeta{
                Name:      ingName,
                Namespace: site.Namespace,
            },
            Spec: networkingv1.IngressSpec{
                Rules: []networkingv1.IngressRule{
                    {
                        IngressRuleValue: networkingv1.IngressRuleValue{
                            HTTP: &networkingv1.HTTPIngressRuleValue{
                                Paths: []networkingv1.HTTPIngressPath{
                                    {
                                        Path:     pathPrefix,
                                        PathType: func() *networkingv1.PathType { pt := networkingv1.PathTypePrefix; return &pt }(),
                                        Backend: networkingv1.IngressBackend{
                                            Service: &networkingv1.IngressServiceBackend{
                                                Name: svcName,
                                                Port: networkingv1.ServiceBackendPort{
                                                    Number: 80,
                                                },
                                            },
                                        },
                                    },
                                },
                            },
                        },
                    },
                },
            },
        }
    
        if err := ctrl.SetControllerReference(&site, ing, r.Scheme); err == nil {
            if err := r.Create(ctx, ing); err != nil {
                logger.Error(err, "failed to create ingress")
                site.Status.Phase = "Failed"
                r.Status().Update(ctx, &site)
                return ctrl.Result{}, err
            }
        }
    } else if err == nil {
        updated := false
        paths := ing.Spec.Rules[0].IngressRuleValue.HTTP.Paths
        if len(paths) == 0 || paths[0].Path != pathPrefix {
            ing.Spec.Rules[0].IngressRuleValue.HTTP.Paths[0].Path = pathPrefix
            updated = true
        }
        if updated {
            if err := r.Update(ctx, ing); err != nil {
                logger.Error(err, "failed to update ingress")
                site.Status.Phase = "Failed"
                r.Status().Update(ctx, &site)
                return ctrl.Result{}, err
            }
        }
    } else {
        site.Status.Phase = "Failed"
        r.Status().Update(ctx, &site)
        return ctrl.Result{}, err
    }
    



    // Self-healing
    podList := &corev1.PodList{}
    _ = r.List(ctx, podList, client.InNamespace(site.Namespace), client.MatchingLabels{"app": site.Name})

    readyCount := int32(0)
    for _, pod := range podList.Items {
        for _, cond := range pod.Status.Conditions {
            if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
                readyCount++
            }
        }
    }

    site.Status.ReadyReplicas = readyCount
    if readyCount < site.Spec.Replicas {
        site.Status.Phase = "Pending"
	// Exponential backoff
        return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
    } else {
        site.Status.Phase = "Running"
    }
    r.Status().Update(ctx, &site)

    logger.Info("Reconciled NginxStaticSite successfully", "name", site.Name)

    return ctrl.Result{}, nil
}

// helper to parse storage size
func resourceMustParse(size string) resource.Quantity {
    q, _ := resource.ParseQuantity(size)
    return q
}

func (r *NginxStaticSiteReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&webv1alpha1.NginxStaticSite{}).
        Complete(r)
}

